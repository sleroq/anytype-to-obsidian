package exporter

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
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
	if !strings.Contains(note, "related:") || !strings.Contains(note, "- \"[[Task Two.md]]\"") {
		t.Fatalf("expected related object relation to be rendered, got:\n%s", note)
	}
	if !strings.Contains(note, "status:") || !strings.Contains(note, "- \"Doing\"") {
		t.Fatalf("expected status option to be resolved, got:\n%s", note)
	}
	if !strings.Contains(note, "Task Type:") || !strings.Contains(note, "- \"Bug\"") {
		t.Fatalf("expected relation-id property to render readable key and option value, got:\n%s", note)
	}
	if !strings.Contains(note, "tags:") || !strings.Contains(note, "- \"go\"") || !strings.Contains(note, "- \"export\"") {
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
	if !strings.Contains(note, "backlinks:") || !strings.Contains(note, "- \"obj-2\"") {
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
	if !strings.Contains(note, "backlinks:") || !strings.Contains(note, "- \"[[Task Two.md]]\"") {
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

func TestExporterRenamesPicturePropertyToCoverByDefault(t *testing.T) {
	root := t.TempDir()
	input := filepath.Join(root, "Anytype-json")
	output := filepath.Join(root, "vault")

	mustMkdirAll(t, filepath.Join(input, "objects"))
	mustMkdirAll(t, filepath.Join(input, "relations"))
	mustMkdirAll(t, filepath.Join(input, "relationsOptions"))
	mustMkdirAll(t, filepath.Join(input, "filesObjects"))
	mustMkdirAll(t, filepath.Join(input, "files"))

	writePBJSON(t, filepath.Join(input, "objects", "obj-1.pb.json"), "Page", map[string]any{
		"id":      "obj-1",
		"name":    "Task One",
		"picture": "files/cover.png",
	}, []map[string]any{
		{"id": "obj-1", "childrenIds": []string{"title"}},
		{"id": "title", "text": map[string]any{"text": "Task One", "style": "Title"}},
	})

	_, err := (Exporter{InputDir: input, OutputDir: output}).Run()
	if err != nil {
		t.Fatalf("run exporter: %v", err)
	}

	noteBytes, err := os.ReadFile(filepath.Join(output, "notes", "Task One.md"))
	if err != nil {
		t.Fatalf("read note: %v", err)
	}
	note := string(noteBytes)
	if !strings.Contains(note, "cover: \"files/cover.png\"") {
		t.Fatalf("expected picture to be exported as cover, got:\n%s", note)
	}
	if strings.Contains(note, "picture:") {
		t.Fatalf("expected picture key to be renamed by default, got:\n%s", note)
	}
}

func TestExporterCanDisablePictureToCoverRename(t *testing.T) {
	root := t.TempDir()
	input := filepath.Join(root, "Anytype-json")
	output := filepath.Join(root, "vault")

	mustMkdirAll(t, filepath.Join(input, "objects"))
	mustMkdirAll(t, filepath.Join(input, "relations"))
	mustMkdirAll(t, filepath.Join(input, "relationsOptions"))
	mustMkdirAll(t, filepath.Join(input, "filesObjects"))
	mustMkdirAll(t, filepath.Join(input, "files"))

	writePBJSON(t, filepath.Join(input, "objects", "obj-1.pb.json"), "Page", map[string]any{
		"id":      "obj-1",
		"name":    "Task One",
		"picture": "files/cover.png",
	}, []map[string]any{
		{"id": "obj-1", "childrenIds": []string{"title"}},
		{"id": "title", "text": map[string]any{"text": "Task One", "style": "Title"}},
	})

	_, err := (Exporter{InputDir: input, OutputDir: output, DisablePictureToCover: true}).Run()
	if err != nil {
		t.Fatalf("run exporter: %v", err)
	}

	noteBytes, err := os.ReadFile(filepath.Join(output, "notes", "Task One.md"))
	if err != nil {
		t.Fatalf("read note: %v", err)
	}
	note := string(noteBytes)
	if !strings.Contains(note, "picture: \"files/cover.png\"") {
		t.Fatalf("expected picture key when rename is disabled, got:\n%s", note)
	}
	if strings.Contains(note, "cover:") {
		t.Fatalf("expected cover key to be absent when rename is disabled, got:\n%s", note)
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
	if !strings.Contains(note, "[sample.txt](../files/sample.txt)") {
		t.Fatalf("expected file link, got:\n%s", note)
	}
	if !strings.Contains(note, "[Example](https://example.com)") {
		t.Fatalf("expected bookmark link, got:\n%s", note)
	}

	if _, err := os.Stat(filepath.Join(output, "files", "sample.txt")); err != nil {
		t.Fatalf("expected copied file: %v", err)
	}
}

func TestExporterRendersObsidianCompatibleBlocks(t *testing.T) {
	root := t.TempDir()
	input := filepath.Join(root, "Anytype-json")
	output := filepath.Join(root, "vault")

	mustMkdirAll(t, filepath.Join(input, "objects"))
	mustMkdirAll(t, filepath.Join(input, "relations"))
	mustMkdirAll(t, filepath.Join(input, "relationsOptions"))
	mustMkdirAll(t, filepath.Join(input, "filesObjects"))
	mustMkdirAll(t, filepath.Join(input, "files"))

	writePBJSON(t, filepath.Join(input, "objects", "blocks-page.pb.json"), "Page", map[string]any{
		"id":   "blocks-page",
		"name": "Blocks Page",
	}, []map[string]any{
		{"id": "blocks-page", "childrenIds": []string{"title", "h1", "h2", "toc", "line-divider", "dots-divider", "date-link", "num-1", "num-2", "num-3", "code", "callout", "toggle"}},
		{"id": "title", "text": map[string]any{"text": "Blocks Page", "style": "Title"}},
		{"id": "h1", "text": map[string]any{"text": "Heading One", "style": "Header1"}},
		{"id": "h2", "text": map[string]any{"text": "Heading Two", "style": "Header2"}},
		{"id": "toc", "tableOfContents": map[string]any{}},
		{"id": "line-divider", "div": map[string]any{"style": "Line"}},
		{"id": "dots-divider", "div": map[string]any{"style": "Dots"}},
		{"id": "date-link", "link": map[string]any{"targetBlockId": "_date_2026-02-04"}},
		{"id": "num-1", "text": map[string]any{"text": "first", "style": "Numbered"}},
		{"id": "num-2", "text": map[string]any{"text": "second", "style": "Numbered"}, "childrenIds": []string{"num-2-1"}},
		{"id": "num-2-1", "text": map[string]any{"text": "nested", "style": "Numbered"}},
		{"id": "num-3", "text": map[string]any{"text": "third", "style": "Numbered"}},
		{"id": "code", "fields": map[string]any{"lang": "jsx"}, "text": map[string]any{"text": "\nconsole.log('lol')", "style": "Code"}},
		{"id": "callout", "text": map[string]any{"text": "Callout title", "style": "Callout"}, "childrenIds": []string{"callout-body"}},
		{"id": "callout-body", "text": map[string]any{"text": "inside callout", "style": "Paragraph"}},
		{"id": "toggle", "text": map[string]any{"text": "Collapsed title", "style": "Toggle"}, "childrenIds": []string{"toggle-body"}},
		{"id": "toggle-body", "text": map[string]any{"text": "inside toggle", "style": "Paragraph"}},
	})

	_, err := (Exporter{InputDir: input, OutputDir: output}).Run()
	if err != nil {
		t.Fatalf("run exporter: %v", err)
	}

	noteBytes, err := os.ReadFile(filepath.Join(output, "notes", "Blocks Page.md"))
	if err != nil {
		t.Fatalf("read note: %v", err)
	}
	note := string(noteBytes)
	if strings.Contains(note, "# Blocks Page") {
		t.Fatalf("expected root title block to be skipped in note body, got:\n%s", note)
	}

	if !strings.Contains(note, "- [Heading One](#heading-one)") || !strings.Contains(note, "- [Heading Two](#heading-two)") {
		t.Fatalf("expected generated table of contents, got:\n%s", note)
	}
	if !strings.Contains(note, "---") {
		t.Fatalf("expected line divider to render as horizontal rule, got:\n%s", note)
	}
	if !strings.Contains(note, "***") {
		t.Fatalf("expected dots divider to render as horizontal rule, got:\n%s", note)
	}
	if !strings.Contains(note, "2026-02-04") {
		t.Fatalf("expected date link target to render as date text, got:\n%s", note)
	}
	if !strings.Contains(note, "1. first\n2. second\n1. nested\n3. third") {
		t.Fatalf("expected numbered list sequence with nested numbering, got:\n%s", note)
	}
	if !strings.Contains(note, "```jsx\nconsole.log('lol')\n```") {
		t.Fatalf("expected code block with language, got:\n%s", note)
	}
	if !strings.Contains(note, "> [!note] Callout title\n> inside callout") {
		t.Fatalf("expected callout block, got:\n%s", note)
	}
	if !strings.Contains(note, "> [!note]- Collapsed title\n> inside toggle") {
		t.Fatalf("expected collapsed callout for toggle block, got:\n%s", note)
	}
	if !strings.Contains(note, "> [!note] Callout title\n> inside callout\n\n> [!note]- Collapsed title\n> inside toggle") {
		t.Fatalf("expected adjacent callouts to be separated by a blank line, got:\n%s", note)
	}
}

func TestExporterRendersMentionMarksAsNoteLinks(t *testing.T) {
	root := t.TempDir()
	input := filepath.Join(root, "Anytype-json")
	output := filepath.Join(root, "vault")

	mustMkdirAll(t, filepath.Join(input, "objects"))
	mustMkdirAll(t, filepath.Join(input, "relations"))
	mustMkdirAll(t, filepath.Join(input, "relationsOptions"))
	mustMkdirAll(t, filepath.Join(input, "filesObjects"))
	mustMkdirAll(t, filepath.Join(input, "files"))

	writePBJSON(t, filepath.Join(input, "objects", "person-1.pb.json"), "Page", map[string]any{
		"id":   "person-1",
		"name": "Anastasiya Pervusheva",
	}, []map[string]any{
		{"id": "person-1", "childrenIds": []string{"title"}},
		{"id": "title", "text": map[string]any{"text": "Anastasiya Pervusheva", "style": "Title"}},
	})

	writePBJSON(t, filepath.Join(input, "objects", "obj-1.pb.json"), "Page", map[string]any{
		"id":   "obj-1",
		"name": "Mention Page",
	}, []map[string]any{
		{"id": "obj-1", "childrenIds": []string{"title", "p1"}},
		{"id": "title", "text": map[string]any{"text": "Mention Page", "style": "Title"}},
		{"id": "p1", "text": map[string]any{
			"text":  "Hello Anastasiya Pervusheva!",
			"style": "Paragraph",
			"marks": map[string]any{
				"marks": []any{
					map[string]any{
						"range": map[string]any{"from": 6, "to": 27},
						"type":  "Mention",
						"param": "person-1",
					},
				},
			},
		}},
	})

	_, err := (Exporter{InputDir: input, OutputDir: output}).Run()
	if err != nil {
		t.Fatalf("run exporter: %v", err)
	}

	noteBytes, err := os.ReadFile(filepath.Join(output, "notes", "Mention Page.md"))
	if err != nil {
		t.Fatalf("read note: %v", err)
	}
	note := string(noteBytes)
	if !strings.Contains(note, "Hello [[Anastasiya Pervusheva.md]]!") {
		t.Fatalf("expected mention mark to render note link, got:\n%s", note)
	}
}

func TestExporterSkipsSystemTitleInsideHeaderLayout(t *testing.T) {
	root := t.TempDir()
	input := filepath.Join(root, "Anytype-json")
	output := filepath.Join(root, "vault")

	mustMkdirAll(t, filepath.Join(input, "objects"))
	mustMkdirAll(t, filepath.Join(input, "relations"))
	mustMkdirAll(t, filepath.Join(input, "relationsOptions"))
	mustMkdirAll(t, filepath.Join(input, "filesObjects"))
	mustMkdirAll(t, filepath.Join(input, "files"))

	writePBJSON(t, filepath.Join(input, "objects", "header-page.pb.json"), "Page", map[string]any{
		"id":   "header-page",
		"name": "Header Page",
	}, []map[string]any{
		{"id": "header-page", "childrenIds": []string{"header", "content"}},
		{"id": "header", "layout": map[string]any{"style": "Header"}, "childrenIds": []string{"title", "description"}},
		{"id": "title", "fields": map[string]any{"_detailsKey": []any{"name"}}, "text": map[string]any{"text": "Header Page", "style": "Title"}},
		{"id": "description", "fields": map[string]any{"_detailsKey": "description"}, "text": map[string]any{"text": "", "style": "Description"}},
		{"id": "content", "text": map[string]any{"text": "Body paragraph", "style": "Paragraph"}},
	})

	_, err := (Exporter{InputDir: input, OutputDir: output}).Run()
	if err != nil {
		t.Fatalf("run exporter: %v", err)
	}

	noteBytes, err := os.ReadFile(filepath.Join(output, "notes", "Header Page.md"))
	if err != nil {
		t.Fatalf("read note: %v", err)
	}
	note := string(noteBytes)
	if strings.Contains(note, "\n# Header Page\n") || strings.Contains(note, "\n# \n") {
		t.Fatalf("expected system title block to be skipped in note body, got:\n%s", note)
	}
	if !strings.Contains(note, "Body paragraph") {
		t.Fatalf("expected body content to be rendered, got:\n%s", note)
	}
}

func TestExporterAppliesFilesystemTimestampsFromAnytypeDetails(t *testing.T) {
	root := t.TempDir()
	input := filepath.Join(root, "Anytype-json")
	output := filepath.Join(root, "vault")

	mustMkdirAll(t, filepath.Join(input, "objects"))
	mustMkdirAll(t, filepath.Join(input, "relations"))
	mustMkdirAll(t, filepath.Join(input, "relationsOptions"))
	mustMkdirAll(t, filepath.Join(input, "filesObjects"))
	mustMkdirAll(t, filepath.Join(input, "files"))

	createdUnix := int64(1700000000)
	modifiedUnix := int64(1730000000)

	writePBJSON(t, filepath.Join(input, "objects", "obj-1.pb.json"), "Page", map[string]any{
		"id":               "obj-1",
		"name":             "Timestamped",
		"createdDate":      createdUnix,
		"lastModifiedDate": modifiedUnix,
	}, []map[string]any{
		{"id": "obj-1", "childrenIds": []string{"title"}},
		{"id": "title", "text": map[string]any{"text": "Timestamped", "style": "Title"}},
	})

	_, err := (Exporter{InputDir: input, OutputDir: output}).Run()
	if err != nil {
		t.Fatalf("run exporter: %v", err)
	}

	notePath := filepath.Join(output, "notes", "Timestamped.md")
	info, err := os.Stat(notePath)
	if err != nil {
		t.Fatalf("stat note: %v", err)
	}

	if got := info.ModTime().UTC().Unix(); got != modifiedUnix {
		t.Fatalf("expected note mtime %d, got %d", modifiedUnix, got)
	}
	if runtime.GOOS == "darwin" {
		if got := int64(info.Sys().(*syscall.Stat_t).Birthtimespec.Sec); got != createdUnix {
			t.Fatalf("expected note birthtime %d, got %d", createdUnix, got)
		}
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
	if !strings.Contains(personNote, "type: \"[[Human.md]]\"") {
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
	if !strings.Contains(note, "tags:") || !strings.Contains(note, "- \"[[go.md]]\"") {
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
	if strings.Contains(template, "\n# Contact\n") || strings.Contains(template, "\n# \n") {
		t.Fatalf("expected root title block to be skipped in template body, got:\n%s", template)
	}
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
		"",
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
		"",
		nil,
		nil,
		true,
		false,
	)
	if converted != "2024-10-27" {
		t.Fatalf("expected unix milliseconds string to be converted via type hint, got %#v", converted)
	}
}

func TestAnytypeTimestampsPrefersCreatedForAccessAndModifiedForWrite(t *testing.T) {
	createdUnix := int64(1700000000)
	changedUnix := int64(1720000000)
	modifiedUnix := int64(1730000000)

	atime, mtime, ok := anytypeTimestamps(map[string]any{
		"createdDate":      createdUnix,
		"changedDate":      changedUnix,
		"lastModifiedDate": modifiedUnix,
	})
	if !ok {
		t.Fatalf("expected timestamps to be resolved")
	}
	if atime.UTC().Unix() != createdUnix {
		t.Fatalf("expected atime from createdDate %d, got %d", createdUnix, atime.UTC().Unix())
	}
	if mtime.UTC().Unix() != modifiedUnix {
		t.Fatalf("expected mtime from lastModifiedDate %d, got %d", modifiedUnix, mtime.UTC().Unix())
	}
}

func TestExporterInfersNoteFileNameFromTitleThenDetailsThenID(t *testing.T) {
	root := t.TempDir()
	input := filepath.Join(root, "Anytype-json")
	output := filepath.Join(root, "vault")

	mustMkdirAll(t, filepath.Join(input, "objects"))
	mustMkdirAll(t, filepath.Join(input, "relations"))
	mustMkdirAll(t, filepath.Join(input, "relationsOptions"))
	mustMkdirAll(t, filepath.Join(input, "filesObjects"))
	mustMkdirAll(t, filepath.Join(input, "files"))

	writePBJSON(t, filepath.Join(input, "objects", "obj-block.pb.json"), "Page", map[string]any{
		"id": "obj-block",
	}, []map[string]any{
		{"id": "obj-block", "childrenIds": []string{"title-block"}},
		{"id": "title-block", "text": map[string]any{"text": "From Title Block", "style": "Title"}},
	})

	writePBJSON(t, filepath.Join(input, "objects", "obj-details.pb.json"), "Page", map[string]any{
		"id":    "obj-details",
		"title": "From Details Title",
	}, []map[string]any{
		{"id": "obj-details", "childrenIds": []string{"paragraph"}},
		{"id": "paragraph", "text": map[string]any{"text": "body", "style": "Paragraph"}},
	})

	writePBJSON(t, filepath.Join(input, "objects", "obj-fallback.pb.json"), "Page", map[string]any{
		"id": "obj-fallback",
	}, []map[string]any{{"id": "obj-fallback", "childrenIds": []string{}}})

	_, err := (Exporter{InputDir: input, OutputDir: output}).Run()
	if err != nil {
		t.Fatalf("run exporter: %v", err)
	}

	if _, err := os.Stat(filepath.Join(output, "notes", "From Title Block.md")); err != nil {
		t.Fatalf("expected title-block fallback filename: %v", err)
	}
	if _, err := os.Stat(filepath.Join(output, "notes", "From Details Title.md")); err != nil {
		t.Fatalf("expected details.title fallback filename: %v", err)
	}
	if _, err := os.Stat(filepath.Join(output, "notes", "obj-fallback.md")); err != nil {
		t.Fatalf("expected object-id fallback filename: %v", err)
	}
}

func TestExporterResetsNumberedListAfterNonNumberedSiblingAndUsesTabIndent(t *testing.T) {
	root := t.TempDir()
	input := filepath.Join(root, "Anytype-json")
	output := filepath.Join(root, "vault")

	mustMkdirAll(t, filepath.Join(input, "objects"))
	mustMkdirAll(t, filepath.Join(input, "relations"))
	mustMkdirAll(t, filepath.Join(input, "relationsOptions"))
	mustMkdirAll(t, filepath.Join(input, "filesObjects"))
	mustMkdirAll(t, filepath.Join(input, "files"))

	writePBJSON(t, filepath.Join(input, "objects", "list-page.pb.json"), "Page", map[string]any{
		"id":   "list-page",
		"name": "List Page",
	}, []map[string]any{
		{"id": "list-page", "childrenIds": []string{"title", "num-1", "paragraph", "num-2", "num-3", "num-parent"}},
		{"id": "title", "text": map[string]any{"text": "List Page", "style": "Title"}},
		{"id": "num-1", "text": map[string]any{"text": "first", "style": "Numbered"}},
		{"id": "paragraph", "text": map[string]any{"text": "break", "style": "Paragraph"}},
		{"id": "num-2", "text": map[string]any{"text": "second", "style": "Numbered"}},
		{"id": "num-3", "text": map[string]any{"text": "third", "style": "Numbered"}},
		{"id": "num-parent", "text": map[string]any{"text": "parent", "style": "Numbered"}, "childrenIds": []string{"num-child"}},
		{"id": "num-child", "text": map[string]any{"text": "nested", "style": "Numbered"}},
	})

	_, err := (Exporter{InputDir: input, OutputDir: output}).Run()
	if err != nil {
		t.Fatalf("run exporter: %v", err)
	}

	noteBytes, err := os.ReadFile(filepath.Join(output, "notes", "List Page.md"))
	if err != nil {
		t.Fatalf("read note: %v", err)
	}
	note := string(noteBytes)

	if !strings.Contains(note, "1. first\nbreak\n1. second\n2. third") {
		t.Fatalf("expected numbering reset after paragraph break, got:\n%s", note)
	}
	if !strings.Contains(note, "3. parent\n1. nested") {
		t.Fatalf("expected nested numbered item to keep independent numbering, got:\n%s", note)
	}
}

func TestExporterBuildsFilePathFromFileObjectWhenSourceIsMissing(t *testing.T) {
	root := t.TempDir()
	input := filepath.Join(root, "Anytype-json")
	output := filepath.Join(root, "vault")

	mustMkdirAll(t, filepath.Join(input, "objects"))
	mustMkdirAll(t, filepath.Join(input, "relations"))
	mustMkdirAll(t, filepath.Join(input, "relationsOptions"))
	mustMkdirAll(t, filepath.Join(input, "filesObjects"))
	mustMkdirAll(t, filepath.Join(input, "files"))

	if err := os.WriteFile(filepath.Join(input, "files", "Report.pdf"), []byte("pdf"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	writePBJSON(t, filepath.Join(input, "filesObjects", "file-1.pb.json"), "FileObject", map[string]any{
		"id":      "file-1",
		"name":    "Report",
		"fileExt": "pdf",
	}, nil)

	writePBJSON(t, filepath.Join(input, "objects", "file-page.pb.json"), "Page", map[string]any{
		"id":   "file-page",
		"name": "File Page",
	}, []map[string]any{
		{"id": "file-page", "childrenIds": []string{"title", "file-block"}},
		{"id": "title", "text": map[string]any{"text": "File Page", "style": "Title"}},
		{"id": "file-block", "file": map[string]any{"name": "Report.pdf", "type": "File", "targetObjectId": "file-1"}},
	})

	_, err := (Exporter{InputDir: input, OutputDir: output}).Run()
	if err != nil {
		t.Fatalf("run exporter: %v", err)
	}

	noteBytes, err := os.ReadFile(filepath.Join(output, "notes", "File Page.md"))
	if err != nil {
		t.Fatalf("read note: %v", err)
	}
	note := string(noteBytes)
	if !strings.Contains(note, "[Report.pdf](../files/Report.pdf)") {
		t.Fatalf("expected file link to use synthesized files path, got:\n%s", note)
	}
}

func TestDateFormattingAndTimestampFallbackVariants(t *testing.T) {
	if got := formatDateValue("2026-02-01T09:10:11+03:00"); got != "2026-02-01" {
		t.Fatalf("expected RFC3339 date formatting, got %#v", got)
	}
	if got := formatDateValue("2026-02-02"); got != "2026-02-02" {
		t.Fatalf("expected date-string passthrough formatting, got %#v", got)
	}
	if got := formatDateValue("not-a-date"); got != "not-a-date" {
		t.Fatalf("expected invalid date value to be preserved, got %#v", got)
	}

	atime, mtime, ok := anytypeTimestamps(map[string]any{"changedDate": "1700001000000"})
	if !ok {
		t.Fatalf("expected changedDate-only details to produce file timestamps")
	}
	if atime.UTC().Unix() != 1700001000 {
		t.Fatalf("expected atime fallback to changedDate, got %d", atime.UTC().Unix())
	}
	if mtime.UTC().Unix() != 1700001000 {
		t.Fatalf("expected mtime fallback to changedDate, got %d", mtime.UTC().Unix())
	}
}

func TestExporterGeneratesBaseFileFromDataviewQuery(t *testing.T) {
	root := t.TempDir()
	input := filepath.Join(root, "Anytype-json")
	output := filepath.Join(root, "vault")

	mustMkdirAll(t, filepath.Join(input, "objects"))
	mustMkdirAll(t, filepath.Join(input, "relations"))
	mustMkdirAll(t, filepath.Join(input, "relationsOptions"))
	mustMkdirAll(t, filepath.Join(input, "filesObjects"))
	mustMkdirAll(t, filepath.Join(input, "files"))

	writePBJSON(t, filepath.Join(input, "relations", "rel-created.pb.json"), "STRelation", map[string]any{
		"id":             "rel-created",
		"relationKey":    "createdDate",
		"relationFormat": 4,
		"name":           "createdDate",
	}, nil)
	writePBJSON(t, filepath.Join(input, "relations", "rel-modified.pb.json"), "STRelation", map[string]any{
		"id":             "rel-modified",
		"relationKey":    "lastModifiedDate",
		"relationFormat": 4,
		"name":           "lastModifiedDate",
	}, nil)
	writePBJSON(t, filepath.Join(input, "relations", "rel-status.pb.json"), "STRelation", map[string]any{
		"id":             "rel-status",
		"relationKey":    "status",
		"relationFormat": 3,
		"name":           "Status",
	}, nil)
	writePBJSON(t, filepath.Join(input, "relations", "rel-task-type.pb.json"), "STRelation", map[string]any{
		"id":             "rel-task-type",
		"relationKey":    "65edf2aa8efc1e005b0cb9d2",
		"relationFormat": 3,
		"name":           "Task Type",
	}, nil)
	writePBJSON(t, filepath.Join(input, "relations", "rel-due-date.pb.json"), "STRelation", map[string]any{
		"id":             "rel-due-date",
		"relationKey":    "dueDate",
		"relationFormat": 4,
		"name":           "Due Date",
	}, nil)

	writePBJSON(t, filepath.Join(input, "relationsOptions", "opt-task-type-focus.pb.json"), "STRelationOption", map[string]any{
		"id":   "opt-task-type-focus",
		"name": "Focus",
	}, nil)
	writePBJSON(t, filepath.Join(input, "relationsOptions", "opt-status-doing.pb.json"), "STRelationOption", map[string]any{
		"id":   "opt-status-doing",
		"name": "Doing",
	}, nil)

	writePBJSON(t, filepath.Join(input, "objects", "query.pb.json"), "Page", map[string]any{
		"id":   "query",
		"name": "General Journal",
	}, []map[string]any{
		{"id": "query", "childrenIds": []string{"title", "dataview"}},
		{"id": "title", "text": map[string]any{"text": "General Journal", "style": "Title"}},
		{"id": "dataview", "dataview": map[string]any{
			"views": []any{
				map[string]any{
					"id":   "view-1",
					"type": "Kanban",
					"name": "All",
					"relations": []any{
						map[string]any{"key": "name", "isVisible": true},
						map[string]any{"key": "tag", "isVisible": true},
						map[string]any{"key": "dueDate", "isVisible": true},
						map[string]any{"key": "status", "isVisible": false},
					},
					"sorts": []any{
						map[string]any{"RelationKey": "lastModifiedDate", "type": "Desc", "format": "date", "includeTime": true, "emptyPlacement": "NotSpecified", "noCollate": false},
						map[string]any{"RelationKey": "createdDate", "type": "Desc", "format": "date", "includeTime": true, "emptyPlacement": "Start", "noCollate": true},
						map[string]any{"RelationKey": "status", "type": "Custom", "customOrder": []any{"opt-status-doing"}, "format": "status", "includeTime": false, "emptyPlacement": "End", "noCollate": false},
					},
					"filters": []any{
						map[string]any{"operator": "No", "RelationKey": "65edf2aa8efc1e005b0cb9d2", "condition": "In", "value": []any{"opt-task-type-focus"}, "format": "status", "includeTime": false},
					},
					"groupRelationKey": "status",
					"pageLimit":        100,
				},
			},
		}},
	})

	_, err := (Exporter{InputDir: input, OutputDir: output}).Run()
	if err != nil {
		t.Fatalf("run exporter: %v", err)
	}

	baseBytes, err := os.ReadFile(filepath.Join(output, "bases", "General Journal.base"))
	if err != nil {
		t.Fatalf("read base file: %v", err)
	}
	base := string(baseBytes)

	if !strings.Contains(base, "views:") || !strings.Contains(base, "name: \"All\"") {
		t.Fatalf("expected base views to be rendered, got:\n%s", base)
	}
	if !strings.Contains(base, "order:") || !strings.Contains(base, "\"file.name\"") || !strings.Contains(base, "\"note.tags\"") || !strings.Contains(base, "\"note.dueDate\"") {
		t.Fatalf("expected selected properties mapped into view order, got:\n%s", base)
	}
	if strings.Contains(base, "\n      - \"note.status\"\n") {
		t.Fatalf("expected hidden relation to be excluded from selected properties, got:\n%s", base)
	}
	if !strings.Contains(base, "sort:") || !strings.Contains(base, "property: \"file.mtime\"") || !strings.Contains(base, "property: \"file.ctime\"") {
		t.Fatalf("expected created/modified sorts mapped into sort metadata, got:\n%s", base)
	}
	if !strings.Contains(base, "groupBy:") || !strings.Contains(base, "\"note.status\"") {
		t.Fatalf("expected groupBy to be rendered, got:\n%s", base)
	}
	if !strings.Contains(base, "Task Type") || !strings.Contains(base, "Focus") {
		t.Fatalf("expected filter value and relation key mapping, got:\n%s", base)
	}
	if !strings.Contains(base, "direction: \"CUSTOM\"") || !strings.Contains(base, "customOrder:") || !strings.Contains(base, "\"Doing\"") {
		t.Fatalf("expected custom sort metadata to be preserved, got:\n%s", base)
	}
}

func TestParseDataviewViewsMapsGalleryToCards(t *testing.T) {
	views := parseDataviewViews(map[string]any{
		"views": []any{
			map[string]any{
				"type": "Gallery",
				"name": "All",
			},
		},
	}, nil, nil, nil, nil, nil, false)

	if len(views) != 1 {
		t.Fatalf("expected one view, got %d", len(views))
	}
	if views[0].Type != "cards" {
		t.Fatalf("expected gallery view to map to cards, got %q", views[0].Type)
	}
	if views[0].Name != "All" {
		t.Fatalf("expected view name to be preserved, got %q", views[0].Name)
	}
}

func TestBuildFilterExpressionSupportsAllAnytypeConditions(t *testing.T) {
	relations := map[string]relationDef{
		"status": {Key: "status", Name: "Status", Format: 3},
	}
	optionsByID := map[string]string{"opt-a": "A"}

	conditions := []string{
		"Equal",
		"NotEqual",
		"Greater",
		"Less",
		"GreaterOrEqual",
		"LessOrEqual",
		"Like",
		"NotLike",
		"In",
		"NotIn",
		"Empty",
		"NotEmpty",
		"AllIn",
		"NotAllIn",
		"ExactIn",
		"NotExactIn",
		"Exists",
	}

	for _, condition := range conditions {
		value := any("opt-a")
		switch condition {
		case "In", "NotIn", "AllIn", "NotAllIn", "ExactIn", "NotExactIn":
			value = []any{"opt-a"}
		}
		expr := buildFilterExpression(map[string]any{
			"RelationKey": "status",
			"condition":   condition,
			"value":       value,
			"format":      "status",
		}, relations, optionsByID, nil, nil, nil, false)
		if strings.TrimSpace(expr) == "" {
			t.Fatalf("expected non-empty expression for condition %s", condition)
		}
	}
}

func TestExporterRunsPrettierWhenEnabled(t *testing.T) {
	root := t.TempDir()
	input := filepath.Join(root, "Anytype-json")
	output := filepath.Join(root, "vault")
	prepareMinimalExportFixture(t, input)

	originalRunner := prettierCommandRunner
	t.Cleanup(func() {
		prettierCommandRunner = originalRunner
	})

	called := false
	callCount := 0
	calledWithDir := ""
	prettierCommandRunner = func(outputDir string) error {
		called = true
		callCount++
		calledWithDir = outputDir
		return nil
	}

	_, err := (Exporter{InputDir: input, OutputDir: output, RunPrettier: true}).Run()
	if err != nil {
		t.Fatalf("run exporter: %v", err)
	}
	if !called {
		t.Fatalf("expected prettier runner to be called")
	}
	if calledWithDir != output {
		t.Fatalf("expected prettier runner to be called with output dir %q, got %q", output, calledWithDir)
	}
	if callCount != 1 {
		t.Fatalf("expected prettier runner to be called once, got %d", callCount)
	}
}

func TestExporterIgnoresPrettierFailure(t *testing.T) {
	root := t.TempDir()
	input := filepath.Join(root, "Anytype-json")
	output := filepath.Join(root, "vault")
	prepareMinimalExportFixture(t, input)

	originalRunner := prettierCommandRunner
	t.Cleanup(func() {
		prettierCommandRunner = originalRunner
	})

	prettierCommandRunner = func(string) error {
		return os.ErrNotExist
	}

	_, err := (Exporter{InputDir: input, OutputDir: output, RunPrettier: true}).Run()
	if err != nil {
		t.Fatalf("run exporter should not fail when prettier fails: %v", err)
	}

	if _, statErr := os.Stat(filepath.Join(output, "notes", "Task One.md")); statErr != nil {
		t.Fatalf("expected export files to be written despite prettier failure: %v", statErr)
	}
}

func TestExporterWritesIconizeDataFromEmojiAndImageIcons(t *testing.T) {
	root := t.TempDir()
	input := filepath.Join(root, "Anytype-json")
	output := filepath.Join(root, "vault")

	mustMkdirAll(t, filepath.Join(input, "objects"))
	mustMkdirAll(t, filepath.Join(input, "relations"))
	mustMkdirAll(t, filepath.Join(input, "relationsOptions"))
	mustMkdirAll(t, filepath.Join(input, "filesObjects"))
	mustMkdirAll(t, filepath.Join(input, "files"))

	if err := os.WriteFile(filepath.Join(input, "files", "icon-image.bin"), []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}, 0o644); err != nil {
		t.Fatalf("write icon image file: %v", err)
	}

	writePBJSON(t, filepath.Join(input, "filesObjects", "icon-file-1.pb.json"), "FileObject", map[string]any{
		"id":     "icon-file-1",
		"name":   "icon-image",
		"source": "files/icon-image.bin",
	}, nil)

	writePBJSON(t, filepath.Join(input, "objects", "obj-emoji.pb.json"), "Page", map[string]any{
		"id":        "obj-emoji",
		"name":      "Emoji Note",
		"iconEmoji": "📡",
	}, []map[string]any{
		{"id": "obj-emoji", "childrenIds": []string{"title-emoji"}},
		{"id": "title-emoji", "text": map[string]any{"text": "Emoji Note", "style": "Title"}},
	})

	writePBJSON(t, filepath.Join(input, "objects", "obj-image.pb.json"), "Page", map[string]any{
		"id":        "obj-image",
		"name":      "Image Note",
		"iconImage": "icon-file-1",
	}, []map[string]any{
		{"id": "obj-image", "childrenIds": []string{"title-image"}},
		{"id": "title-image", "text": map[string]any{"text": "Image Note", "style": "Title"}},
	})

	_, err := (Exporter{InputDir: input, OutputDir: output}).Run()
	if err != nil {
		t.Fatalf("run exporter: %v", err)
	}

	dataPath := filepath.Join(output, ".obsidian", "plugins", "obsidian-icon-folder", "data.json")
	dataBytes, err := os.ReadFile(dataPath)
	if err != nil {
		t.Fatalf("read iconize data: %v", err)
	}

	var data map[string]any
	if err := json.Unmarshal(dataBytes, &data); err != nil {
		t.Fatalf("decode iconize data: %v", err)
	}

	if got := asString(data["notes/Emoji Note.md"]); got != "📡" {
		t.Fatalf("expected emoji icon mapping, got %q", got)
	}

	imageIconValue := asString(data["notes/Image Note.md"])
	if !strings.HasPrefix(imageIconValue, iconizeAnytypePackPrefix+"AnytypeIcon") {
		t.Fatalf("expected generated icon pack reference for image icon, got %q", imageIconValue)
	}

	if _, ok := data["settings"].(map[string]any); !ok {
		t.Fatalf("expected iconize settings to be present in data.json")
	}

	iconName := strings.TrimPrefix(imageIconValue, iconizeAnytypePackPrefix)
	iconSVGPath := filepath.Join(output, ".obsidian", "icons", iconizeAnytypePackName, iconName+".svg")
	iconSVG, err := os.ReadFile(iconSVGPath)
	if err != nil {
		t.Fatalf("read generated icon svg: %v", err)
	}
	iconSVGContent := string(iconSVG)
	if !strings.Contains(iconSVGContent, "<svg") || !strings.Contains(iconSVGContent, "data:image/") {
		t.Fatalf("expected generated icon svg to embed image data, got:\n%s", iconSVGContent)
	}
}

func TestExporterCanDisableIconizeIntegration(t *testing.T) {
	root := t.TempDir()
	input := filepath.Join(root, "Anytype-json")
	output := filepath.Join(root, "vault")

	prepareMinimalExportFixture(t, input)

	writePBJSON(t, filepath.Join(input, "objects", "obj-icon.pb.json"), "Page", map[string]any{
		"id":        "obj-icon",
		"name":      "Icon Note",
		"iconEmoji": "✅",
	}, []map[string]any{
		{"id": "obj-icon", "childrenIds": []string{"title"}},
		{"id": "title", "text": map[string]any{"text": "Icon Note", "style": "Title"}},
	})

	_, err := (Exporter{InputDir: input, OutputDir: output, DisableIconizeIcons: true}).Run()
	if err != nil {
		t.Fatalf("run exporter: %v", err)
	}

	if _, err := os.Stat(filepath.Join(output, ".obsidian", "plugins", "obsidian-icon-folder", "data.json")); !os.IsNotExist(err) {
		t.Fatalf("expected iconize data.json to not exist when integration is disabled, got: %v", err)
	}
	if _, err := os.Stat(filepath.Join(output, ".obsidian", "icons", iconizeAnytypePackName)); !os.IsNotExist(err) {
		t.Fatalf("expected icon pack directory to not exist when integration is disabled, got: %v", err)
	}
}

func prepareMinimalExportFixture(t *testing.T, input string) {
	t.Helper()
	mustMkdirAll(t, filepath.Join(input, "objects"))
	mustMkdirAll(t, filepath.Join(input, "relations"))
	mustMkdirAll(t, filepath.Join(input, "relationsOptions"))
	mustMkdirAll(t, filepath.Join(input, "filesObjects"))
	mustMkdirAll(t, filepath.Join(input, "files"))

	writePBJSON(t, filepath.Join(input, "objects", "obj-1.pb.json"), "Page", map[string]any{
		"id":   "obj-1",
		"name": "Task One",
	}, []map[string]any{
		{"id": "obj-1", "childrenIds": []string{"title"}},
		{"id": "title", "text": map[string]any{"text": "Task One", "style": "Title"}},
	})
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
