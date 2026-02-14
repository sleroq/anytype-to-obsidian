# anytype-to-obsidian: implementation notes

## Goal

- Source of truth is `Anytype-json` (not `Anytype-md`).
- Export to Obsidian with correct relations, properties, blocks, templates, and query -> `.base` conversion.

## Important data layout

- `Anytype-json/objects` - main objects.
- `Anytype-json/relations` - relation defs (`relationKey`, `relationFormat`).
- `Anytype-json/relationsOptions` - options for tag/status-like relations.
- `Anytype-json/types` - type objects.
- `Anytype-json/templates` - templates.
- `Anytype-json/filesObjects` + `Anytype-json/files` - file metadata and binaries.

## Core code paths

- `cmd/anytype-to-obsidian/main.go` - CLI flags + interactive mode (no args).
- `internal/app/exporter/exporter.go` - main export pipeline and mapping.
- `internal/app/exporter/*_test.go` - behavior tests.
- `internal/infra/anytypejson` - Anytype JSON parsing.
- `internal/domain` - conversion/value helpers.

## Rules that matter for new features

- Resolve object detail keys both by relation key and relation object ID.
- Keep raw values when metadata is missing (do not drop unknown data).
- For `type` relation values, support IDs from `types/*.pb.json` as fallback.
- `link-as-note-properties` can force values like `type,tag,status` to be note links.
- Dataview/query blocks must export to `bases/*.base` with filters/sort/group/order.
- File names must be deterministic and collision-safe (`name.md`, `name-2.md`, ...).
- Apply Anytype timestamps to exported files (`mtime`/`atime`; macOS birthtime when available).

## Output contract

- `notes/*.md`
- `templates/*.md`
- `bases/*.base`
- `files/*`
- `_anytype/index.json`
- `_anytype/raw/*.json`

## Verification commands

- Preferred full check: `go test ./cmd/... ./internal/...`
- Exporter tests only: `go test ./internal/app/exporter`
- Single test: `go test ./internal/app/exporter -run '^TestName$' -v`
- Vet: `go vet ./...`

## Feature areas to protect with tests

- Relation mapping (object/tag/status/type/file).
- Multi-select/select preservation.
- Query/dataview -> `.base` conversion.
- Table/file/bookmark/link block conversion.
- Deterministic naming and frontmatter filtering behavior.

## Style

- Prefer small local fixes over big refactors.
- Do not add new dependencies unless explicitly requested.
- `map[string]any` is acceptable for dynamic Anytype payload parts.
