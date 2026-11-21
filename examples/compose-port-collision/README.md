# Fixture: host port collision

Deliberately broken environment for rule **SM-COMPOSE-005**. Requires Docker Engine and the Compose plugin.

## What is broken?

`web` and `admin` both publish host port 18080, so whichever starts second fails with `port is already allocated`. Host port 18080 must otherwise be free.

## Run it

From the repository root:

```bash
docker compose -f examples/compose-port-collision/compose.yaml up -d   # may report an error; that is the failure
go run ./cmd/stackmedic diagnose -file examples/compose-port-collision/compose.yaml
```

Expected: exactly one **high** `SM-COMPOSE-005` finding. Evidence: both services publishing `18080/tcp` and the start error. Recommendation: `"18081:8000"` for `admin`.

## Correct the problem

Change `admin`'s mapping to `"18081:8000"` and run `up -d`. Then rerun `diagnose`: it reports 0 findings once the service has run.

## Cleanup

```bash
docker compose -f examples/compose-port-collision/compose.yaml down -v
```

Throwaway, disposable environment only. Do not copy its credentials anywhere real.
