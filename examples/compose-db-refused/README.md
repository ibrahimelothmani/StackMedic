# Fixture: connection refused on the wrong port

Deliberately broken environment for rule **SM-COMPOSE-007**. Requires Docker Engine and the Compose plugin.

## What is broken?

`api` connects to `postgres:5433`, but postgres listens on 5432. postgres is healthy, so the refusal points at the port.

## Run it

From the repository root:

```bash
docker compose -f examples/compose-db-refused/compose.yaml up -d   # may report an error; that is the failure
go run ./cmd/stackmedic diagnose -file examples/compose-db-refused/compose.yaml
```

Expected: exactly one **high** `SM-COMPOSE-007` finding. Evidence: the `Connection refused` log line, `DATABASE_PORT=5433`, and the postgres image default port 5432. Recommendation: `Change DATABASE_PORT to "5432".`

## Correct the problem

Set `DATABASE_PORT: "5432"` and run `up -d --force-recreate api`. Then rerun `diagnose`: it reports 0 findings once the service has run.

## Cleanup

```bash
docker compose -f examples/compose-db-refused/compose.yaml down -v
```

Throwaway, disposable environment only. Do not copy its credentials anywhere real.
