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

	flag.StringVar(&input, "input", "./Anytype-json", "Path to Anytype-json export directory")
	flag.StringVar(&output, "output", "./obsidian-vault", "Path to output Obsidian vault")
	flag.Parse()

	exp := exporter.Exporter{
		InputDir:  input,
		OutputDir: output,
	}

	stats, err := exp.Run()
	if err != nil {
		fmt.Fprintf(os.Stderr, "export failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("exported %d notes, copied %d files\n", stats.Notes, stats.Files)
}
