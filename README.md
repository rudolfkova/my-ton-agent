# mytonstorage-agent

Второй микросервис для вынесения тяжелых provider proof-check операций из координатора.

## Назначение

- принимает задачу от координатора (`POST /internal/v1/jobs/provider-check`);
- выполняет ADNL/RLDP проверку провайдера;
- возвращает нормализованные результаты координатору;

## Конфиг

Скопируйте шаблон:

```bash
cp .env.example .env
```

Переменные в `.env`:

- `AGENT_PORT` (по умолчанию `9091`)
- `AGENT_ACCESS_TOKEN` (опционально, если задан — нужен `Authorization: Bearer ...`)
- `AGENT_LOG_LEVEL` (`0..3`, по умолчанию `1`)
- `COORDINATOR_URL` (например `http://coordinator:9090`)
- `COORDINATOR_ACCESS_TOKEN` (должен совпадать с `AGENT_REGISTRATION_TOKEN` на координаторе)
- `AGENT_ID` (уникальный идентификатор агента)
- `AGENT_PUBLIC_URL` (адрес, куда координатор отправляет check jobs, например `http://157.22.231.18:9091`)
- `AGENT_REGISTRATION_INTERVAL_SEC` (интервал heartbeat, по умолчанию `15`)
- `AGENT_COORDINATOR_TIMEOUT_SEC` (таймаут запросов к координатору, по умолчанию `10`)

## Локальный запуск

```bash
go mod tidy
set -a && source .env && set +a
go run ./cmd
```

## Docker запуск

```bash
cp .env.example .env
docker compose up --build -d
docker compose ps
```

Логи:

```bash
docker compose logs -f agent
```

Healthcheck:

```bash
curl -s http://localhost:9091/health
```