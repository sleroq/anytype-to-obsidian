# Project info

We are creating anytype-to-obsidian exporter. Anytype's exports are in ./Anytype-md/ and ./Anytype-json/ folders. Anytype-md is mostly useless as it's losing data as multi-selects and relations

We need to read exported data from Anytype-json and create exporter which:

- preserves all of the relations between the objects
- preserves all of the fields, inclusing multi-selects and selects with objects
- preserves tables and embeds if it's possible to convert them to obsidian alternative

You have anytype-mcp connected so you can query production anytype server which has saygex space, identical to the exports. So we can compare the data if needed.

in the anytype-heart folder we have anytype's backend which might be helpful to understand how export is created ./anytype-heart/core/block/export/export.go and how to parse the json / what types to infer so we don't reinvent them

we need to cover our exporter's features with tests so we can make sure we support properties mentioned above

## Context

- Source of truth: `Anytype-json`.
- `Anytype-md` is intentionally not used due to data loss.
- Target: Obsidian-compatible markdown + filesystem assets.

## Key Anytype Export Facts

- Files are protobuf-snapshot JSON (`.pb.json`).
- Entity folders:
  - `objects/` - regular content objects.
  - `relations/` - relation definitions (`relationKey`, `relationFormat`).
  - `relationsOptions/` - options for tag/status-like relations.
  - `types/` - object type definitions.
  - `templates/` - template objects.
  - `filesObjects/` - file object metadata.
  - `files/` - binary file assets.

## DDD Shape (lightweight)

- `internal/domain`: entities + value conversion rules.
- `internal/infra/anytypejson`: export reader/parser.
- `internal/app/exporter`: orchestration and markdown projection.
- `cmd/anytype-to-obsidian`: CLI entrypoint.

## Mapping Rules (may be changed later)

1. Build indexes
   - object id -> object snapshot
   - relation key -> relation definition
   - relation option id -> option object
   - file object id -> source file path

2. Property conversion
   - Some object details keys use relation object IDs instead of `relationKey`; resolve via relation index before filtering/mapping.
   - `object` relation format: render note links when possible.
   - `type` relation values can point to IDs from `types/*.pb.json` (not only `objects/`), so keep an id->name index for `types/` as fallback when note link is unavailable.
   - `tag` / `status`: map option IDs to option names.
   - `file`: map file object IDs to `files/<name>`.
   - unknown/missing metadata: preserve raw value.

3. Block conversion
   - text -> markdown text/styles.
   - file block -> markdown link/image.
   - bookmark block -> markdown link.
   - latex block -> `$$...$$`.
   - table block -> markdown table (best effort).
   - unsupported block -> raw fallback snippet.

4. Preservation
   - frontmatter for readable properties.
   - sidecar raw JSON per object for lossless data (`_anytype/raw`).
   - global index file for deterministic mapping (`_anytype/index.json`).
   - note filenames must prefer object `details.name`; fallback to root `Title` block text, then `details.title`, then object id.
   - filename collisions must be resolved deterministically with numeric suffixes (`name.md`, `name-2.md`, ...).
   - filename escaping mode is configurable: `auto` (by runtime OS), `posix`, `windows`.

## Test Focus

- relation mapping correctness (object/tag/status/file).
- multi-select preservation.
- table conversion behavior.
- file/bookmark rendering.
- output determinism for filename mapping.

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
- Exclude specific properties even when they are normally included:
  - `go run ./cmd/anytype-to-obsidian -exclude-properties "id,spaceId"`
- Force-include specific properties without enabling all dynamic ones:
  - `go run ./cmd/anytype-to-obsidian -force-include-properties "anytype_id,lastModifiedDate"`

### Property Filtering Notes

- `dynamicPropertyKeys` covers system-managed fields that can be globally enabled by `-include-dynamic-properties`.
- `defaultHiddenPropertyKeys` covers non-dynamic fields that are still hidden by default.
- Property filter precedence: `force-include` > `exclude` > default hidden/dynamic/archived rules.
- Type-aware frontmatter order should use `types/*.pb.json` lists in this order: `recommendedFeaturedRelations` + `recommendedRelations` + `recommendedFileRelations`, then `recommendedHiddenRelations`, then remaining object keys.
- Relation IDs in type recommendation lists may be stale/missing in `relations/`; always resolve best-effort and keep fallback behavior (do not drop unmatched object properties).

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
