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

	noteBytes, err := os.ReadFile(filepath.Join(output, "notes", "task-one.md"))
	if err != nil {
		t.Fatalf("read note: %v", err)
	}
	note := string(noteBytes)
	if !strings.Contains(note, "related: \"[[notes/task-two.md]]\"") {
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

	noteBytes, err := os.ReadFile(filepath.Join(output, "notes", "task-one.md"))
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

	noteBytes, err := os.ReadFile(filepath.Join(output, "notes", "task-one.md"))
	if err != nil {
		t.Fatalf("read note: %v", err)
	}
	note := string(noteBytes)
	if !strings.Contains(note, "backlinks: \"obj-2\"") {
		t.Fatalf("expected backlinks to be included when enabled, got:\n%s", note)
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

	noteBytes, err := os.ReadFile(filepath.Join(output, "notes", "table-page.md"))
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
