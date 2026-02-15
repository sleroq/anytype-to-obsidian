# AGENTS.md - anytype-to-obsidian

## Scope and Intent

- This file is for agents working in the root module `github.com/sleroq/anytype-to-obsidian`.
- Primary goal: convert Anytype JSON export into Obsidian artifacts with stable, loss-aware mapping.
- Source of truth is `Anytype-json/` (not markdown exports).
- Keep changes minimal, local, and behavior-preserving unless explicitly asked otherwise.

## Related Agent Rules

- There is a nested module at `anytype-heart/` with its own `AGENTS.md`; do not mix its conventions into root changes unless explicitly working there.

## Repository Layout

- `cmd/anytype-to-obsidian/main.go` - CLI entrypoint, flags, interactive setup.
- `internal/app/exporter/` - export pipeline, frontmatter/body rendering, `.base` generation.
- `internal/infra/anytypejson/` - parser/loader for Anytype `.pb.json` snapshots.
- `internal/infra/exportfs/` - filesystem copy/path/timestamp helpers.
- `internal/domain/anytype/` - shared conversion and Anytype value logic.
- `Anytype-json/` - test/dev input fixtures and reference structure.
- `obsidian-vault/` - typical output target during manual runs.

## Anytype Input Model (Important)

- `Anytype-json/objects` - object snapshots (`Page`, etc).
- `Anytype-json/relations` - relation definitions (`relationKey`, `relationFormat`, `relationMaxCount`).
- `Anytype-json/relationsOptions` - option objects for status/tag-like relations.
- `Anytype-json/types` - type metadata and recommended/hidden relation refs.
- `Anytype-json/templates` - templates mapped into Obsidian templates.
- `Anytype-json/filesObjects` + `Anytype-json/files` - file object metadata + binary files.

## Output Contract (Do Not Break)

- `notes/*.md` - exported notes and synthetic link notes.
- `templates/*.md` - exported templates.
- `bases/*.base` - converted Anytype dataview/query definitions.
- `files/*` - copied assets/files.
- `_anytype/index.json` - deterministic object ID -> note path index.
- `_anytype/raw/*.json` - raw details sidecars for each exported object.

## Build, Run, Lint, Test

- Build CLI: `go build ./cmd/anytype-to-obsidian`
- Run interactive mode: `go run ./cmd/anytype-to-obsidian`
- Run with flags: `go run ./cmd/anytype-to-obsidian -input ./Anytype-json -output ./obsidian-vault`
- Preferred full verification: `go test ./cmd/... ./internal/...`
- Alternative broad verification: `go test ./...`
- Exporter package tests: `go test ./internal/app/exporter`
- Verbose exporter tests: `go test ./internal/app/exporter -v`
- Run a single test (primary): `go test ./internal/app/exporter -run '^TestName$' -v`
- Run one named test (example): `go test ./internal/app/exporter -run '^TestExporterLinksQueriesToBaseFiles$' -v`
- Run subset by regex: `go test ./internal/app/exporter -run 'Query|Base|Dataview' -v`
- List test names quickly: `go test ./internal/app/exporter -list 'Test'`
- Re-run by exact copied name: `go test ./internal/app/exporter -run '^ExactTestName$' -v`
- Vet/lint baseline: `go vet ./...`
- Format code: `gofmt -w ./cmd ./internal`

## Single-Test Cookbook

- Typical workflow:
  - `go test ./internal/app/exporter -list 'Test'`
  - pick one test name
  - `go test ./internal/app/exporter -run '^PickedTestName$' -v`
- If test relies on regex grouping, keep anchors `^...$` to avoid accidental extra matches.
- For query conversion failures, start with a focused regex run (`-run 'Query|Base|Dataview'`).
- For markdown/frontmatter failures, run nearby tests in `internal/app/exporter/exporter_test.go` first.
- Keep `-v` on while debugging to improve signal for flaky/ordering issues.

## Style and Conventions (Go)

- Follow idiomatic Go and let `gofmt` define formatting.
- Imports are grouped standard library first, then third-party, then internal module imports.
- Keep import aliases only when needed for clarity or collision avoidance (for example `anytypedomain`).
- Prefer small pure helper functions over broad refactors.
- Keep functions focused; split large logic only when it improves readability/testability.
- Avoid introducing new dependencies unless explicitly requested.
- Preserve existing public behavior and file/output schemas.

## Types and Data Modeling

- Dynamic Anytype payloads are intentionally modeled with `map[string]any` and `[]any`.
- Convert dynamic values through existing helpers (`asString`, `asInt`, `anyToStringSlice`, etc).
- Do not over-constrain unknown Anytype fields; preserve raw values when metadata is missing.
- Prefer existing domain conversion entry points in `internal/domain/anytype`.
- Keep struct field names and JSON tags aligned with snapshot schema.

## Test Strategy Expectations

- Prefer table-driven or focused behavior tests near changed logic.
- Add/update tests for behavior-affecting changes in `internal/app/exporter/exporter_test.go`.
- Cover relation mapping edge cases: object refs, status/tag options, file refs, type fallback.
- Cover frontmatter filtering edge cases: hidden/dynamic/archived/include/exclude/empty behavior.
- Cover query conversion edge cases: filter operators, sort/group/order, property path mapping.
- Cover markdown rendering edge cases: tables, file/bookmark/link blocks, mentions, callouts/toggles.

## Practical Agent Workflow

- Before edits, locate existing pattern in the same package and mirror it.
- Prefer changing one package at a time.
- After edits run, in order when feasible:
  - `go test ./cmd/... ./internal/...`
  - `go vet ./...`
- If touching only exporter behavior, at minimum run: `go test ./internal/app/exporter -v`.
- If filenames/paths/frontmatter logic changed, validate resulting files in `obsidian-vault/` manually.

## Common Pitfalls

- Do not silently drop unknown Anytype details.
- Do not assume one relation key source; ID and key both appear in exports.
- Do not break relative path behavior for note links vs file links.
- Do not alter output directory contract or naming stability.
- Do not pull `anytype-heart` module commands/conventions into root tasks unintentionally.

## A note to the agent

We are building this together. When you learn something non-obvious, add it to the AGENTS.md file of the corresponding project so future changes can go faster.
