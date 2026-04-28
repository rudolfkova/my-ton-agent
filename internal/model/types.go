package model

import "mytonstorage-agent/internal/constants"

type ProviderCheckRequest struct {
	JobID     string                       `json:"job_id"`
	Provider  ProviderIP                   `json:"provider"`
	Contracts []ContractToProviderRelation `json:"contracts"`
}

type ProviderCheckResponse struct {
	JobID   string                 `json:"job_id"`
	Result  []ContractProofsResult `json:"result"`
	Message string                 `json:"message,omitempty"`
}

type ContractToProviderRelation struct {
	ProviderPublicKey string `json:"provider_public_key"`
	ProviderAddress   string `json:"provider_address"`
	Address           string `json:"address"`
	BagID             string `json:"bag_id"`
	Size              uint64 `json:"size"`
}

type ProviderIP struct {
	PublicKey string `json:"public_key"`
	Storage   IPInfo `json:"storage"`
	Provider  IPInfo `json:"provider"`
}

type IPInfo struct {
	PublicKey []byte `json:"pk"`
	IP        string `json:"ip"`
	Port      int32  `json:"port"`
}

type ContractProofsResult struct {
	ContractAddress string               `json:"contract_address"`
	ProviderAddress string               `json:"provider_address"`
	Reason          constants.ReasonCode `json:"reason"`
}
