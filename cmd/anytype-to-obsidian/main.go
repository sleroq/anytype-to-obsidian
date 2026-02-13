package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/sleroq/anytype-to-obsidian/internal/app/exporter"
)

func main() {
	var input string
	var output string
	var filenameEscaping string
	var includeDynamicProperties bool
	var includeArchivedProperties bool

	flag.StringVar(&input, "input", "./Anytype-json", "Path to Anytype-json export directory")
	flag.StringVar(&output, "output", "./obsidian-vault", "Path to output Obsidian vault")
	flag.StringVar(&filenameEscaping, "filename-escaping", "auto", "Filename escaping mode: auto, posix, windows")
	flag.BoolVar(&includeDynamicProperties, "include-dynamic-properties", false, "Include dynamic/system-managed Anytype properties (e.g. backlinks, lastModifiedDate)")
	flag.BoolVar(&includeArchivedProperties, "include-archived-properties", false, "Include archived/unresolved Anytype relation properties that have no readable relation name")
	flag.Parse()

	exp := exporter.Exporter{
		InputDir:                  input,
		OutputDir:                 output,
		FilenameEscaping:          filenameEscaping,
		IncludeDynamicProperties:  includeDynamicProperties,
		IncludeArchivedProperties: includeArchivedProperties,
	}

	stats, err := exp.Run()
	if err != nil {
		fmt.Fprintf(os.Stderr, "export failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("exported %d notes, copied %d files\n", stats.Notes, stats.Files)
}
