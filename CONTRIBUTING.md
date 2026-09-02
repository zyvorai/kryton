# Contributing

1. Keep provider-specific behavior behind `internal/provider.Provider` (`internal/demo`, `internal/dockur`, `internal/kubevirt`).
2. Do not expose raw provider identifiers as the primary public machine ID.
3. Add tests for lifecycle and error paths.
4. Run `make check` before opening a pull request. CI also runs `make lint` (golangci-lint) and `make vulncheck` (govulncheck) plus a gosec scan — run those locally too if you have the tools installed.
5. Keep the project free of Microsoft installation media, keys, activation bypasses, or other license-restricted assets.
6. Update docs (`README.md`, `docs/`) when adding API fields, providers, or configuration.
7. New source files must include the Apache-2.0 header. Run `./scripts/add-license-headers.sh` to apply headers to Go, shell, web, and OpenAPI files.
8. Add a line under `## [Unreleased]` in `CHANGELOG.md` for any user-visible change (new field, provider, config variable, fix).

Contributions are accepted under the Apache License 2.0.
