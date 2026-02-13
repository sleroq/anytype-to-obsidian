# anytype-to-obsidian

Exporter from Anytype JSON export (`Anytype-json`) to an Obsidian vault.

## Why

Anytype markdown export is lossy for important cases (multi-selects, object relations, rich relation metadata). This project uses Anytype json as the source of truth and builds correct markdown notes from it.

## Current Goals

- Preserve object-to-object relations.
- Preserve all fields, including multi-select/tag/status and object relations.
- Convert content blocks to Obsidian-friendly markdown.
- Convert tables to markdown tables when possible.
- Convert files/bookmarks and keep file assets in the vault.
- Keep raw Anytype details for lossless traceability.

## Usage

```bash
go run ./cmd/anytype-to-obsidian \
  -input ./Anytype-json \
  -output ./obsidian-vault
```

Flags:

- `-input`: path to `Anytype-json` directory.
- `-output`: target Obsidian vault path.

## Output Structure

- `notes/*.md` - exported notes.
- `files/*` - copied assets from Anytype export.
- `_anytype/index.json` - mapping and metadata index.
- `_anytype/raw/*.json` - raw details for each exported object.

## Development

Run tests:

```bash
go test ./...
```
