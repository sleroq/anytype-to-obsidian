package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/sleroq/anytype-to-obsidian/internal/app/exporter"
)

func main() {
	var input string
	var output string
	var filenameEscaping string
	var includeDynamicProperties bool
	var includeArchivedProperties bool
	var excludeEmptyProperties bool
	var excludeProperties string
	var includeProperties string
	var linkAsNoteProperties string

	flag.StringVar(&input, "input", "./Anytype-json", "Path to Anytype-json export directory")
	flag.StringVar(&output, "output", "./obsidian-vault", "Path to output Obsidian vault")
	flag.StringVar(&filenameEscaping, "filename-escaping", "auto", "Filename escaping mode: auto, posix, windows")
	flag.BoolVar(&includeDynamicProperties, "include-dynamic-properties", false, "Include dynamic/system-managed Anytype properties (e.g. backlinks, lastModifiedDate)")
	flag.BoolVar(&includeArchivedProperties, "include-archived-properties", false, "Include archived/unresolved Anytype relation properties that have no readable relation name")
	flag.BoolVar(&excludeEmptyProperties, "exclude-empty-properties", false, "Exclude frontmatter properties with empty values (nil, empty strings, empty arrays, empty objects)")
	flag.StringVar(&excludeProperties, "exclude-properties", "", "Comma-separated property keys/names to always exclude from frontmatter")
	flag.StringVar(&includeProperties, "force-include-properties", "", "Comma-separated property keys/names to always include in frontmatter")
	flag.StringVar(&linkAsNoteProperties, "link-as-note-properties", "", "Comma-separated property keys/names to render relation values as note links when possible (e.g. type,tag,status)")
	flag.Parse()

	exp := exporter.Exporter{
		InputDir:                  input,
		OutputDir:                 output,
		FilenameEscaping:          filenameEscaping,
		IncludeDynamicProperties:  includeDynamicProperties,
		IncludeArchivedProperties: includeArchivedProperties,
		ExcludeEmptyProperties:    excludeEmptyProperties,
		ExcludePropertyKeys:       parseCommaSeparatedList(excludeProperties),
		ForceIncludePropertyKeys:  parseCommaSeparatedList(includeProperties),
		LinkAsNotePropertyKeys:    parseCommaSeparatedList(linkAsNoteProperties),
	}

	stats, err := exp.Run()
	if err != nil {
		fmt.Fprintf(os.Stderr, "export failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("exported %d notes, copied %d files\n", stats.Notes, stats.Files)
}

func parseCommaSeparatedList(value string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed == "" {
			continue
		}
		out = append(out, trimmed)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
