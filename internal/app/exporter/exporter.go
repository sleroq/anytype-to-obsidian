package exporter

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

type Exporter struct {
	InputDir                  string
	OutputDir                 string
	IncludeDynamicProperties  bool
	IncludeArchivedProperties bool
}

type Stats struct {
	Notes int
	Files int
}

type snapshotFile struct {
	SbType   string `json:"sbType"`
	Snapshot struct {
		Data struct {
			Blocks  []block        `json:"blocks"`
			Details map[string]any `json:"details"`
		} `json:"data"`
	} `json:"snapshot"`
}

type block struct {
	ID         string         `json:"id"`
	ChildrenID []string       `json:"childrenIds"`
	Fields     map[string]any `json:"fields"`

	Text     *textBlock     `json:"text"`
	File     *fileBlock     `json:"file"`
	Bookmark *bookmarkBlock `json:"bookmark"`
	Latex    *latexBlock    `json:"latex"`
	Link     *linkBlock     `json:"link"`
	Layout   *layoutBlock   `json:"layout"`
	Table    map[string]any `json:"table"`
}

type textBlock struct {
	Text    string `json:"text"`
	Style   string `json:"style"`
	Checked bool   `json:"checked"`
}

type fileBlock struct {
	Name           string `json:"name"`
	Type           string `json:"type"`
	TargetObjectID string `json:"targetObjectId"`
}

type bookmarkBlock struct {
	URL   string `json:"url"`
	Title string `json:"title"`
}

type latexBlock struct {
	Text string `json:"text"`
}

type linkBlock struct {
	TargetBlockID string `json:"targetBlockId"`
}

type layoutBlock struct {
	Style string `json:"style"`
}

type relationDef struct {
	ID     string
	Key    string
	Name   string
	Format int
}

type objectInfo struct {
	ID      string
	Name    string
	SbType  string
	Details map[string]any
	Blocks  []block
}

type indexFile struct {
	Notes map[string]string `json:"notes"`
}

var dynamicPropertyKeys = map[string]struct{}{
	"addedDate":          {},
	"backlinks":          {},
	"fileBackupStatus":   {},
	"fileIndexingStatus": {},
	"fileSyncStatus":     {},
	"lastMessageDate":    {},
	"lastModifiedBy":     {},
	"lastModifiedDate":   {},
	"lastOpenedBy":       {},
	"lastOpenedDate":     {},
	"lastUsedDate":       {},
	"links":              {},
	"mentions":           {},
	"revision":           {},
	"syncDate":           {},
	"syncError":          {},
	"syncStatus":         {},
}

func (e Exporter) Run() (Stats, error) {
	if e.InputDir == "" || e.OutputDir == "" {
		return Stats{}, fmt.Errorf("input and output directories are required")
	}

	if err := os.MkdirAll(e.OutputDir, 0o755); err != nil {
		return Stats{}, fmt.Errorf("create output dir: %w", err)
	}

	objects, err := readObjects(filepath.Join(e.InputDir, "objects"))
	if err != nil {
		return Stats{}, err
	}
	relations, err := readRelations(filepath.Join(e.InputDir, "relations"))
	if err != nil {
		return Stats{}, err
	}
	optionsByID, err := readOptions(filepath.Join(e.InputDir, "relationsOptions"))
	if err != nil {
		return Stats{}, err
	}
	fileObjects, err := readFileObjects(filepath.Join(e.InputDir, "filesObjects"))
	if err != nil {
		return Stats{}, err
	}

	noteDir := filepath.Join(e.OutputDir, "notes")
	rawDir := filepath.Join(e.OutputDir, "_anytype", "raw")
	if err := os.MkdirAll(noteDir, 0o755); err != nil {
		return Stats{}, err
	}
	if err := os.MkdirAll(rawDir, 0o755); err != nil {
		return Stats{}, err
	}

	copiedFiles, err := copyDir(filepath.Join(e.InputDir, "files"), filepath.Join(e.OutputDir, "files"))
	if err != nil {
		return Stats{}, err
	}

	notePathByID := make(map[string]string, len(objects))
	used := map[string]int{}
	for _, obj := range objects {
		base := sanitizeName(obj.Name)
		if base == "" {
			base = obj.ID
		}
		n := used[base]
		used[base] = n + 1
		if n > 0 {
			base = base + "-" + strconv.Itoa(n+1)
		}
		notePathByID[obj.ID] = filepath.ToSlash(filepath.Join("notes", base+".md"))
	}

	idToObject := make(map[string]objectInfo, len(objects))
	for _, o := range objects {
		idToObject[o.ID] = o
	}

	for _, obj := range objects {
		noteRelPath := notePathByID[obj.ID]
		noteAbsPath := filepath.Join(e.OutputDir, filepath.FromSlash(noteRelPath))
		if err := os.MkdirAll(filepath.Dir(noteAbsPath), 0o755); err != nil {
			return Stats{}, err
		}

		fm := renderFrontmatter(obj, relations, optionsByID, notePathByID, fileObjects, e.IncludeDynamicProperties, e.IncludeArchivedProperties)
		body := renderBody(obj, idToObject, notePathByID, fileObjects)
		if err := os.WriteFile(noteAbsPath, []byte(fm+body), 0o644); err != nil {
			return Stats{}, fmt.Errorf("write note %s: %w", obj.ID, err)
		}

		rawPath := filepath.Join(rawDir, obj.ID+".json")
		rawPayload := map[string]any{
			"id":      obj.ID,
			"sbType":  obj.SbType,
			"details": obj.Details,
		}
		rawBytes, _ := json.MarshalIndent(rawPayload, "", "  ")
		if err := os.WriteFile(rawPath, rawBytes, 0o644); err != nil {
			return Stats{}, err
		}
	}

	idx := indexFile{Notes: notePathByID}
	indexBytes, _ := json.MarshalIndent(idx, "", "  ")
	if err := os.MkdirAll(filepath.Join(e.OutputDir, "_anytype"), 0o755); err != nil {
		return Stats{}, err
	}
	if err := os.WriteFile(filepath.Join(e.OutputDir, "_anytype", "index.json"), indexBytes, 0o644); err != nil {
		return Stats{}, err
	}

	return Stats{Notes: len(objects), Files: copiedFiles}, nil
}

func readObjects(dir string) ([]objectInfo, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read objects dir: %w", err)
	}
	var out []objectInfo
	for _, ent := range entries {
		if ent.IsDir() || !strings.HasSuffix(ent.Name(), ".pb.json") {
			continue
		}
		f, err := readSnapshot(filepath.Join(dir, ent.Name()))
		if err != nil {
			return nil, err
		}
		id := asString(f.Snapshot.Data.Details["id"])
		if id == "" {
			id = strings.TrimSuffix(ent.Name(), ".pb.json")
		}
		out = append(out, objectInfo{
			ID:      id,
			Name:    asString(f.Snapshot.Data.Details["name"]),
			SbType:  f.SbType,
			Details: f.Snapshot.Data.Details,
			Blocks:  f.Snapshot.Data.Blocks,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func readRelations(dir string) (map[string]relationDef, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read relations dir: %w", err)
	}
	out := make(map[string]relationDef)
	for _, ent := range entries {
		if ent.IsDir() || !strings.HasSuffix(ent.Name(), ".pb.json") {
			continue
		}
		f, err := readSnapshot(filepath.Join(dir, ent.Name()))
		if err != nil {
			return nil, err
		}
		id := asString(f.Snapshot.Data.Details["id"])
		key := asString(f.Snapshot.Data.Details["relationKey"])
		if key == "" && id == "" {
			continue
		}
		def := relationDef{
			ID:     id,
			Key:    key,
			Name:   asString(f.Snapshot.Data.Details["name"]),
			Format: asInt(f.Snapshot.Data.Details["relationFormat"]),
		}
		if key != "" {
			out[key] = def
		}
		if id != "" {
			out[id] = def
		}
	}
	return out, nil
}

func readOptions(dir string) (map[string]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read relation options dir: %w", err)
	}
	out := make(map[string]string)
	for _, ent := range entries {
		if ent.IsDir() || !strings.HasSuffix(ent.Name(), ".pb.json") {
			continue
		}
		f, err := readSnapshot(filepath.Join(dir, ent.Name()))
		if err != nil {
			return nil, err
		}
		id := asString(f.Snapshot.Data.Details["id"])
		if id == "" {
			continue
		}
		out[id] = asString(f.Snapshot.Data.Details["name"])
	}
	return out, nil
}

func readFileObjects(dir string) (map[string]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read filesObjects dir: %w", err)
	}
	out := make(map[string]string)
	for _, ent := range entries {
		if ent.IsDir() || !strings.HasSuffix(ent.Name(), ".pb.json") {
			continue
		}
		f, err := readSnapshot(filepath.Join(dir, ent.Name()))
		if err != nil {
			return nil, err
		}
		id := asString(f.Snapshot.Data.Details["id"])
		source := asString(f.Snapshot.Data.Details["source"])
		if id == "" {
			continue
		}
		if source != "" {
			out[id] = filepath.ToSlash(source)
			continue
		}
		fileExt := asString(f.Snapshot.Data.Details["fileExt"])
		name := asString(f.Snapshot.Data.Details["name"])
		if name == "" {
			name = id
		}
		if fileExt != "" {
			name = name + "." + fileExt
		}
		out[id] = filepath.ToSlash(filepath.Join("files", name))
	}
	return out, nil
}

func readSnapshot(path string) (snapshotFile, error) {
	var s snapshotFile
	b, err := os.ReadFile(path)
	if err != nil {
		return s, fmt.Errorf("read %s: %w", path, err)
	}
	if err := json.Unmarshal(b, &s); err != nil {
		return s, fmt.Errorf("decode %s: %w", path, err)
	}
	return s, nil
}

func renderFrontmatter(obj objectInfo, relations map[string]relationDef, optionsByID map[string]string, notes map[string]string, fileObjects map[string]string, includeDynamicProperties bool, includeArchivedProperties bool) string {
	keys := make([]string, 0, len(obj.Details))
	for k := range obj.Details {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var buf bytes.Buffer
	buf.WriteString("---\n")
	buf.WriteString("anytype_id: ")
	writeYAMLString(&buf, obj.ID)
	buf.WriteString("\n")

	usedKeys := map[string]struct{}{"anytype_id": {}}
	for _, k := range keys {
		rel, hasRel := relations[k]
		if !includeDynamicProperties {
			if _, dynamic := dynamicPropertyKeys[k]; dynamic {
				continue
			}
			if hasRel {
				if _, dynamic := dynamicPropertyKeys[rel.Key]; dynamic {
					continue
				}
			}
		}
		if !includeArchivedProperties && shouldSkipUnnamedProperty(k, rel, hasRel) {
			continue
		}
		v := obj.Details[k]
		converted := convertPropertyValue(k, v, relations, optionsByID, notes, fileObjects)
		outKey := frontmatterKey(k, rel, hasRel)
		if _, exists := usedKeys[outKey]; exists {
			outKey = k
		}
		usedKeys[outKey] = struct{}{}
		writeYAMLKeyValue(&buf, outKey, converted)
	}

	buf.WriteString("---\n\n")
	return buf.String()
}

func frontmatterKey(rawKey string, rel relationDef, hasRel bool) string {
	if !hasRel {
		return rawKey
	}
	if rel.Name == "" {
		return rawKey
	}
	if rel.Key != "" && rawKey != rel.Key {
		return rel.Name
	}
	return rawKey
}

func shouldSkipUnnamedProperty(key string, rel relationDef, hasRel bool) bool {
	if hasRel {
		return strings.TrimSpace(rel.Name) == ""
	}
	return isLikelyAnytypeObjectID(key)
}

func isLikelyAnytypeObjectID(s string) bool {
	if len(s) < 16 {
		return false
	}
	for _, r := range s {
		if (r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') {
			continue
		}
		return false
	}
	return true
}

func convertPropertyValue(key string, value any, relations map[string]relationDef, optionsByID map[string]string, notes map[string]string, fileObjects map[string]string) any {
	rel, hasRel := relations[key]
	if !hasRel {
		return value
	}

	switch rel.Format {
	case 100:
		ids := anyToStringSlice(value)
		if len(ids) == 0 {
			if s := asString(value); s != "" {
				ids = []string{s}
			}
		}
		if len(ids) == 0 {
			return value
		}
		out := make([]string, 0, len(ids))
		for _, id := range ids {
			if note, ok := notes[id]; ok {
				out = append(out, "[["+note+"]]")
			} else {
				out = append(out, id)
			}
		}
		if len(out) == 1 {
			return out[0]
		}
		return out
	case 11, 3:
		ids := anyToStringSlice(value)
		if len(ids) == 0 {
			if s := asString(value); s != "" {
				ids = []string{s}
			}
		}
		if len(ids) == 0 {
			return value
		}
		out := make([]string, 0, len(ids))
		for _, id := range ids {
			if n, ok := optionsByID[id]; ok && n != "" {
				out = append(out, n)
			} else {
				out = append(out, id)
			}
		}
		if len(out) == 1 {
			return out[0]
		}
		return out
	case 5:
		ids := anyToStringSlice(value)
		out := make([]string, 0, len(ids))
		for _, id := range ids {
			if src, ok := fileObjects[id]; ok {
				out = append(out, src)
			} else {
				out = append(out, id)
			}
		}
		if len(out) == 1 {
			return out[0]
		}
		if len(out) > 1 {
			return out
		}
		return value
	default:
		return value
	}
}

func renderBody(obj objectInfo, objects map[string]objectInfo, notes map[string]string, fileObjects map[string]string) string {
	byID := make(map[string]block, len(obj.Blocks))
	for _, b := range obj.Blocks {
		byID[b.ID] = b
	}

	root, ok := byID[obj.ID]
	if !ok {
		return ""
	}

	var buf bytes.Buffer
	for _, id := range root.ChildrenID {
		renderBlock(&buf, byID, id, notes, fileObjects, 0)
	}
	return strings.TrimLeft(buf.String(), "\n")
}

func renderBlock(buf *bytes.Buffer, byID map[string]block, id string, notes map[string]string, fileObjects map[string]string, depth int) {
	b, ok := byID[id]
	if !ok {
		return
	}

	if b.Text != nil {
		line := renderTextBlock(*b.Text, depth)
		if line != "" {
			buf.WriteString(line)
			if !strings.HasSuffix(line, "\n") {
				buf.WriteString("\n")
			}
		}
	} else if b.File != nil {
		path := fileObjects[b.File.TargetObjectID]
		if path == "" {
			path = filepath.ToSlash(filepath.Join("files", sanitizeName(strings.TrimSpace(b.File.Name))))
		}
		if strings.EqualFold(b.File.Type, "image") {
			buf.WriteString("![" + escapeBrackets(b.File.Name) + "](" + path + ")\n")
		} else {
			title := b.File.Name
			if title == "" {
				title = filepath.Base(path)
			}
			buf.WriteString("[" + escapeBrackets(title) + "](" + path + ")\n")
		}
	} else if b.Bookmark != nil {
		title := strings.TrimSpace(b.Bookmark.Title)
		if title == "" {
			title = b.Bookmark.URL
		}
		if b.Bookmark.URL != "" {
			buf.WriteString("[" + escapeBrackets(title) + "](" + b.Bookmark.URL + ")\n")
		}
	} else if b.Latex != nil {
		if strings.TrimSpace(b.Latex.Text) != "" {
			buf.WriteString("$$\n" + b.Latex.Text + "\n$$\n")
		}
	} else if b.Link != nil {
		if note, ok := notes[b.Link.TargetBlockID]; ok {
			buf.WriteString("[[" + note + "]]\n")
		}
	} else if b.Table != nil {
		table := renderTable(byID, b)
		if table != "" {
			buf.WriteString(table)
			if !strings.HasSuffix(table, "\n") {
				buf.WriteString("\n")
			}
		}
		return
	}

	for _, cid := range b.ChildrenID {
		renderBlock(buf, byID, cid, notes, fileObjects, depth+1)
	}
}

func renderTextBlock(t textBlock, depth int) string {
	text := strings.TrimRight(t.Text, "\n")
	style := t.Style
	indent := strings.Repeat("  ", max(0, depth-1))

	switch style {
	case "Title", "Header1", "ToggleHeader1":
		return "# " + text + "\n"
	case "Header2", "ToggleHeader2":
		return "## " + text + "\n"
	case "Header3", "ToggleHeader3":
		return "### " + text + "\n"
	case "Header4":
		return "#### " + text + "\n"
	case "Checkbox":
		if t.Checked {
			return indent + "- [x] " + text + "\n"
		}
		return indent + "- [ ] " + text + "\n"
	case "Marked":
		return indent + "- " + text + "\n"
	case "Numbered":
		return indent + "1. " + text + "\n"
	case "Code":
		return "```\n" + text + "\n```\n"
	case "Quote", "Toggle":
		return "> " + strings.ReplaceAll(text, "\n", "\n> ") + "\n"
	default:
		if strings.TrimSpace(text) == "" {
			return "\n"
		}
		return text + "\n"
	}
}

func renderTable(byID map[string]block, tableBlock block) string {
	var colsBlock block
	var rowsBlock block
	foundCols := false
	foundRows := false

	for _, cid := range tableBlock.ChildrenID {
		c, ok := byID[cid]
		if !ok || c.Layout == nil {
			continue
		}
		if c.Layout.Style == "TableColumns" {
			colsBlock = c
			foundCols = true
		}
		if c.Layout.Style == "TableRows" {
			rowsBlock = c
			foundRows = true
		}
	}

	if !foundCols || !foundRows {
		return ""
	}

	colCount := len(colsBlock.ChildrenID)
	if colCount == 0 {
		return ""
	}

	rows := make([][]string, 0, len(rowsBlock.ChildrenID))
	for _, rid := range rowsBlock.ChildrenID {
		rb, ok := byID[rid]
		if !ok {
			continue
		}
		row := make([]string, colCount)
		for i := 0; i < colCount; i++ {
			if i < len(rb.ChildrenID) {
				row[i] = extractPlainText(byID, rb.ChildrenID[i])
			} else {
				row[i] = ""
			}
			if row[i] == "" {
				row[i] = " "
			}
		}
		rows = append(rows, row)
	}

	if len(rows) == 0 {
		return ""
	}

	var buf bytes.Buffer
	header := rows[0]
	writeMarkdownTableRow(&buf, header)
	sep := make([]string, len(header))
	for i := range sep {
		sep[i] = "---"
	}
	writeMarkdownTableRow(&buf, sep)
	for i := 1; i < len(rows); i++ {
		writeMarkdownTableRow(&buf, rows[i])
	}
	return buf.String()
}

func writeMarkdownTableRow(buf *bytes.Buffer, row []string) {
	buf.WriteString("|")
	for _, c := range row {
		cell := strings.ReplaceAll(c, "|", "\\|")
		cell = strings.ReplaceAll(cell, "\n", " ")
		buf.WriteString(" " + strings.TrimSpace(cell) + " |")
	}
	buf.WriteString("\n")
}

func extractPlainText(byID map[string]block, id string) string {
	b, ok := byID[id]
	if !ok {
		return ""
	}
	if b.Text != nil {
		return strings.TrimSpace(b.Text.Text)
	}
	if b.Bookmark != nil {
		if strings.TrimSpace(b.Bookmark.Title) != "" {
			return strings.TrimSpace(b.Bookmark.Title)
		}
		return strings.TrimSpace(b.Bookmark.URL)
	}
	if b.File != nil {
		return strings.TrimSpace(b.File.Name)
	}
	var parts []string
	for _, cid := range b.ChildrenID {
		t := extractPlainText(byID, cid)
		if t != "" {
			parts = append(parts, t)
		}
	}
	return strings.TrimSpace(strings.Join(parts, " "))
}

func writeYAMLKeyValue(buf *bytes.Buffer, key string, value any) {
	if key == "" {
		return
	}
	safeKey := sanitizeYAMLKey(key)
	buf.WriteString(safeKey)
	buf.WriteString(":")
	writeYAMLValue(buf, value, 0)
	buf.WriteString("\n")
}

func writeYAMLValue(buf *bytes.Buffer, value any, indent int) {
	switch v := value.(type) {
	case nil:
		buf.WriteString(" null")
	case string:
		buf.WriteString(" ")
		writeYAMLString(buf, v)
	case bool:
		if v {
			buf.WriteString(" true")
		} else {
			buf.WriteString(" false")
		}
	case float64:
		buf.WriteString(" ")
		buf.WriteString(strconv.FormatFloat(v, 'f', -1, 64))
	case int:
		buf.WriteString(" ")
		buf.WriteString(strconv.Itoa(v))
	case []string:
		if len(v) == 0 {
			buf.WriteString(" []")
			return
		}
		for _, item := range v {
			buf.WriteString("\n")
			buf.WriteString(strings.Repeat("  ", indent+1))
			buf.WriteString("- ")
			writeYAMLString(buf, item)
		}
	case []any:
		if len(v) == 0 {
			buf.WriteString(" []")
			return
		}
		primitive := true
		for _, it := range v {
			switch it.(type) {
			case string, float64, bool, int:
			default:
				primitive = false
			}
		}
		if primitive {
			for _, item := range v {
				buf.WriteString("\n")
				buf.WriteString(strings.Repeat("  ", indent+1))
				buf.WriteString("- ")
				switch iv := item.(type) {
				case string:
					writeYAMLString(buf, iv)
				case float64:
					buf.WriteString(strconv.FormatFloat(iv, 'f', -1, 64))
				case bool:
					if iv {
						buf.WriteString("true")
					} else {
						buf.WriteString("false")
					}
				case int:
					buf.WriteString(strconv.Itoa(iv))
				}
			}
		} else {
			b, _ := json.Marshal(v)
			buf.WriteString(" ")
			writeYAMLString(buf, string(b))
		}
	default:
		b, _ := json.Marshal(v)
		if string(b) == "" {
			buf.WriteString(" null")
			return
		}
		buf.WriteString(" ")
		writeYAMLString(buf, string(b))
	}
}

func writeYAMLString(buf *bytes.Buffer, s string) {
	escaped := strings.ReplaceAll(s, "\\", "\\\\")
	escaped = strings.ReplaceAll(escaped, "\"", "\\\"")
	escaped = strings.ReplaceAll(escaped, "\n", "\\n")
	buf.WriteString("\"")
	buf.WriteString(escaped)
	buf.WriteString("\"")
}

func sanitizeYAMLKey(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "field"
	}
	return s
}

func sanitizeName(s string) string {
	s = strings.TrimSpace(strings.ToLower(s))
	if s == "" {
		return ""
	}
	var b strings.Builder
	prevDash := false
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			prevDash = false
			continue
		}
		if !prevDash {
			b.WriteRune('-')
			prevDash = true
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return "untitled"
	}
	return out
}

func asString(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
	case int:
		return strconv.Itoa(t)
	default:
		return ""
	}
}

func asInt(v any) int {
	switch t := v.(type) {
	case float64:
		return int(t)
	case int:
		return t
	case string:
		i, _ := strconv.Atoi(t)
		return i
	default:
		return 0
	}
}

func anyToStringSlice(v any) []string {
	switch t := v.(type) {
	case []string:
		return t
	case []any:
		out := make([]string, 0, len(t))
		for _, item := range t {
			s := asString(item)
			if s != "" {
				out = append(out, s)
			}
		}
		return out
	case string:
		if t == "" {
			return nil
		}
		return []string{t}
	default:
		return nil
	}
}

func escapeBrackets(s string) string {
	s = strings.ReplaceAll(s, "[", "\\[")
	s = strings.ReplaceAll(s, "]", "\\]")
	return s
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func copyDir(src, dst string) (int, error) {
	entries, err := os.ReadDir(src)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, fmt.Errorf("read dir %s: %w", src, err)
	}
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return 0, err
	}

	copied := 0
	for _, ent := range entries {
		if ent.IsDir() {
			continue
		}
		inPath := filepath.Join(src, ent.Name())
		outPath := filepath.Join(dst, ent.Name())
		if err := copyFile(inPath, outPath); err != nil {
			return copied, err
		}
		copied++
	}
	return copied, nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Sync()
}
