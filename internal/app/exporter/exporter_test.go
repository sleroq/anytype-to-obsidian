package exporter

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExporterPreservesRelationsAndFields(t *testing.T) {
	root := t.TempDir()
	input := filepath.Join(root, "Anytype-json")
	output := filepath.Join(root, "vault")

	mustMkdirAll(t, filepath.Join(input, "objects"))
	mustMkdirAll(t, filepath.Join(input, "relations"))
	mustMkdirAll(t, filepath.Join(input, "relationsOptions"))
	mustMkdirAll(t, filepath.Join(input, "filesObjects"))
	mustMkdirAll(t, filepath.Join(input, "files"))

	writePBJSON(t, filepath.Join(input, "relations", "rel-related.pb.json"), "STRelation", map[string]any{
		"id":             "rel-related",
		"relationKey":    "related",
		"relationFormat": 100,
		"name":           "Related",
	}, nil)
	writePBJSON(t, filepath.Join(input, "relations", "rel-status.pb.json"), "STRelation", map[string]any{
		"id":             "rel-status",
		"relationKey":    "status",
		"relationFormat": 3,
		"name":           "Status",
	}, nil)
	writePBJSON(t, filepath.Join(input, "relations", "rel-tag.pb.json"), "STRelation", map[string]any{
		"id":             "rel-tag",
		"relationKey":    "tag",
		"relationFormat": 11,
		"name":           "Tag",
	}, nil)
	writePBJSON(t, filepath.Join(input, "relations", "rel-backlinks.pb.json"), "STRelation", map[string]any{
		"id":             "rel-backlinks",
		"relationKey":    "backlinks",
		"relationFormat": 100,
		"name":           "Backlinks",
	}, nil)
	writePBJSON(t, filepath.Join(input, "relations", "rel-task-type.pb.json"), "STRelation", map[string]any{
		"id":             "bafyreihowvwq6jmco67ilpwej23jopfic3stteazzbdonl7bvfkfdbk2de",
		"relationKey":    "65edf2aa8efc1e005b0cb9d2",
		"relationFormat": 3,
		"name":           "Task Type",
	}, nil)
	writePBJSON(t, filepath.Join(input, "relations", "rel-last-modified-date.pb.json"), "STRelation", map[string]any{
		"id":             "rel-last-modified-date",
		"relationKey":    "lastModifiedDate",
		"relationFormat": 4,
		"name":           "Last Modified Date",
	}, nil)

	writePBJSON(t, filepath.Join(input, "relationsOptions", "opt-status.pb.json"), "STRelationOption", map[string]any{
		"id":   "opt-status-doing",
		"name": "Doing",
	}, nil)
	writePBJSON(t, filepath.Join(input, "relationsOptions", "opt-tag-1.pb.json"), "STRelationOption", map[string]any{
		"id":   "opt-tag-go",
		"name": "go",
	}, nil)
	writePBJSON(t, filepath.Join(input, "relationsOptions", "opt-tag-2.pb.json"), "STRelationOption", map[string]any{
		"id":   "opt-tag-export",
		"name": "export",
	}, nil)
	writePBJSON(t, filepath.Join(input, "relationsOptions", "opt-task-type.pb.json"), "STRelationOption", map[string]any{
		"id":   "opt-task-type-bug",
		"name": "Bug",
	}, nil)

	writePBJSON(t, filepath.Join(input, "objects", "obj-2.pb.json"), "Page", map[string]any{
		"id":   "obj-2",
		"name": "Task Two",
	}, []map[string]any{
		{"id": "obj-2", "childrenIds": []string{"title"}},
		{"id": "title", "text": map[string]any{"text": "Task Two", "style": "Title"}},
	})

	writePBJSON(t, filepath.Join(input, "objects", "obj-1.pb.json"), "Page", map[string]any{
		"id":                       "obj-1",
		"name":                     "Task One",
		"author":                   "john",
		"layout":                   "note",
		"internalFlags":            []any{"flag-a"},
		"sourceObject":             "obj-3",
		"spaceId":                  "space-1",
		"importType":               "local",
		"createdDate":              1720000000,
		"featuredRelations":        []any{"status"},
		"related":                  []any{"obj-2"},
		"status":                   []any{"opt-status-doing"},
		"65edf2aa8efc1e005b0cb9d2": []any{"opt-task-type-bug"},
		"abcdefabcdefabcdefabcdef": []any{"opt-status-doing"},
		"tag":                      []any{"opt-tag-go", "opt-tag-export"},
		"backlinks":                []any{"obj-2"},
		"rel-backlinks":            []any{"obj-2"},
		"lastModifiedDate":         1730000000,
		"rel-last-modified-date":   1730000000,
	}, []map[string]any{
		{"id": "obj-1", "childrenIds": []string{"title"}},
		{"id": "title", "text": map[string]any{"text": "Task One", "style": "Title"}},
	})

	stats, err := (Exporter{InputDir: input, OutputDir: output}).Run()
	if err != nil {
		t.Fatalf("run exporter: %v", err)
	}
	if stats.Notes != 2 {
		t.Fatalf("expected 2 notes, got %d", stats.Notes)
	}

	noteBytes, err := os.ReadFile(filepath.Join(output, "notes", "Task One.md"))
	if err != nil {
		t.Fatalf("read note: %v", err)
	}
	note := string(noteBytes)
	if !strings.Contains(note, "related: \"[[notes/Task Two.md]]\"") {
		t.Fatalf("expected related object relation to be rendered, got:\n%s", note)
	}
	if !strings.Contains(note, "status: \"Doing\"") {
		t.Fatalf("expected status option to be resolved, got:\n%s", note)
	}
	if !strings.Contains(note, "Task Type: \"Bug\"") {
		t.Fatalf("expected relation-id property to render readable key and option value, got:\n%s", note)
	}
	if !strings.Contains(note, "tag:") || !strings.Contains(note, "- \"go\"") || !strings.Contains(note, "- \"export\"") {
		t.Fatalf("expected multi tag values, got:\n%s", note)
	}
	if strings.Contains(note, "abcdefabcdefabcdefabcdef:") {
		t.Fatalf("expected unresolved archived-style property to be excluded by default, got:\n%s", note)
	}
	if strings.Contains(note, "backlinks:") {
		t.Fatalf("expected backlinks to be excluded by default, got:\n%s", note)
	}
	if strings.Contains(note, "Backlinks:") {
		t.Fatalf("expected relation-id backlinks to be excluded by default, got:\n%s", note)
	}
	if strings.Contains(note, "lastModifiedDate:") {
		t.Fatalf("expected lastModifiedDate to be excluded by default, got:\n%s", note)
	}
	if strings.Contains(note, "Last Modified Date:") {
		t.Fatalf("expected relation-id lastModifiedDate to be excluded by default, got:\n%s", note)
	}
	if strings.Contains(note, "anytype_id:") {
		t.Fatalf("expected anytype_id to be excluded by default, got:\n%s", note)
	}
	if !strings.Contains(note, "author: \"john\"") {
		t.Fatalf("expected author to be included by default, got:\n%s", note)
	}
	for _, hiddenKey := range []string{
		"layout:",
		"internalFlags:",
		"sourceObject:",
		"spaceId:",
		"importType:",
		"createdDate:",
		"featuredRelations:",
		"id:",
	} {
		if strings.Contains(note, hiddenKey) {
			t.Fatalf("expected %s to be excluded by default, got:\n%s", hiddenKey, note)
		}
	}

	if _, err := os.Stat(filepath.Join(output, "_anytype", "raw", "obj-1.json")); err != nil {
		t.Fatalf("expected raw sidecar: %v", err)
	}
}

func TestExporterIncludesArchivedPropertiesWhenEnabled(t *testing.T) {
	root := t.TempDir()
	input := filepath.Join(root, "Anytype-json")
	output := filepath.Join(root, "vault")

	mustMkdirAll(t, filepath.Join(input, "objects"))
	mustMkdirAll(t, filepath.Join(input, "relations"))
	mustMkdirAll(t, filepath.Join(input, "relationsOptions"))
	mustMkdirAll(t, filepath.Join(input, "filesObjects"))
	mustMkdirAll(t, filepath.Join(input, "files"))

	writePBJSON(t, filepath.Join(input, "objects", "obj-1.pb.json"), "Page", map[string]any{
		"id":                       "obj-1",
		"name":                     "Task One",
		"abcdefabcdefabcdefabcdef": []any{"opt-status-doing"},
	}, []map[string]any{
		{"id": "obj-1", "childrenIds": []string{"title"}},
		{"id": "title", "text": map[string]any{"text": "Task One", "style": "Title"}},
	})

	_, err := (Exporter{InputDir: input, OutputDir: output, IncludeArchivedProperties: true}).Run()
	if err != nil {
		t.Fatalf("run exporter: %v", err)
	}

	noteBytes, err := os.ReadFile(filepath.Join(output, "notes", "Task One.md"))
	if err != nil {
		t.Fatalf("read note: %v", err)
	}
	note := string(noteBytes)
	if !strings.Contains(note, "abcdefabcdefabcdefabcdef:") {
		t.Fatalf("expected unresolved archived-style property to be included when enabled, got:\n%s", note)
	}
}

func TestExporterIncludesDynamicPropertiesWhenEnabled(t *testing.T) {
	root := t.TempDir()
	input := filepath.Join(root, "Anytype-json")
	output := filepath.Join(root, "vault")

	mustMkdirAll(t, filepath.Join(input, "objects"))
	mustMkdirAll(t, filepath.Join(input, "relations"))
	mustMkdirAll(t, filepath.Join(input, "relationsOptions"))
	mustMkdirAll(t, filepath.Join(input, "filesObjects"))
	mustMkdirAll(t, filepath.Join(input, "files"))

	writePBJSON(t, filepath.Join(input, "relations", "rel-backlinks.pb.json"), "STRelation", map[string]any{
		"id":             "rel-backlinks",
		"relationKey":    "backlinks",
		"relationFormat": 100,
		"name":           "Backlinks",
	}, nil)

	writePBJSON(t, filepath.Join(input, "objects", "obj-1.pb.json"), "Page", map[string]any{
		"id":        "obj-1",
		"name":      "Task One",
		"backlinks": []any{"obj-2"},
	}, []map[string]any{
		{"id": "obj-1", "childrenIds": []string{"title"}},
		{"id": "title", "text": map[string]any{"text": "Task One", "style": "Title"}},
	})

	_, err := (Exporter{InputDir: input, OutputDir: output, IncludeDynamicProperties: true}).Run()
	if err != nil {
		t.Fatalf("run exporter: %v", err)
	}

	noteBytes, err := os.ReadFile(filepath.Join(output, "notes", "Task One.md"))
	if err != nil {
		t.Fatalf("read note: %v", err)
	}
	note := string(noteBytes)
	if !strings.Contains(note, "backlinks: \"obj-2\"") {
		t.Fatalf("expected backlinks to be included when enabled, got:\n%s", note)
	}
}

func TestExporterSupportsPropertyIncludeExcludeOverrides(t *testing.T) {
	root := t.TempDir()
	input := filepath.Join(root, "Anytype-json")
	output := filepath.Join(root, "vault")

	mustMkdirAll(t, filepath.Join(input, "objects"))
	mustMkdirAll(t, filepath.Join(input, "relations"))
	mustMkdirAll(t, filepath.Join(input, "relationsOptions"))
	mustMkdirAll(t, filepath.Join(input, "filesObjects"))
	mustMkdirAll(t, filepath.Join(input, "files"))

	writePBJSON(t, filepath.Join(input, "relations", "rel-backlinks.pb.json"), "STRelation", map[string]any{
		"id":             "rel-backlinks",
		"relationKey":    "backlinks",
		"relationFormat": 100,
		"name":           "Backlinks",
	}, nil)

	writePBJSON(t, filepath.Join(input, "objects", "obj-2.pb.json"), "Page", map[string]any{
		"id":   "obj-2",
		"name": "Task Two",
	}, []map[string]any{
		{"id": "obj-2", "childrenIds": []string{"title"}},
		{"id": "title", "text": map[string]any{"text": "Task Two", "style": "Title"}},
	})

	writePBJSON(t, filepath.Join(input, "objects", "obj-1.pb.json"), "Page", map[string]any{
		"id":        "obj-1",
		"name":      "Task One",
		"layout":    "note",
		"backlinks": []any{"obj-2"},
		"custom":    "hidden",
	}, []map[string]any{
		{"id": "obj-1", "childrenIds": []string{"title"}},
		{"id": "title", "text": map[string]any{"text": "Task One", "style": "Title"}},
	})

	_, err := (Exporter{
		InputDir:                 input,
		OutputDir:                output,
		ExcludePropertyKeys:      []string{"custom", "backlinks"},
		ForceIncludePropertyKeys: []string{"anytype_id", "layout", "backlinks"},
	}).Run()
	if err != nil {
		t.Fatalf("run exporter: %v", err)
	}

	noteBytes, err := os.ReadFile(filepath.Join(output, "notes", "Task One.md"))
	if err != nil {
		t.Fatalf("read note: %v", err)
	}
	note := string(noteBytes)
	if !strings.Contains(note, "anytype_id: \"obj-1\"") {
		t.Fatalf("expected anytype_id to be force-included, got:\n%s", note)
	}
	if !strings.Contains(note, "layout: \"note\"") {
		t.Fatalf("expected layout to be force-included, got:\n%s", note)
	}
	if !strings.Contains(note, "backlinks: \"[[notes/Task Two.md]]\"") {
		t.Fatalf("expected force-include to win over exclude for backlinks, got:\n%s", note)
	}
	if strings.Contains(note, "custom:") {
		t.Fatalf("expected custom to be excluded, got:\n%s", note)
	}
}

func TestExporterExcludesEmptyPropertiesWhenEnabled(t *testing.T) {
	root := t.TempDir()
	input := filepath.Join(root, "Anytype-json")
	outputWithEmpty := filepath.Join(root, "vault-with-empty")
	outputWithoutEmpty := filepath.Join(root, "vault-without-empty")

	mustMkdirAll(t, filepath.Join(input, "objects"))
	mustMkdirAll(t, filepath.Join(input, "relations"))
	mustMkdirAll(t, filepath.Join(input, "relationsOptions"))
	mustMkdirAll(t, filepath.Join(input, "filesObjects"))
	mustMkdirAll(t, filepath.Join(input, "files"))

	writePBJSON(t, filepath.Join(input, "objects", "obj-1.pb.json"), "Page", map[string]any{
		"id":          "obj-1",
		"name":        "Task One",
		"emptyString": "",
		"spaceOnly":   "   ",
		"emptyList":   []any{},
		"emptyMap":    map[string]any{},
		"nullField":   nil,
		"nonEmpty":    "value",
	}, []map[string]any{
		{"id": "obj-1", "childrenIds": []string{"title"}},
		{"id": "title", "text": map[string]any{"text": "Task One", "style": "Title"}},
	})

	_, err := (Exporter{InputDir: input, OutputDir: outputWithEmpty}).Run()
	if err != nil {
		t.Fatalf("run exporter (default): %v", err)
	}

	noteWithEmptyBytes, err := os.ReadFile(filepath.Join(outputWithEmpty, "notes", "Task One.md"))
	if err != nil {
		t.Fatalf("read note (default): %v", err)
	}
	noteWithEmpty := string(noteWithEmptyBytes)
	for _, expected := range []string{
		"emptyString: \"\"",
		"spaceOnly: \"   \"",
		"emptyList: []",
		"emptyMap: \"{}\"",
		"nullField: null",
		"nonEmpty: \"value\"",
	} {
		if !strings.Contains(noteWithEmpty, expected) {
			t.Fatalf("expected %q in default export, got:\n%s", expected, noteWithEmpty)
		}
	}

	_, err = (Exporter{InputDir: input, OutputDir: outputWithoutEmpty, ExcludeEmptyProperties: true}).Run()
	if err != nil {
		t.Fatalf("run exporter (exclude-empty): %v", err)
	}

	noteWithoutEmptyBytes, err := os.ReadFile(filepath.Join(outputWithoutEmpty, "notes", "Task One.md"))
	if err != nil {
		t.Fatalf("read note (exclude-empty): %v", err)
	}
	noteWithoutEmpty := string(noteWithoutEmptyBytes)
	for _, unexpected := range []string{
		"emptyString:",
		"spaceOnly:",
		"emptyList:",
		"emptyMap:",
		"nullField:",
	} {
		if strings.Contains(noteWithoutEmpty, unexpected) {
			t.Fatalf("did not expect %q when exclude-empty is enabled, got:\n%s", unexpected, noteWithoutEmpty)
		}
	}
	if !strings.Contains(noteWithoutEmpty, "nonEmpty: \"value\"") {
		t.Fatalf("expected non-empty property to remain, got:\n%s", noteWithoutEmpty)
	}
}

func TestExporterRendersTableAndFileBookmark(t *testing.T) {
	root := t.TempDir()
	input := filepath.Join(root, "Anytype-json")
	output := filepath.Join(root, "vault")

	mustMkdirAll(t, filepath.Join(input, "objects"))
	mustMkdirAll(t, filepath.Join(input, "relations"))
	mustMkdirAll(t, filepath.Join(input, "relationsOptions"))
	mustMkdirAll(t, filepath.Join(input, "filesObjects"))
	mustMkdirAll(t, filepath.Join(input, "files"))

	if err := os.WriteFile(filepath.Join(input, "files", "sample.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	writePBJSON(t, filepath.Join(input, "filesObjects", "file-1.pb.json"), "FileObject", map[string]any{
		"id":      "file-1",
		"name":    "sample",
		"fileExt": "txt",
		"source":  "files/sample.txt",
	}, nil)

	writePBJSON(t, filepath.Join(input, "objects", "table-page.pb.json"), "Page", map[string]any{
		"id":   "table-page",
		"name": "Table Page",
	}, []map[string]any{
		{"id": "table-page", "childrenIds": []string{"title", "table-1", "file-block", "bookmark-block"}},
		{"id": "title", "text": map[string]any{"text": "Table Page", "style": "Title"}},
		{"id": "table-1", "table": map[string]any{}, "childrenIds": []string{"table-cols", "table-rows"}},
		{"id": "table-cols", "layout": map[string]any{"style": "TableColumns"}, "childrenIds": []string{"col-1", "col-2"}},
		{"id": "table-rows", "layout": map[string]any{"style": "TableRows"}, "childrenIds": []string{"row-1", "row-2"}},
		{"id": "row-1", "childrenIds": []string{"cell-1-1", "cell-1-2"}},
		{"id": "row-2", "childrenIds": []string{"cell-2-1", "cell-2-2"}},
		{"id": "cell-1-1", "childrenIds": []string{"cell-1-1-text"}},
		{"id": "cell-1-1-text", "text": map[string]any{"text": "h1", "style": "Paragraph"}},
		{"id": "cell-1-2", "childrenIds": []string{"cell-1-2-text"}},
		{"id": "cell-1-2-text", "text": map[string]any{"text": "h2", "style": "Paragraph"}},
		{"id": "cell-2-1", "childrenIds": []string{"cell-2-1-text"}},
		{"id": "cell-2-1-text", "text": map[string]any{"text": "v1", "style": "Paragraph"}},
		{"id": "cell-2-2", "childrenIds": []string{"cell-2-2-text"}},
		{"id": "cell-2-2-text", "text": map[string]any{"text": "v2", "style": "Paragraph"}},
		{"id": "file-block", "file": map[string]any{"name": "sample.txt", "type": "File", "targetObjectId": "file-1"}},
		{"id": "bookmark-block", "bookmark": map[string]any{"url": "https://example.com", "title": "Example"}},
	})

	_, err := (Exporter{InputDir: input, OutputDir: output}).Run()
	if err != nil {
		t.Fatalf("run exporter: %v", err)
	}

	noteBytes, err := os.ReadFile(filepath.Join(output, "notes", "Table Page.md"))
	if err != nil {
		t.Fatalf("read note: %v", err)
	}
	note := string(noteBytes)
	if !strings.Contains(note, "| h1 | h2 |") || !strings.Contains(note, "| v1 | v2 |") {
		t.Fatalf("expected markdown table, got:\n%s", note)
	}
	if !strings.Contains(note, "[sample.txt](files/sample.txt)") {
		t.Fatalf("expected file link, got:\n%s", note)
	}
	if !strings.Contains(note, "[Example](https://example.com)") {
		t.Fatalf("expected bookmark link, got:\n%s", note)
	}

	if _, err := os.Stat(filepath.Join(output, "files", "sample.txt")); err != nil {
		t.Fatalf("expected copied file: %v", err)
	}
}

func TestExporterUsesAnytypeNameForNoteFileNameAndHandlesCollisions(t *testing.T) {
	root := t.TempDir()
	input := filepath.Join(root, "Anytype-json")
	output := filepath.Join(root, "vault")

	mustMkdirAll(t, filepath.Join(input, "objects"))
	mustMkdirAll(t, filepath.Join(input, "relations"))
	mustMkdirAll(t, filepath.Join(input, "relationsOptions"))
	mustMkdirAll(t, filepath.Join(input, "filesObjects"))
	mustMkdirAll(t, filepath.Join(input, "files"))

	writePBJSON(t, filepath.Join(input, "objects", "obj-1.pb.json"), "Page", map[string]any{
		"id":   "obj-1",
		"name": "Readable Name",
	}, []map[string]any{
		{"id": "obj-1", "childrenIds": []string{"title-1"}},
		{"id": "title-1", "text": map[string]any{"text": "Displayed Title One", "style": "Title"}},
	})

	writePBJSON(t, filepath.Join(input, "objects", "obj-2.pb.json"), "Page", map[string]any{
		"id":   "obj-2",
		"name": "Readable Name",
	}, []map[string]any{
		{"id": "obj-2", "childrenIds": []string{"title-2"}},
		{"id": "title-2", "text": map[string]any{"text": "Displayed Title Two", "style": "Title"}},
	})

	_, err := (Exporter{InputDir: input, OutputDir: output}).Run()
	if err != nil {
		t.Fatalf("run exporter: %v", err)
	}

	if _, err := os.Stat(filepath.Join(output, "notes", "Readable Name.md")); err != nil {
		t.Fatalf("expected first name-based note file, got error: %v", err)
	}
	if _, err := os.Stat(filepath.Join(output, "notes", "Readable Name-2.md")); err != nil {
		t.Fatalf("expected collision-safe second name-based note file, got error: %v", err)
	}

	indexBytes, err := os.ReadFile(filepath.Join(output, "_anytype", "index.json"))
	if err != nil {
		t.Fatalf("read index: %v", err)
	}

	var idx indexFile
	if err := json.Unmarshal(indexBytes, &idx); err != nil {
		t.Fatalf("decode index: %v", err)
	}

	if got := idx.Notes["obj-1"]; got != "notes/Readable Name.md" {
		t.Fatalf("unexpected note path for obj-1: %q", got)
	}
	if got := idx.Notes["obj-2"]; got != "notes/Readable Name-2.md" {
		t.Fatalf("unexpected note path for obj-2: %q", got)
	}
}

func TestExporterSupportsWindowsFilenameEscaping(t *testing.T) {
	root := t.TempDir()
	input := filepath.Join(root, "Anytype-json")
	output := filepath.Join(root, "vault")

	mustMkdirAll(t, filepath.Join(input, "objects"))
	mustMkdirAll(t, filepath.Join(input, "relations"))
	mustMkdirAll(t, filepath.Join(input, "relationsOptions"))
	mustMkdirAll(t, filepath.Join(input, "filesObjects"))
	mustMkdirAll(t, filepath.Join(input, "files"))

	writePBJSON(t, filepath.Join(input, "objects", "obj-1.pb.json"), "Page", map[string]any{
		"id":   "obj-1",
		"name": "CON",
	}, []map[string]any{
		{"id": "obj-1", "childrenIds": []string{"title-1"}},
		{"id": "title-1", "text": map[string]any{"text": "Ignored Title", "style": "Title"}},
	})

	writePBJSON(t, filepath.Join(input, "objects", "obj-2.pb.json"), "Page", map[string]any{
		"id":   "obj-2",
		"name": "a:b* c?",
	}, []map[string]any{
		{"id": "obj-2", "childrenIds": []string{"title-2"}},
		{"id": "title-2", "text": map[string]any{"text": "Ignored Title", "style": "Title"}},
	})

	_, err := (Exporter{InputDir: input, OutputDir: output, FilenameEscaping: "windows"}).Run()
	if err != nil {
		t.Fatalf("run exporter: %v", err)
	}

	if _, err := os.Stat(filepath.Join(output, "notes", "CON-file.md")); err != nil {
		t.Fatalf("expected windows-safe reserved-name file, got error: %v", err)
	}
	if _, err := os.Stat(filepath.Join(output, "notes", "a-b- c-.md")); err != nil {
		t.Fatalf("expected windows-safe escaped file, got error: %v", err)
	}
}

func TestExporterResolvesTypeRelationFromTypesDirectory(t *testing.T) {
	root := t.TempDir()
	input := filepath.Join(root, "Anytype-json")
	output := filepath.Join(root, "vault")

	mustMkdirAll(t, filepath.Join(input, "objects"))
	mustMkdirAll(t, filepath.Join(input, "relations"))
	mustMkdirAll(t, filepath.Join(input, "relationsOptions"))
	mustMkdirAll(t, filepath.Join(input, "filesObjects"))
	mustMkdirAll(t, filepath.Join(input, "files"))
	mustMkdirAll(t, filepath.Join(input, "types"))

	writePBJSON(t, filepath.Join(input, "relations", "rel-type.pb.json"), "STRelation", map[string]any{
		"id":             "rel-type",
		"relationKey":    "type",
		"relationFormat": 100,
		"name":           "type",
	}, nil)

	typeID := "bafyreiaxyq4jrnqouh5ohxikp4tpy2fzkgkrb47kdxwtynfwcrckvg2jti"
	writePBJSON(t, filepath.Join(input, "types", typeID+".pb.json"), "STType", map[string]any{
		"id":   typeID,
		"name": "Human",
	}, nil)

	writePBJSON(t, filepath.Join(input, "objects", "obj-1.pb.json"), "Page", map[string]any{
		"id":   "obj-1",
		"name": "Dan Brown",
		"type": typeID,
	}, []map[string]any{
		{"id": "obj-1", "childrenIds": []string{"title"}},
		{"id": "title", "text": map[string]any{"text": "Dan Brown", "style": "Title"}},
	})

	_, err := (Exporter{InputDir: input, OutputDir: output}).Run()
	if err != nil {
		t.Fatalf("run exporter: %v", err)
	}

	noteBytes, err := os.ReadFile(filepath.Join(output, "notes", "Dan Brown.md"))
	if err != nil {
		t.Fatalf("read note: %v", err)
	}
	note := string(noteBytes)
	if !strings.Contains(note, "type: \"Human\"") {
		t.Fatalf("expected type relation to be resolved from types directory, got:\n%s", note)
	}
}

func TestExporterCanLinkTypePropertyAsNoteAndCreatesTypeNote(t *testing.T) {
	root := t.TempDir()
	input := filepath.Join(root, "Anytype-json")
	output := filepath.Join(root, "vault")

	mustMkdirAll(t, filepath.Join(input, "objects"))
	mustMkdirAll(t, filepath.Join(input, "relations"))
	mustMkdirAll(t, filepath.Join(input, "relationsOptions"))
	mustMkdirAll(t, filepath.Join(input, "filesObjects"))
	mustMkdirAll(t, filepath.Join(input, "files"))
	mustMkdirAll(t, filepath.Join(input, "types"))

	writePBJSON(t, filepath.Join(input, "relations", "rel-type.pb.json"), "STRelation", map[string]any{
		"id":             "rel-type",
		"relationKey":    "type",
		"relationFormat": 100,
		"name":           "type",
	}, nil)

	typeID := "type-human"
	writePBJSON(t, filepath.Join(input, "types", typeID+".pb.json"), "STType", map[string]any{
		"id":                           typeID,
		"name":                         "Human",
		"pluralName":                   "Humans",
		"recommendedRelations":         []string{"rel-contact"},
		"recommendedHiddenRelations":   []string{"rel-last-modified-date"},
		"recommendedFeaturedRelations": []string{},
		"recommendedFileRelations":     []string{},
	}, []map[string]any{
		{"id": typeID, "childrenIds": []string{"title"}},
		{"id": "title", "text": map[string]any{"text": "Human", "style": "Title"}},
	})

	writePBJSON(t, filepath.Join(input, "objects", "obj-1.pb.json"), "Page", map[string]any{
		"id":   "obj-1",
		"name": "Dan Brown",
		"type": typeID,
	}, []map[string]any{
		{"id": "obj-1", "childrenIds": []string{"title"}},
		{"id": "title", "text": map[string]any{"text": "Dan Brown", "style": "Title"}},
	})

	stats, err := (Exporter{InputDir: input, OutputDir: output, LinkAsNotePropertyKeys: []string{"type"}}).Run()
	if err != nil {
		t.Fatalf("run exporter: %v", err)
	}
	if stats.Notes != 2 {
		t.Fatalf("expected object and synthetic type note, got %d", stats.Notes)
	}

	personNoteBytes, err := os.ReadFile(filepath.Join(output, "notes", "Dan Brown.md"))
	if err != nil {
		t.Fatalf("read person note: %v", err)
	}
	personNote := string(personNoteBytes)
	if !strings.Contains(personNote, "type: \"[[notes/Human.md]]\"") {
		t.Fatalf("expected type property to be rendered as note link, got:\n%s", personNote)
	}

	typeNoteBytes, err := os.ReadFile(filepath.Join(output, "notes", "Human.md"))
	if err != nil {
		t.Fatalf("read type note: %v", err)
	}
	typeNote := string(typeNoteBytes)
	if !strings.Contains(typeNote, "pluralName: \"Humans\"") {
		t.Fatalf("expected synthetic type note to include useful type data, got:\n%s", typeNote)
	}
}

func TestExporterCanLinkTagPropertyAsNoteAndCreatesOptionNote(t *testing.T) {
	root := t.TempDir()
	input := filepath.Join(root, "Anytype-json")
	output := filepath.Join(root, "vault")

	mustMkdirAll(t, filepath.Join(input, "objects"))
	mustMkdirAll(t, filepath.Join(input, "relations"))
	mustMkdirAll(t, filepath.Join(input, "relationsOptions"))
	mustMkdirAll(t, filepath.Join(input, "filesObjects"))
	mustMkdirAll(t, filepath.Join(input, "files"))

	writePBJSON(t, filepath.Join(input, "relations", "rel-tag.pb.json"), "STRelation", map[string]any{
		"id":             "rel-tag",
		"relationKey":    "tag",
		"relationFormat": 11,
		"name":           "Tag",
	}, nil)

	writePBJSON(t, filepath.Join(input, "relationsOptions", "opt-tag-1.pb.json"), "STRelationOption", map[string]any{
		"id":   "opt-tag-go",
		"name": "go",
	}, nil)

	writePBJSON(t, filepath.Join(input, "objects", "obj-1.pb.json"), "Page", map[string]any{
		"id":   "obj-1",
		"name": "Tagged Page",
		"tag":  []any{"opt-tag-go"},
	}, []map[string]any{
		{"id": "obj-1", "childrenIds": []string{"title"}},
		{"id": "title", "text": map[string]any{"text": "Tagged Page", "style": "Title"}},
	})

	stats, err := (Exporter{InputDir: input, OutputDir: output, LinkAsNotePropertyKeys: []string{"tag"}}).Run()
	if err != nil {
		t.Fatalf("run exporter: %v", err)
	}
	if stats.Notes != 2 {
		t.Fatalf("expected object and synthetic tag option note, got %d", stats.Notes)
	}

	noteBytes, err := os.ReadFile(filepath.Join(output, "notes", "Tagged Page.md"))
	if err != nil {
		t.Fatalf("read page note: %v", err)
	}
	note := string(noteBytes)
	if !strings.Contains(note, "tag: \"[[notes/go.md]]\"") {
		t.Fatalf("expected tag property to be rendered as note link, got:\n%s", note)
	}

	if _, err := os.Stat(filepath.Join(output, "notes", "go.md")); err != nil {
		t.Fatalf("expected synthetic tag option note to exist: %v", err)
	}
}

func TestExporterOrdersTypePropertiesAndExcludesDynamicTypeHiddenByDefault(t *testing.T) {
	root := t.TempDir()
	input := filepath.Join(root, "Anytype-json")
	outputDefault := filepath.Join(root, "vault-default")
	outputDynamic := filepath.Join(root, "vault-dynamic")

	mustMkdirAll(t, filepath.Join(input, "objects"))
	mustMkdirAll(t, filepath.Join(input, "relations"))
	mustMkdirAll(t, filepath.Join(input, "relationsOptions"))
	mustMkdirAll(t, filepath.Join(input, "filesObjects"))
	mustMkdirAll(t, filepath.Join(input, "files"))
	mustMkdirAll(t, filepath.Join(input, "types"))

	writePBJSON(t, filepath.Join(input, "relations", "rel-contact.pb.json"), "STRelation", map[string]any{
		"id":             "rel-contact",
		"relationKey":    "contact",
		"relationFormat": 1,
		"name":           "Contact",
	}, nil)
	writePBJSON(t, filepath.Join(input, "relations", "rel-last-modified-date.pb.json"), "STRelation", map[string]any{
		"id":             "rel-last-modified-date",
		"relationKey":    "lastModifiedDate",
		"relationFormat": 4,
		"name":           "Last Modified Date",
	}, nil)

	typeID := "type-human"
	writePBJSON(t, filepath.Join(input, "types", typeID+".pb.json"), "STType", map[string]any{
		"id":                           typeID,
		"name":                         "Human",
		"recommendedRelations":         []string{"rel-contact"},
		"recommendedHiddenRelations":   []string{"rel-last-modified-date"},
		"recommendedFeaturedRelations": []string{},
		"recommendedFileRelations":     []string{},
	}, nil)

	writePBJSON(t, filepath.Join(input, "objects", "obj-1.pb.json"), "Page", map[string]any{
		"id":               "obj-1",
		"name":             "John",
		"type":             typeID,
		"contact":          "john@example.com",
		"lastModifiedDate": 1700000000,
		"customExtra":      "keep",
	}, []map[string]any{
		{"id": "obj-1", "childrenIds": []string{"title"}},
		{"id": "title", "text": map[string]any{"text": "John", "style": "Title"}},
	})

	_, err := (Exporter{InputDir: input, OutputDir: outputDefault}).Run()
	if err != nil {
		t.Fatalf("run exporter: %v", err)
	}

	noteBytes, err := os.ReadFile(filepath.Join(outputDefault, "notes", "John.md"))
	if err != nil {
		t.Fatalf("read note: %v", err)
	}
	note := string(noteBytes)

	if strings.Contains(note, "lastModifiedDate:") {
		t.Fatalf("expected dynamic type-hidden lastModifiedDate to be excluded by default, got:\n%s", note)
	}

	contactIdx := strings.Index(note, "contact: \"john@example.com\"")
	extraIdx := strings.Index(note, "customExtra: \"keep\"")
	if contactIdx < 0 || extraIdx < 0 {
		t.Fatalf("expected ordered properties to exist, got:\n%s", note)
	}
	if !(contactIdx < extraIdx) {
		t.Fatalf("expected type visible then non-type order, got:\n%s", note)
	}

	_, err = (Exporter{InputDir: input, OutputDir: outputDynamic, IncludeDynamicProperties: true}).Run()
	if err != nil {
		t.Fatalf("run exporter with dynamic properties: %v", err)
	}

	noteBytes, err = os.ReadFile(filepath.Join(outputDynamic, "notes", "John.md"))
	if err != nil {
		t.Fatalf("read note with dynamic properties: %v", err)
	}
	note = string(noteBytes)

	if !strings.Contains(note, "lastModifiedDate: \"2023-11-14\"") {
		t.Fatalf("expected dynamic type-hidden lastModifiedDate to be included when enabled, got:\n%s", note)
	}

	contactIdx = strings.Index(note, "contact: \"john@example.com\"")
	hiddenIdx := strings.Index(note, "lastModifiedDate: \"2023-11-14\"")
	extraIdx = strings.Index(note, "customExtra: \"keep\"")
	if !(contactIdx < hiddenIdx && hiddenIdx < extraIdx) {
		t.Fatalf("expected type visible then type hidden then non-type order when dynamic is enabled, got:\n%s", note)
	}
}

func TestExporterGeneratesTemplatesFromTemplateBlocks(t *testing.T) {
	root := t.TempDir()
	input := filepath.Join(root, "Anytype-json")
	output := filepath.Join(root, "vault")

	mustMkdirAll(t, filepath.Join(input, "objects"))
	mustMkdirAll(t, filepath.Join(input, "relations"))
	mustMkdirAll(t, filepath.Join(input, "relationsOptions"))
	mustMkdirAll(t, filepath.Join(input, "filesObjects"))
	mustMkdirAll(t, filepath.Join(input, "files"))
	mustMkdirAll(t, filepath.Join(input, "types"))
	mustMkdirAll(t, filepath.Join(input, "templates"))

	writePBJSON(t, filepath.Join(input, "relations", "rel-date-of-birth.pb.json"), "STRelation", map[string]any{
		"id":             "rel-date-of-birth",
		"relationKey":    "dateOfBirth",
		"relationFormat": 4,
		"name":           "Birthday",
	}, nil)
	writePBJSON(t, filepath.Join(input, "relations", "rel-custom.pb.json"), "STRelation", map[string]any{
		"id":             "rel-custom",
		"relationKey":    "65eddcbe8efc1e005b0cb88d",
		"relationFormat": 8,
		"name":           "Another Email",
	}, nil)

	typeID := "type-human"
	writePBJSON(t, filepath.Join(input, "types", typeID+".pb.json"), "STType", map[string]any{
		"id":   typeID,
		"name": "Human",
	}, nil)

	writePBJSON(t, filepath.Join(input, "templates", "tmpl-1.pb.json"), "Template", map[string]any{
		"id":               "tmpl-1",
		"name":             "Contact",
		"targetObjectType": typeID,
	}, []map[string]any{
		{"id": "tmpl-1", "childrenIds": []string{"title", "rel-a", "rel-a-dup", "rel-b", "body"}},
		{"id": "title", "text": map[string]any{"text": "Contact", "style": "Title"}},
		{"id": "rel-a", "relation": map[string]any{"key": "dateOfBirth"}},
		{"id": "rel-a-dup", "relation": map[string]any{"key": "dateOfBirth"}},
		{"id": "rel-b", "relation": map[string]any{"key": "65eddcbe8efc1e005b0cb88d"}},
		{"id": "body", "text": map[string]any{"text": "Template body", "style": "Paragraph"}},
	})

	_, err := (Exporter{InputDir: input, OutputDir: output}).Run()
	if err != nil {
		t.Fatalf("run exporter: %v", err)
	}

	templateBytes, err := os.ReadFile(filepath.Join(output, "templates", "Human - Contact.md"))
	if err != nil {
		t.Fatalf("read template: %v", err)
	}
	template := string(templateBytes)
	if !strings.Contains(template, "anytype_template_id: \"tmpl-1\"") {
		t.Fatalf("expected template id frontmatter, got:\n%s", template)
	}
	if !strings.Contains(template, "anytype_target_type: \"Human\"") {
		t.Fatalf("expected target type name frontmatter, got:\n%s", template)
	}
	if !strings.Contains(template, "dateOfBirth: null") {
		t.Fatalf("expected relation block field to be exported, got:\n%s", template)
	}
	if !strings.Contains(template, "Another Email: null") {
		t.Fatalf("expected custom relation block field to use relation name, got:\n%s", template)
	}
	if strings.Count(template, "dateOfBirth: null") != 1 {
		t.Fatalf("expected duplicate relation blocks to be deduplicated, got:\n%s", template)
	}
	if !strings.Contains(template, "Template body") {
		t.Fatalf("expected template body text to be rendered, got:\n%s", template)
	}
}

func TestExporterTemplateFileNamesAvoidIDsAndUseNumericSuffixes(t *testing.T) {
	root := t.TempDir()
	input := filepath.Join(root, "Anytype-json")
	output := filepath.Join(root, "vault")

	mustMkdirAll(t, filepath.Join(input, "objects"))
	mustMkdirAll(t, filepath.Join(input, "relations"))
	mustMkdirAll(t, filepath.Join(input, "relationsOptions"))
	mustMkdirAll(t, filepath.Join(input, "filesObjects"))
	mustMkdirAll(t, filepath.Join(input, "files"))
	mustMkdirAll(t, filepath.Join(input, "types"))
	mustMkdirAll(t, filepath.Join(input, "templates"))

	typeID := "type-human"
	writePBJSON(t, filepath.Join(input, "types", typeID+".pb.json"), "STType", map[string]any{
		"id":   typeID,
		"name": "Human",
	}, nil)

	writePBJSON(t, filepath.Join(input, "templates", "tmpl-alpha.pb.json"), "Template", map[string]any{
		"id":               "tmpl-alpha",
		"targetObjectType": typeID,
	}, []map[string]any{{"id": "tmpl-alpha", "childrenIds": []string{}}})

	writePBJSON(t, filepath.Join(input, "templates", "tmpl-beta.pb.json"), "Template", map[string]any{
		"id":               "tmpl-beta",
		"targetObjectType": typeID,
	}, []map[string]any{{"id": "tmpl-beta", "childrenIds": []string{}}})

	_, err := (Exporter{InputDir: input, OutputDir: output}).Run()
	if err != nil {
		t.Fatalf("run exporter: %v", err)
	}

	if _, err := os.Stat(filepath.Join(output, "templates", "Human - Template.md")); err != nil {
		t.Fatalf("expected first template file without id fallback, got: %v", err)
	}
	if _, err := os.Stat(filepath.Join(output, "templates", "Human - Template-2.md")); err != nil {
		t.Fatalf("expected collision suffix for second template, got: %v", err)
	}
	if _, err := os.Stat(filepath.Join(output, "templates", "Human - tmpl-alpha.md")); err == nil {
		t.Fatalf("did not expect id-based template filename")
	}
}

func TestConvertPropertyValueFormatsDateToDay(t *testing.T) {
	converted := convertPropertyValue(
		"dueDate",
		float64(1730000000),
		map[string]relationDef{"dueDate": {Format: 4}},
		nil,
		nil,
		nil,
		nil,
		false,
		false,
	)
	if converted != "2024-10-27" {
		t.Fatalf("expected unix seconds to be converted to YYYY-MM-DD, got %#v", converted)
	}

	converted = convertPropertyValue(
		"dateByTypeOnly",
		"1730000000000",
		nil,
		nil,
		nil,
		nil,
		nil,
		true,
		false,
	)
	if converted != "2024-10-27" {
		t.Fatalf("expected unix milliseconds string to be converted via type hint, got %#v", converted)
	}
}

func writePBJSON(t *testing.T, path string, sbType string, details map[string]any, blocks []map[string]any) {
	t.Helper()
	if blocks == nil {
		blocks = []map[string]any{}
	}
	payload := map[string]any{
		"sbType": sbType,
		"snapshot": map[string]any{
			"data": map[string]any{
				"blocks":  blocks,
				"details": details,
			},
		},
	}
	b, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.WriteFile(path, b, 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func mustMkdirAll(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
}
