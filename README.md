# anytype-to-obsidian

Exporter from Anytype JSON export (`Anytype-json`) to an Obsidian vault.

## Why

Anytype markdown export is lossy for important cases (multi-selects, object relations, rich relation metadata). This project uses Anytype json as the source of truth and builds correct markdown notes from it.

## Features

- Preserve object-to-object relations.
- Preserve relation-backed fields, including multi-select/tag/status, object, file, and type relations.
- Resolve relation keys by relation key or relation object ID, with readable frontmatter keys where possible.
- Apply type-aware frontmatter ordering: type-visible properties first, then type-hidden properties, then remaining properties.
- Convert supported blocks (text, file, bookmark, latex, link, table) to Obsidian-friendly markdown
- Convert Anytype dataview/query blocks into Obsidian `.base` files with per-view filters, grouping, ordering, and sort metadata.
- Support all current Anytype dataview filter conditions and quick-date options when rendering base filters.
- Optionally render selected relation values as note links (`-link-as-note-properties`), including synthetic notes for missing tag/status/type option/type objects.
- Export templates

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
- `-exclude-empty-properties`: exclude frontmatter properties with empty values (`null`, empty string, empty array, empty object).
- `-exclude-properties`: comma-separated list of property keys/names to always exclude from frontmatter.
- `-force-include-properties`: comma-separated list of property keys/names to always include in frontmatter.
- `-filename-escaping`: filename escaping mode (`auto`, `posix`, `windows`).
- `-link-as-note-properties`: comma-separated relation property keys/names to render values as note links when possible (for example `type,tag,status`).

Dynamic properties are excluded by default because Obsidian manages equivalents itself (for example backlinks), and these values are backend-managed in Anytype.

Archived/unresolved relation properties are also excluded by default when the exporter cannot resolve a readable relation name. Use `-include-archived-properties` to keep those raw keys.

### Property Filtering

The exporter hides certain properties by default to keep frontmatter clean:

- **Dynamic properties** (e.g., `backlinks`, `lastModifiedDate`, `lastOpenedDate`, `syncStatus`) — excluded by default; enable with `-include-dynamic-properties`.
- **Internal properties** (e.g., `id`, `spaceId`, `layout`, `createdDate`, `internalFlags`, `featuredRelations`) — always excluded to avoid clutter.

To override these rules for specific properties:

- Use `-force-include-properties "anytype_id,lastModifiedDate"` to include specific properties without enabling all dynamic ones.
- Use `-exclude-properties "customField,backlinks"` to explicitly exclude properties even when they would normally be included.
- Use `-exclude-empty-properties` to skip properties that resolve to empty values in frontmatter.

Precedence: `force-include` > `exclude` > default hidden/dynamic/archived rules; then `-exclude-empty-properties` removes remaining empty values.

## Output Structure

- `notes/*.md` - exported notes.
- `templates/*.md` - exported templates from Anytype with type-prefixed filenames (e.g., `Human - Contact.md`).
- `bases/*.base` - exported Obsidian Bases files generated from Anytype dataview/query blocks.
- `files/*` - copied assets from Anytype export.
- `_anytype/index.json` - mapping and metadata index.
- `_anytype/raw/*.json` - raw details for each exported object.

## Development

Run tests:

```bash
go test ./...
```
