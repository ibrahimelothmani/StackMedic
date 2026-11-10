# Contributing to StackMedic

Thanks for helping build StackMedic: an evidence-first, local-first toolkit for diagnosing application and infrastructure failures.

> **Status:** StackMedic is pre-alpha. Its diagnostic engine and public plugin APIs are still being designed. Please check current issues and discussions before proposing substantial implementation work.

## Where to start

- **Documentation:** Improve setup instructions, architecture explanations, or reproducible troubleshooting recipes.
- **Failure fixtures:** Add a minimal, safe Docker Compose example that reproduces a specific failure with exact expected observations.
- **Diagnostic rules (once the core exists):** Contribute a rule with evidence extraction, severity, guidance, and tests.
- **Bug reports:** Include the environment, expected behavior, actual behavior, safe-to-share logs, and a minimal reproduction.

Look for issues labeled `good first issue` and `help wanted` once the maintainers have published them.

## Proposing a contribution

1. Search existing issues and discussions to avoid duplicate work.
2. Open an issue before starting a major feature or changing a public interface.
3. Fork the repository, create a descriptive branch, and work on one focused change.
4. Add or update tests and documentation as appropriate.
5. Open a pull request explaining **what changed**, **why it matters**, **how you verified it**, and any **security implications**.

Example branch names: `docs/improve-setup`, `fixture/compose-dns-mismatch`, `fix/compose-parser`.

## Local example: DNS mismatch

This repo includes a deliberately broken fixture that does not depend on StackMedic's unfinished CLI:

```bash
docker compose -f examples/compose-dns-mismatch/compose.yaml up -d
docker compose -f examples/compose-dns-mismatch/compose.yaml logs api
docker compose -f examples/compose-dns-mismatch/compose.yaml down -v
```

See `examples/compose-dns-mismatch/README.md` for the failure, intended diagnosis, and cleanup instructions.

## Quality expectations

- Prefer **small, reviewable** pull requests with a clear scope.
- Keep diagnostic findings **evidence-backed**; do not present guesses as facts.
- Keep tests deterministic and reproducible, especially for infrastructure failures.
- Default to **read-only** diagnostics. Any future remediation must be explicitly approved, bounded, and verifiable.
- Never commit real secrets, private keys, credentials, or unredacted user logs.
- Avoid introducing a cloud dependency or requiring a paid LLM for core functionality.
- Keep contributor interfaces documented and avoid breaking changes without discussion.

## Security issues

Please **do not** report exploitable vulnerabilities in public issues. Follow [SECURITY.md](SECURITY.md).

## License

By submitting a contribution, you agree that your contribution will be distributed under the repository's [Apache License 2.0](LICENSE).
