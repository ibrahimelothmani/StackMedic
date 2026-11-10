# Fixture: Docker Compose service-name / DNS mismatch

This deliberately broken environment demonstrates a common dependency failure that StackMedic should detect in its first release. **The StackMedic CLI is not implemented yet**; the example currently works with Docker Compose alone.

## What is broken?

In `compose.yaml`, the database service is named `postgres`, but the simulated API uses `DATABASE_HOST=database`.

Docker Compose makes services discoverable by **service name** on the default network. `database` is not defined as a service or network alias here, so the API process fails DNS resolution, logs an error, and exits. Compose retries the API a limited number of times.

## Run the fixture

Requires Docker Engine and the Docker Compose plugin.

From the **repository root**:

```bash
docker compose -f examples/compose-dns-mismatch/compose.yaml up -d
docker compose -f examples/compose-dns-mismatch/compose.yaml ps -a
docker compose -f examples/compose-dns-mismatch/compose.yaml logs api
```

Expected observation: the `postgres` service becomes healthy, while the `api` container logs an error similar to `unable to resolve database hostname 'database'`.

The exact text of the OS-level DNS error and the container status can vary by Docker version and platform.

## Correct the problem

Change `DATABASE_HOST: database` to `DATABASE_HOST: postgres` in `compose.yaml`, then run:

```bash
docker compose -f examples/compose-dns-mismatch/compose.yaml up -d --force-recreate api
docker compose -f examples/compose-dns-mismatch/compose.yaml logs api
```

The script should now report that `postgres` resolves successfully. This fixture deliberately checks DNS resolution only; it is **not** a real database client or API health check.

## Planned StackMedic behavior

Once implemented, StackMedic should identify the mismatch between the configured hostname and the existing Compose service names, cite the log and configuration evidence, suggest the smallest fix, and perform **no automatic changes**.

## Cleanup

```bash
docker compose -f examples/compose-dns-mismatch/compose.yaml down -v
```

`-v` deletes any Docker volumes associated with this isolated demo. Do not run it against a production Compose project.

## Safety

This fixture uses throwaway credentials, publishes no network ports, and should be run only in a disposable development environment. Do not copy its passwords into real infrastructure.
