# Fixture: missing bind mount source

Deliberately broken environment for rule **SM-COMPOSE-006**. Requires Docker Engine and the Compose plugin.

## What is broken?

The bind source `./settings.json` does not exist. `create_host_path: false` makes Docker refuse to start the container instead of silently creating an empty root-owned directory (the default for short syntax).

## Run it

From the repository root:

```bash
docker compose -f examples/compose-missing-mount/compose.yaml up -d   # may report an error; that is the failure
go run ./cmd/stackmedic diagnose -file examples/compose-missing-mount/compose.yaml
```

Expected: exactly one **high** `SM-COMPOSE-006` finding. Evidence: the mount and `settings.json: missing` (StackMedic checks the path on this machine).

## Correct the problem

Create the file, e.g. `echo '{}' > examples/compose-missing-mount/settings.json`, and run `up -d`. Delete it again afterwards. Then rerun `diagnose`: it reports 0 findings once the service has run.

## Cleanup

```bash
docker compose -f examples/compose-missing-mount/compose.yaml down -v
```

Throwaway, disposable environment only. Do not copy its credentials anywhere real.
