# Fixture: out-of-memory kill

Deliberately broken environment for rule **SM-COMPOSE-004**. Requires Docker Engine and the Compose plugin.

## What is broken?

The importer writes about 256 MiB into memory under `mem_limit: 64m` with no swap, so the kernel kills it.

## Run it

From the repository root:

```bash
docker compose -f examples/compose-oom/compose.yaml up -d   # may report an error; that is the failure
go run ./cmd/stackmedic diagnose -file examples/compose-oom/compose.yaml
```

Expected: exactly one **high** `SM-COMPOSE-004` finding. Evidence: `oom_killed=true exit_code=137` and `mem_limit: 64 MiB`.

## Correct the problem

Raise `mem_limit` (and `memswap_limit`) to `512m`, or shrink the allocation, and run `up -d --force-recreate`. Then rerun `diagnose`: it reports 0 findings once the service has run.

## Cleanup

```bash
docker compose -f examples/compose-oom/compose.yaml down -v
```

Throwaway, disposable environment only. Do not copy its credentials anywhere real.
