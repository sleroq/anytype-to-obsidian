# anytype-to-obsidian

Convert Anytype JSON Export into Obsidian Markdown with relations and metadata preserved.

## Features

- Relations are exported correctly (object links, tags, statuses, files, types, multi-selects).
- Anytype queries/dataviews are converted into Obsidian Bases (`.base`) files.
- Property visibility is configurable: include dynamic, archived, hidden, empty, or specific properties.
- Select-like values (for example `tag`, `status`, `type`) can be exported as linked objects/notes when needed.
- Pretty Properties plugin integration. Compatible properties are converted and renamed.
- Anytype templates are exported.
- Iconize plugin integration: `iconEmoji`/`iconImage` are exported to `.obsidian/plugins/obsidian-icon-folder/data.json` (with generated icon pack files for image icons).
- Anytype -> Obsidian block conversion is fully supported in this project scope; if you find an unsupported block, please open an issue.

## Quick start

1. Export your Anytype Space in Any-Block format. Choose `JSON` as the file format and disale `Zip archive`
2. Get binary for your platform from [releases](https://github.com/sleroq/anytype-to-obsidian/releases)
3. Run it specifying location of exported space: `anytype-to-obsidian.exe -input ./Anytype-exported-json -output ./result-directory`

## Usage

Run in interactive mode (no arguments):

```bash
go run ./cmd/anytype-to-obsidian
```

This opens a setup form and uses these defaults:

- input: `./Anytype-json`
- output: `./obsidian-vault`

Or run directly with flags:

```bash
go run ./cmd/anytype-to-obsidian -input ./Anytype-exported-json -output ./result-directory
```

## Main options

- `-input`: path to `Anytype-json`.
- `-output`: output Obsidian vault path.
- `-prettier`: format exported markdown via `npx prettier` (`true` by default).
- `-filename-escaping`: `auto`, `posix`, or `windows`.
- `-include-dynamic-properties`: include system-managed Anytype fields.
- `-include-archived-properties`: include unresolved/archived relation fields.
- `-exclude-empty-properties`: drop empty frontmatter values.
- `-exclude-properties`: comma-separated property keys/names to exclude.
- `-force-include-properties`: comma-separated property keys/names to include even if hidden by default.
- `-link-as-note-properties`: comma-separated relation keys/names to export as note links (for example `type,tag,status`).
- `-disable-picture-to-cover`: keep the original `picture` property name instead of exporting it as `cover`.
- `-disable-iconize-icons`: disable Iconize plugin data/icon export.

Property precedence:

- `force-include` -> `exclude` -> default hidden/dynamic/archived rules
- then `-exclude-empty-properties` removes remaining empty values

## Output

- `notes/*.md` - exported notes.
- `templates/*.md` - exported templates.
- `bases/*.base` - exported Obsidian Bases from Anytype queries.
- `files/*` - copied files/assets.
- `_anytype/index.json` - mapping/index metadata.
- `_anytype/raw/*.json` - raw Anytype payload sidecars.

## Issues

If some relation, property, query, or block does not export as expected, open an issue with a minimal example object/export.
