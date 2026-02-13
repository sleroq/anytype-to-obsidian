# Project info

We are creating anytype-to-obsidian exporter. Anytype's exports are in ./Anytype-md/ and ./Anytype-json/ folders. Anytype-md is mostly useless as it's losing data as multi-selects and relations

We need to read exported data from Anytype-json and create exporter which:
- preserves all of the relations between the objects
- preserves all of the fields, inclusing multi-selects and selects with objects
- preserves tables and embeds if it's possible to convert them to obsidian alternative

You have anytype-mcp connected so you can query production anytype server which has saygex space, identical to the exports. So we can compare the data if needed.

in the anytype-heart folder we have anytype's backend which might be helpful to understand how export is created ./anytype-heart/core/block/export/export.go and how to parse the json / what types to infer so we don't reinvent them

we need to cover our exporter's features with tests so we can make sure we support properties mentioned above


### Test

- Run all tests in root module:
  - `go test ./...`
- Run tests in exporter package only:
  - `go test ./internal/app/exporter`
- Run a single test by exact name (important):
  - `go test ./internal/app/exporter -run '^TestExporterPreservesRelationsAndFields$' -v`
- Run tests by pattern:
  - `go test ./internal/app/exporter -run 'TestExporter.*' -v`
- Disable test cache when validating changes:
  - `go test ./... -count=1`

### Lint / Static Analysis

- Primary static checks:
  - `go vet ./...`
- If `golangci-lint` is installed:
  - `golangci-lint run ./...`
- Current known lint status (as of this file creation):
  - `errcheck` warnings exist in `internal/app/exporter/exporter.go` for deferred `Close()` return values.

### Run CLI Locally

- Run exporter with defaults:
  - `go run ./cmd/anytype-to-obsidian`
- Run with explicit paths:
  - `go run ./cmd/anytype-to-obsidian -input ./Anytype-json -output ./obsidian-vault`
- Include dynamic/system-managed Anytype properties:
  - `go run ./cmd/anytype-to-obsidian -include-dynamic-properties`

## Code Style Guidelines

### General Go Style

- Write idiomatic Go.
- Keep functions focused and small where practical.
- Prefer simple local fixes over broad refactors.
- Avoid introducing new dependencies unless explicitly requested.

### Types and Data Structures

- Prefer concrete types over `any` unless handling dynamic JSON-like payloads.
- In this codebase, `map[string]any` is acceptable for Anytype `details` payloads.
- Keep conversion helpers explicit (`asString`, `asInt`, etc.) rather than implicit casts.
- Do not use type suppression patterns that hide bugs.

## A note to the agent

We are building this together. When you learn something non-obvious, add it to the AGENTS.md file of the corresponding project so future changes can go faster.

Write all code comments and log messages only in the Russian language. Use anglicisms but write in Russian.
