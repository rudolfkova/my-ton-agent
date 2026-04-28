package checker

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"fmt"
	"log/slog"
	"math/rand"
	"strconv"
	"sync"
	"time"

	"github.com/xssnick/tonutils-go/adnl"
	"github.com/xssnick/tonutils-go/adnl/keys"
	"github.com/xssnick/tonutils-go/adnl/overlay"
	"github.com/xssnick/tonutils-go/adnl/rldp"
	"github.com/xssnick/tonutils-go/tl"
	"github.com/xssnick/tonutils-go/tlb"
	"github.com/xssnick/tonutils-go/tvm/cell"
	"github.com/xssnick/tonutils-storage/storage"

	"mytonstorage-agent/internal/constants"
	"mytonstorage-agent/internal/model"
)

const (
	pingTimeout    = 7 * time.Second
	rlQueryTimeout = 10 * time.Second
)

type Checker interface {
	CheckProvider(ctx context.Context, req model.ProviderCheckRequest) ([]model.ContractProofsResult, error)
}

type checker struct {
	prv    ed25519.PrivateKey
	logger *slog.Logger
}

func NewChecker(logger *slog.Logger) (Checker, error) {
	_, prv, err := ed25519.GenerateKey(nil)
	if err != nil {
		return nil, fmt.Errorf("failed to generate checker key: %w", err)
	}

	return &checker{
		prv:    prv,
		logger: logger,
	}, nil
}

func (c *checker) CheckProvider(ctx context.Context, req model.ProviderCheckRequest) ([]model.ContractProofsResult, error) {
	log := c.logger.With(
		slog.String("job_id", req.JobID),
		slog.String("provider_pubkey", req.Provider.PublicKey),
	)

	gw := adnl.NewGateway(c.prv)
	defer gw.Close()

	if err := gw.StartClient(); err != nil {
		return nil, fmt.Errorf("failed to start adnl gateway: %w", err)
	}

	var bagsStatuses sync.Map
	checkProviderFiles(ctx, gw, req.Provider, req.Contracts, &bagsStatuses, log)

	result := make([]model.ContractProofsResult, 0, len(req.Contracts))
	bagsStatuses.Range(func(_, value any) bool {
		proof, ok := value.(model.ContractProofsResult)
		if ok {
			result = append(result, proof)
		}
		return true
	})

	return result, nil
}

func checkProviderFiles(
	ctx context.Context,
	gw *adnl.Gateway,
	providerIP model.ProviderIP,
	storageContracts []model.ContractToProviderRelation,
	bagsStatuses *sync.Map,
	log *slog.Logger,
) {
	log.Debug("start checking provider files")
	start := time.Now()
	defer func() {
		log.Debug("finished checking provider files", slog.String("duration", time.Since(start).String()))
	}()

	stats := make(map[constants.ReasonCode]int)
	maxFailureThreshold := uint32(float32(len(storageContracts)) / 100.0 * 20.0)
	var failsInARow uint32

	addr := providerIP.Storage.IP + ":" + strconv.Itoa(int(providerIP.Storage.Port))
	peer, regErr := gw.RegisterClient(addr, providerIP.Storage.PublicKey)
	if regErr != nil {
		log.Debug("failed to create adnl peer", slog.String("error", regErr.Error()))
		fillStatuses(bagsStatuses, storageContracts, providerIP.Storage.IP, providerIP.Storage.Port, constants.CantCreatePeer)
		return
	}

	pingCtx, pingCancel := context.WithTimeout(ctx, pingTimeout)
	_, pingErr := peer.Ping(pingCtx)
	pingCancel()
	if pingErr != nil {
		log.Debug("initial provider ping failed", slog.String("error", pingErr.Error()))
		fillStatuses(bagsStatuses, storageContracts, providerIP.Storage.IP, providerIP.Storage.Port, constants.FailedInitialPing)
		return
	}

	rl := rldp.NewClientV2(peer)
	defer rl.Close()

	for _, sc := range storageContracts {
		statusKey := getKey(sc.BagID, providerIP.Storage.IP, providerIP.Storage.Port)

		if failsInARow > maxFailureThreshold {
			bagsStatuses.Store(statusKey, model.ContractProofsResult{
				ContractAddress: sc.Address,
				ProviderAddress: sc.ProviderAddress,
				Reason:          constants.UnavailableProvider,
			})
			log.Debug("skip contract due to failure threshold", slog.String("bag_id", sc.BagID))
			continue
		}

		reason := checkPiece(ctx, rl, sc.BagID, log)
		bagsStatuses.Store(statusKey, model.ContractProofsResult{
			ContractAddress: sc.Address,
			ProviderAddress: sc.ProviderAddress,
			Reason:          reason,
		})

		stats[reason]++
		if reason == constants.ValidStorageProof {
			failsInARow = 0
		} else {
			failsInARow++
		}

		time.Sleep(500 * time.Millisecond)
	}

	for reason, count := range stats {
		log.Debug("checked provider files", slog.Int("reason", int(reason)), slog.Int("count", count))
	}
}

func checkPiece(ctx context.Context, rl *rldp.RLDP, bagID string, log *slog.Logger) (reason constants.ReasonCode) {
	log = log.With(slog.String("bag_id", bagID))
	reason = constants.NotFound

	peer, ok := rl.GetADNL().(adnl.Peer)
	if !ok {
		log.Error("failed to get adnl peer")
		return constants.UnknownPeer
	}

	peer.Reinit()
	est := time.Now()

	pingCtx, cancelPing := context.WithTimeout(ctx, pingTimeout)
	_, err := peer.Ping(pingCtx)
	cancelPing()
	if err != nil {
		log.Debug("ping to provider failed", slog.String("error", err.Error()))
		return constants.PingFailed
	}

	bag, err := hex.DecodeString(bagID)
	if err != nil {
		log.Debug("failed to decode bag id", slog.String("error", err.Error()))
		return constants.InvalidBagID
	}

	over, err := tl.Hash(keys.PublicKeyOverlay{Key: bag})
	if err != nil {
		log.Debug("failed to hash overlay key", slog.String("error", err.Error()))
		return constants.InvalidBagID
	}

	if time.Since(est) > 5*time.Second {
		peer.Reinit()
		est = time.Now()
	}

	var res storage.TorrentInfoContainer
	rlCtx, cancelQuery := context.WithTimeout(ctx, rlQueryTimeout)
	err = rl.DoQuery(rlCtx, 32<<20, overlay.WrapQuery(over, &storage.GetTorrentInfo{}), &res)
	cancelQuery()
	if err != nil {
		log.Debug("failed to get torrent info from provider", slog.String("error", err.Error()))
		return constants.GetInfoFailed
	}

	cl, err := cell.FromBOC(res.Data)
	if err != nil {
		log.Debug("failed to parse boc of torrent info", slog.String("error", err.Error()))
		return constants.InvalidHeader
	}

	if !bytes.Equal(cl.Hash(), bag) {
		log.Debug("hash not equal bag")
		return constants.InvalidHeader
	}

	var info storage.TorrentInfo
	if err = tlb.LoadFromCell(&info, cl.BeginParse()); err != nil {
		log.Debug("failed to load torrent info from cell", slog.String("error", err.Error()))
		return constants.InvalidHeader
	}

	pieceID := int32(1)
	var piecesCount int32
	if info.PieceSize != 0 {
		piecesCount = int32(info.FileSize / uint64(info.PieceSize))
	}
	if piecesCount != 0 {
		pieceID = rand.Int31n(piecesCount)
	}

	if time.Since(est) > 5*time.Second {
		peer.Reinit()
	}

	var piece storage.Piece
	rl2Ctx, cancelPiece := context.WithTimeout(ctx, rlQueryTimeout)
	err = rl.DoQuery(rl2Ctx, 32<<20, overlay.WrapQuery(over, &storage.GetPiece{PieceID: pieceID}), &piece)
	cancelPiece()
	if err != nil {
		log.Debug("failed to get piece from provider", slog.String("error", err.Error()))
		return constants.CantGetPiece
	}

	proof, err := cell.FromBOC(piece.Proof)
	if err != nil {
		log.Debug("failed to parse boc of piece", slog.String("error", err.Error()))
		return constants.CantParseBoC
	}

	if err = cell.CheckProof(proof, info.RootHash); err != nil {
		log.Debug("proof check failed", slog.String("error", err.Error()))
		return constants.ProofCheckFailed
	}

	return constants.ValidStorageProof
}

func fillStatuses(
	bagsStatuses *sync.Map,
	storageContracts []model.ContractToProviderRelation,
	ip string,
	port int32,
	reason constants.ReasonCode,
) {
	for _, sc := range storageContracts {
		statusKey := getKey(sc.BagID, ip, port)
		bagsStatuses.Store(statusKey, model.ContractProofsResult{
			ContractAddress: sc.Address,
			ProviderAddress: sc.ProviderAddress,
			Reason:          reason,
		})
	}
}

func getKey(bagID string, ip string, port int32) string {
	return bagID + "-" + ip + ":" + strconv.Itoa(int(port))
}
