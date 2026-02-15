# anytype-to-obsidian

Convert Anytype JSON Export into Obsidian Markdown with relations and metadata preserved.

## Features

- Relations are exported correctly (object links, tags, statuses, files, types, multi-selects).
- All of Anytype blocks are supported.
- Anytype queries/collections are converted into Obsidian Bases (`.base`) files.
- Collection Bases are scoped by `createdInContext` membership when available.
- Integration with Pretty Properties and Iconize obsidian plugins. Compatible properties are converted and renamed.
- Select-like values (for example `tag`, `status`, `type`) can be exported as linked objects/notes when needed.
- Property visibility is configurable: include dynamic, archived, hidden, empty, or specific properties.

## Quick start

1. Export your Anytype Space in Any-Block format. Choose `JSON` as the file format and disale `Zip archive`
2. Get binary for your platform from [releases](https://github.com/sleroq/anytype-to-obsidian/releases) (skip if you can run using Nix)
3. Run it specifying location of exported space: `anytype-to-obsidian.exe -input ./Anytype-exported-json -output ./result-directory`

## Usage

Run in interactive mode (no arguments):

```bash
./anytype-to-obsidian
```

This opens a setup form and uses these defaults:

- input: `./Anytype-json`
- output: `./obsidian-vault`

Or run directly with flags:

```bash
/anytype-to-obsidian -input ./Anytype-exported-json -output ./result-directory
```

Run using Nix:

```bash
nix run github:sleroq/anytype-to-obsidian -- -input ./Anytype-exported-json -output ./result-directory
```

## Main options

- `-input`: path to `Anytype-json`.
- `-output`: output Obsidian vault path.
- `-prettier`: format exported markdown via `npx prettier` (`true` by default).
- `-filename-escaping`: `auto`, `posix`, or `windows`.
- `-include-dynamic-properties`: include system-managed Anytype fields.
- `-include-archived-objects`: include archived Anytype objects in export (notes and bases).
- `-include-archived-properties`: include unresolved/archived relation fields and include relation-option dataview objects in `bases/*.base` export.
- `-exclude-empty-properties`: drop empty frontmatter values.
- `-exclude-properties`: comma-separated property keys/names to exclude.
- `-force-include-properties`: comma-separated property keys/names to include even if hidden by default.
- `-link-as-note-properties`: comma-separated relation keys/names to export as note links (for example `type,tag,status`).
- `-disable-picture-to-cover`: keep the original `picture` property name instead of exporting it as `cover`.
- `-disable-pretty-properties-icon`: keep original `iconImage` / `iconEmoji` properties instead of exporting Pretty Properties-compatible `icon`.
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
