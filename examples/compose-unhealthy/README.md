# Fixture: failing healthcheck

Deliberately broken environment for rule **SM-COMPOSE-003**. Requires Docker Engine and the Compose plugin.

## What is broken?

The healthcheck runs `pg_isready ... -p 5433`, but postgres listens on 5432. postgres never becomes healthy, so `api` (which waits for `service_healthy`) is never started.

## Run it

From the repository root:

```bash
docker compose -f examples/compose-unhealthy/compose.yaml up -d   # may report an error; that is the failure
go run ./cmd/stackmedic diagnose -file examples/compose-unhealthy/compose.yaml
```

Expected: exactly one **high** `SM-COMPOSE-003` finding. Evidence: probe output `/run/postgresql:5433 - no response` and `api` waiting on it.

## Correct the problem

Change the probe to `-p 5432` and run `up -d --force-recreate`. Then rerun `diagnose`: it reports 0 findings once the service has run.

## Cleanup

```bash
docker compose -f examples/compose-unhealthy/compose.yaml down -v
```

Throwaway, disposable environment only. Do not copy its credentials anywhere real.
