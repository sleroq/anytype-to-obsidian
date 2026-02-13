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
- `-include-dynamic-properties`: include dynamic/system-managed Anytype properties in note frontmatter.
- `-include-archived-properties`: include archived/unresolved relation properties that do not have readable relation names in the export.

Dynamic properties are excluded by default because Obsidian manages equivalents itself (for example backlinks), and these values are backend-managed in Anytype.

Archived/unresolved relation properties are also excluded by default when the exporter cannot resolve a readable relation name. Use `-include-archived-properties` to keep those raw keys.

Default excluded dynamic property keys:

- `addedDate`
- `backlinks`
- `fileBackupStatus`
- `fileIndexingStatus`
- `fileSyncStatus`
- `lastMessageDate`
- `lastModifiedBy`
- `lastModifiedDate`
- `lastOpenedBy`
- `lastOpenedDate`
- `lastUsedDate`
- `links`
- `mentions`
- `revision`
- `syncDate`
- `syncError`
- `syncStatus`

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
