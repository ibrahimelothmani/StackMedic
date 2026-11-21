# Fixture: crash loop

Deliberately broken environment for rule **SM-COMPOSE-002**. Requires Docker Engine and the Compose plugin.

## What is broken?

`QUEUE_NAME` is empty, so `worker.py` exits with code 3 on every start and Compose restarts it until `on-failure:3` gives up.

## Run it

From the repository root:

```bash
docker compose -f examples/compose-crash-loop/compose.yaml up -d   # may report an error; that is the failure
go run ./cmd/stackmedic diagnose -file examples/compose-crash-loop/compose.yaml
```

Expected: exactly one **high** `SM-COMPOSE-002` finding. Evidence: `exit_code=3 restart_count=3` and the log line `FATAL: required setting QUEUE_NAME is empty`.

## Correct the problem

Set `QUEUE_NAME: jobs` in `compose.yaml` and run `up -d --force-recreate`. Then rerun `diagnose`: it reports 0 findings once the service has run.

## Cleanup

```bash
docker compose -f examples/compose-crash-loop/compose.yaml down -v
```

Throwaway, disposable environment only. Do not copy its credentials anywhere real.
