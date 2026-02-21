package exporter

import (
	"bytes"
	"crypto/sha1"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"unicode"

	anytypedomain "github.com/sleroq/anytype-to-obsidian/internal/domain/anytype"
)

const iconizeAnytypePackName = "anytype"
const iconizeAnytypePackPrefix = "An"

func renderBody(obj objectInfo, objects map[string]objectInfo, notes map[string]string, sourceNotePath string, fileObjects map[string]string, excalidrawEmbeds map[string]string) string {
	byID := make(map[string]block, len(obj.Blocks))
	for _, b := range obj.Blocks {
		byID[b.ID] = b
	}

	root, ok := byID[obj.ID]
	if !ok {
		return ""
	}

	var buf bytes.Buffer
	renderChildren(&buf, byID, root.ChildrenID, notes, sourceNotePath, fileObjects, excalidrawEmbeds, 0, obj.ID)
	return strings.TrimLeft(buf.String(), "\n")
}

func renderChildren(buf *bytes.Buffer, byID map[string]block, children []string, notes map[string]string, sourceNotePath string, fileObjects map[string]string, excalidrawEmbeds map[string]string, depth int, rootID string) {
	numberedIndex := 0
	for _, id := range children {
		b, ok := byID[id]
		if ok && b.Text != nil && b.Text.Style == "Numbered" {
			numberedIndex++
		} else {
			numberedIndex = 0
		}
		renderBlock(buf, byID, id, notes, sourceNotePath, fileObjects, excalidrawEmbeds, depth, rootID, numberedIndex)
	}
}

func renderTemplate(tmpl templateInfo, relations map[string]relationDef, objects map[string]objectInfo, notes map[string]string, fileObjects map[string]string, pictureToCover bool) string {
	keys := collectTemplateRelationKeys(tmpl)

	var buf bytes.Buffer
	buf.WriteString("---\n")

	used := map[string]struct{}{}
	for _, raw := range keys {
		rel, hasRel := relations[raw]
		outKey := frontmatterKey(raw, rel, hasRel, pictureToCover)
		if outKey == "" {
			outKey = raw
		}
		if _, exists := used[outKey]; exists {
			continue
		}
		used[outKey] = struct{}{}
		writeYAMLKeyValue(&buf, outKey, nil)
	}
	buf.WriteString("---\n\n")

	body := renderBody(objectInfo{ID: tmpl.ID, Name: tmpl.Name, Details: tmpl.Details, Blocks: tmpl.Blocks}, objects, notes, "", fileObjects, nil)
	buf.WriteString(body)
	return buf.String()
}

func collectTemplateRelationKeys(tmpl templateInfo) []string {
	byID := make(map[string]block, len(tmpl.Blocks))
	for _, b := range tmpl.Blocks {
		byID[b.ID] = b
	}
	root, ok := byID[tmpl.ID]
	if !ok {
		return nil
	}

	ordered := make([]string, 0)
	seen := make(map[string]struct{})
	var visit func(string)
	visit = func(id string) {
		b, ok := byID[id]
		if !ok {
			return
		}
		if b.Relation != nil {
			key := strings.TrimSpace(b.Relation.Key)
			if key != "" {
				if _, exists := seen[key]; !exists {
					seen[key] = struct{}{}
					ordered = append(ordered, key)
				}
			}
		}
		for _, cid := range b.ChildrenID {
			visit(cid)
		}
	}

	for _, id := range root.ChildrenID {
		visit(id)
	}
	return ordered
}

func renderBlock(buf *bytes.Buffer, byID map[string]block, id string, notes map[string]string, sourceNotePath string, fileObjects map[string]string, excalidrawEmbeds map[string]string, depth int, rootID string, numberedIndex int) {
	b, ok := byID[id]
	if !ok {
		return
	}

	if isSystemTitleBlock(b) {
		return
	}

	if b.Text != nil && (b.Text.Style == "Callout" || b.Text.Style == "Toggle") {
		renderCalloutBlock(buf, byID, b, notes, sourceNotePath, fileObjects, excalidrawEmbeds, depth, rootID)
		return
	}

	if b.Text != nil {
		line := renderTextBlock(*b.Text, depth, b.Fields, notes, sourceNotePath, numberedIndex)
		if line != "" {
			buf.WriteString(line)
			if !strings.HasSuffix(line, "\n") {
				buf.WriteString("\n")
			}
		}
	} else if b.File != nil {
		path := fileObjects[b.File.TargetObjectID]
		if path == "" {
			path = filepath.ToSlash(filepath.Join("files", sanitizeName(strings.TrimSpace(b.File.Name), "posix")))
		}
		path = relativePathTarget(sourceNotePath, path)
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
		if embedTarget, ok := excalidrawEmbeds[b.ID]; ok && embedTarget != "" {
			buf.WriteString("![[" + embedTarget + "]]\n")
		} else if strings.TrimSpace(b.Latex.Text) != "" {
			buf.WriteString("$$\n" + b.Latex.Text + "\n$$\n")
		}
	} else if len(b.Dataview) > 0 {
		dataviewTargetID := rootID
		if target := strings.TrimSpace(asString(anyMapGet(b.Dataview, "TargetObjectId", "targetObjectId"))); target != "" {
			dataviewTargetID = target
		}
		if note, ok := notes[dataviewTargetID]; ok && strings.HasPrefix(filepath.ToSlash(strings.TrimSpace(note)), "bases/") {
			buf.WriteString("![[" + relativeWikiTarget(sourceNotePath, note) + "]]\n")
		}
	} else if b.Link != nil {
		if note, ok := notes[b.Link.TargetBlockID]; ok {
			buf.WriteString("[[" + relativeWikiTarget(sourceNotePath, note) + "]]\n")
		} else if date := linkTargetDate(b.Link.TargetBlockID); date != "" {
			buf.WriteString(date + "\n")
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
	} else if b.Div != nil {
		if divider := renderDivider(b.Div); divider != "" {
			buf.WriteString(divider + "\n")
		}
	} else if b.TOC != nil {
		toc := renderTableOfContents(byID, rootID)
		if toc != "" {
			buf.WriteString(toc)
		}
	}

	renderChildren(buf, byID, b.ChildrenID, notes, sourceNotePath, fileObjects, excalidrawEmbeds, depth+1, rootID)
}

func isSystemTitleBlock(b block) bool {
	if b.Text == nil || b.Text.Style != "Title" {
		return false
	}
	for _, key := range anyToStringSlice(b.Fields["_detailsKey"]) {
		if strings.EqualFold(strings.TrimSpace(key), "name") {
			return true
		}
	}
	return false
}

func renderTextBlock(t textBlock, depth int, fields map[string]any, notes map[string]string, sourceNotePath string, numberedIndex int) string {
	text := strings.TrimRight(t.Text, "\n")
	text = applyTextMarks(text, t.Marks, notes, sourceNotePath)
	style := t.Style
	indent := strings.Repeat("\t", max(0, depth-1))

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
		if numberedIndex <= 0 {
			numberedIndex = 1
		}
		return indent + strconv.Itoa(numberedIndex) + ". " + text + "\n"
	case "Code":
		code := strings.TrimLeft(text, "\n")
		lang := strings.TrimSpace(asString(fields["lang"]))
		if lang != "" {
			return "```" + lang + "\n" + code + "\n```\n"
		}
		return "```\n" + code + "\n```\n"
	case "Quote":
		return "> " + strings.ReplaceAll(text, "\n", "\n> ") + "\n"
	default:
		if strings.TrimSpace(text) == "" {
			return "\n"
		}
		return text + "\n"
	}
}

func applyTextMarks(text string, marks *anytypedomain.TextMarks, notes map[string]string, sourceNotePath string) string {
	if strings.TrimSpace(text) == "" || marks == nil || len(marks.Marks) == 0 {
		return text
	}

	type replacementMark struct {
		from int
		to   int
		repl string
	}

	runes := []rune(text)
	replacements := make([]replacementMark, 0, len(marks.Marks))
	for _, mark := range marks.Marks {
		from := mark.Range.From
		to := mark.Range.To
		if from < 0 {
			from = 0
		}
		if to > len(runes) {
			to = len(runes)
		}
		if to <= from {
			continue
		}

		markType := strings.ToLower(strings.TrimSpace(mark.Type))
		switch markType {
		case "mention":
			note := notes[strings.TrimSpace(mark.Param)]
			if note == "" {
				continue
			}
			replacements = append(replacements, replacementMark{from: from, to: to, repl: "[[" + relativeWikiTarget(sourceNotePath, note) + "]]"})
		case "link":
			url := strings.TrimSpace(mark.Param)
			if url == "" {
				continue
			}
			label := strings.TrimSpace(string(runes[from:to]))
			if label == "" {
				label = url
			}
			replacements = append(replacements, replacementMark{from: from, to: to, repl: "[" + escapeBrackets(label) + "](" + url + ")"})
		}
	}
	if len(replacements) == 0 {
		return text
	}

	sort.Slice(replacements, func(i, j int) bool {
		if replacements[i].from == replacements[j].from {
			return replacements[i].to < replacements[j].to
		}
		return replacements[i].from < replacements[j].from
	})

	var out strings.Builder
	cursor := 0
	for _, replacement := range replacements {
		if replacement.from < cursor {
			continue
		}
		out.WriteString(string(runes[cursor:replacement.from]))
		out.WriteString(replacement.repl)
		cursor = replacement.to
	}
	out.WriteString(string(runes[cursor:]))
	return out.String()
}

func renderCalloutBlock(buf *bytes.Buffer, byID map[string]block, b block, notes map[string]string, sourceNotePath string, fileObjects map[string]string, excalidrawEmbeds map[string]string, depth int, rootID string) {
	if b.Text == nil {
		return
	}
	if depth == 0 && buf.Len() > 0 && !bytes.HasSuffix(buf.Bytes(), []byte("\n\n")) {
		buf.WriteString("\n")
	}
	marker := "> [!note]"
	if b.Text.Style == "Toggle" {
		marker += "-"
	}
	title := strings.TrimSpace(b.Text.Text)
	if title != "" {
		marker += " " + title
	}
	buf.WriteString(marker + "\n")

	var child bytes.Buffer
	renderChildren(&child, byID, b.ChildrenID, notes, sourceNotePath, fileObjects, excalidrawEmbeds, depth+1, rootID)
	body := strings.TrimRight(child.String(), "\n")
	if body == "" {
		buf.WriteString("\n")
		return
	}
	buf.WriteString(prefixLines(body, "> "))
	buf.WriteString("\n\n")
}

func exportExcalidrawDrawings(obj objectInfo, noteRelPath string, excalidrawDir string, filenameEscaping string, usedNames map[string]int) (map[string]string, error) {
	embeds := map[string]string{}
	noteBase := strings.TrimSpace(strings.TrimSuffix(filepath.Base(noteRelPath), filepath.Ext(noteRelPath)))
	if noteBase == "" {
		noteBase = sanitizeName(obj.ID, filenameEscaping)
	}
	drawingIndex := 0

	for _, b := range obj.Blocks {
		if b.Latex == nil {
			continue
		}
		if !strings.EqualFold(strings.TrimSpace(b.Latex.Processor), "Excalidraw") {
			continue
		}
		drawingData := strings.TrimSpace(b.Latex.Text)
		if drawingData == "" {
			continue
		}

		drawingContent, err := renderExcalidrawFile(drawingData)
		if err != nil {
			continue
		}

		drawingIndex++
		baseName := sanitizeName(noteBase+" drawing", filenameEscaping)
		if baseName == "" {
			baseName = sanitizeName(obj.ID+" drawing", filenameEscaping)
		}
		if baseName == "" {
			baseName = "drawing"
		}
		if drawingIndex > 1 {
			baseName = baseName + "-" + strconv.Itoa(drawingIndex)
		}

		usedKey := filenameCollisionKey(baseName, filenameEscaping)
		n := usedNames[usedKey]
		usedNames[usedKey] = n + 1
		if n > 0 {
			baseName = baseName + "-" + strconv.Itoa(n+1)
		}

		drawingFilename := baseName + ".excalidraw.md"
		drawingPath := filepath.Join(excalidrawDir, drawingFilename)
		if err := os.WriteFile(drawingPath, []byte(drawingContent), 0o644); err != nil {
			return nil, err
		}
		if err := applyExportedFileTimes(drawingPath, obj.Details); err != nil {
			return nil, err
		}

		embeds[b.ID] = filepath.ToSlash(filepath.Join("Excalidraw", strings.TrimSuffix(drawingFilename, ".md")))
	}

	if len(embeds) == 0 {
		return nil, nil
	}
	return embeds, nil
}

func renderExcalidrawFile(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", fmt.Errorf("empty excalidraw payload")
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return "", err
	}
	normalizeExcalidrawPayload(payload)
	pretty, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return "", err
	}

	var buf bytes.Buffer
	buf.WriteString("---\n\n")
	buf.WriteString("excalidraw-plugin: parsed\n")
	buf.WriteString("tags: [excalidraw]\n\n")
	buf.WriteString("---\n")
	buf.WriteString("==⚠  Switch to EXCALIDRAW VIEW in the MORE OPTIONS menu of this document. ⚠== You can decompress Drawing data with the command palette: 'Decompress current Excalidraw file'. For more info check in plugin settings under 'Saving'\n\n")
	buf.WriteString("# Excalidraw Data\n\n")
	buf.WriteString("## Text Elements\n")
	buf.WriteString("%%\n")
	buf.WriteString("## Drawing\n")
	buf.WriteString("```json\n")
	buf.Write(pretty)
	buf.WriteString("\n```\n")
	buf.WriteString("%%\n")

	return buf.String(), nil
}

func normalizeExcalidrawPayload(payload map[string]any) {
	if payload == nil {
		return
	}

	if _, ok := payload["type"]; !ok {
		payload["type"] = "excalidraw"
	}
	if _, ok := payload["version"]; !ok {
		payload["version"] = float64(2)
	}
	if _, ok := payload["source"]; !ok {
		payload["source"] = "https://excalidraw.com"
	}

	if _, ok := payload["elements"].([]any); !ok {
		payload["elements"] = []any{}
	}
	if _, ok := payload["files"].(map[string]any); !ok {
		payload["files"] = map[string]any{}
	}

	appState, ok := payload["appState"].(map[string]any)
	if !ok {
		appState = map[string]any{}
		payload["appState"] = appState
	}
	if _, ok := appState["collaborators"].([]any); !ok {
		appState["collaborators"] = []any{}
	}
}

func prefixLines(s string, prefix string) string {
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		if strings.TrimSpace(line) == "" {
			lines[i] = strings.TrimSpace(prefix)
		} else {
			lines[i] = prefix + line
		}
	}
	return strings.Join(lines, "\n")
}

func renderDivider(div map[string]any) string {
	switch strings.ToLower(strings.TrimSpace(asString(div["style"]))) {
	case "line":
		return "---"
	case "dots":
		return "***"
	default:
		return ""
	}
}

func renderTableOfContents(byID map[string]block, rootID string) string {
	root, ok := byID[rootID]
	if !ok {
		return ""
	}

	type heading struct {
		level int
		text  string
	}
	headings := make([]heading, 0)
	var visit func(string)
	visit = func(id string) {
		b, ok := byID[id]
		if !ok {
			return
		}
		if b.Text != nil {
			if level := headingLevel(b.Text.Style); level > 0 {
				text := strings.TrimSpace(b.Text.Text)
				if text != "" {
					headings = append(headings, heading{level: level, text: text})
				}
			}
		}
		for _, cid := range b.ChildrenID {
			visit(cid)
		}
	}

	for _, cid := range root.ChildrenID {
		visit(cid)
	}
	if len(headings) == 0 {
		return ""
	}

	var buf bytes.Buffer
	for _, h := range headings {
		slug := headingSlug(h.text)
		if slug == "" {
			continue
		}
		indent := strings.Repeat("\t", max(0, h.level-1))
		buf.WriteString(indent + "- [" + escapeBrackets(h.text) + "](#" + slug + ")\n")
	}
	return buf.String()
}

func headingLevel(style string) int {
	switch style {
	case "Header1", "ToggleHeader1":
		return 1
	case "Header2", "ToggleHeader2":
		return 2
	case "Header3", "ToggleHeader3":
		return 3
	case "Header4":
		return 4
	default:
		return 0
	}
}

func headingSlug(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" {
		return ""
	}
	var b strings.Builder
	lastDash := false
	for _, r := range s {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(r)
			lastDash = false
		case r == ' ' || r == '-':
			if !lastDash {
				b.WriteRune('-')
				lastDash = true
			}
		}
	}
	return strings.Trim(b.String(), "-")
}

func linkTargetDate(target string) string {
	const prefix = "_date_"
	if strings.HasPrefix(target, prefix) {
		return strings.TrimPrefix(target, prefix)
	}
	return ""
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

func exportPrettyPropertiesPluginData(outputDir string, relations map[string]relationDef, optionsByID map[string]relationOption) error {
	colorByList := map[string]map[string]string{
		"tagColors":              {},
		"propertyPillColors":     {},
		"propertyLongtextColors": {},
	}

	for _, option := range optionsByID {
		name := strings.TrimSpace(option.Name)
		if name == "" {
			continue
		}
		mappedColor, ok := mapAnytypePrettyPropertiesColor(asString(option.Details["relationOptionColor"]))
		if !ok {
			continue
		}

		relationKey := strings.TrimSpace(asString(option.Details["relationKey"]))
		rel, hasRel := relations[relationKey]
		listKey := prettyPropertiesColorListForOption(relationKey, rel, hasRel)
		if listKey == "" {
			continue
		}
		if listKey == "tagColors" {
			name = sanitizeObsidianTag(name)
			if name == "" {
				continue
			}
		}
		colorByList[listKey][name] = mappedColor
	}

	totalColors := 0
	for _, values := range colorByList {
		totalColors += len(values)
	}
	if totalColors == 0 {
		return nil
	}

	dataPath := filepath.Join(outputDir, ".obsidian", "plugins", "pretty-properties", "data.json")
	if err := os.MkdirAll(filepath.Dir(dataPath), 0o755); err != nil {
		return err
	}

	data := map[string]any{}
	if raw, err := os.ReadFile(dataPath); err == nil {
		if err := json.Unmarshal(raw, &data); err != nil {
			return fmt.Errorf("decode %s: %w", dataPath, err)
		}
	} else if !os.IsNotExist(err) {
		return err
	}

	changed := false
	if normalizePrettyPropertiesTagColorKeys(data) {
		changed = true
	}
	for listKey, values := range colorByList {
		colorList := ensureAnyMap(data, listKey)
		for valueName, color := range values {
			if upsertPrettyPropertiesColor(colorList, valueName, color) {
				changed = true
			}
		}
	}

	if !changed {
		return nil
	}

	encoded, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(dataPath, encoded, 0o644)
}

func normalizePrettyPropertiesTagColorKeys(data map[string]any) bool {
	tagColors, ok := data["tagColors"].(map[string]any)
	if !ok || len(tagColors) == 0 {
		return false
	}

	type rename struct {
		from string
		to   string
	}
	renames := make([]rename, 0)
	for key := range tagColors {
		normalized := sanitizeObsidianTag(key)
		if normalized == "" || normalized == key {
			continue
		}
		renames = append(renames, rename{from: key, to: normalized})
	}

	if len(renames) == 0 {
		return false
	}

	changed := false
	for _, item := range renames {
		if _, exists := tagColors[item.to]; !exists {
			tagColors[item.to] = tagColors[item.from]
			changed = true
		}
		delete(tagColors, item.from)
		changed = true
	}

	return changed
}

func ensureAnyMap(data map[string]any, key string) map[string]any {
	if existing, ok := data[key].(map[string]any); ok {
		return existing
	}
	created := map[string]any{}
	data[key] = created
	return created
}

func upsertPrettyPropertiesColor(colorList map[string]any, valueName string, color string) bool {
	existingEntry, ok := colorList[valueName]
	if !ok {
		colorList[valueName] = map[string]any{"pillColor": color, "textColor": "default"}
		return true
	}

	entry, ok := existingEntry.(map[string]any)
	if !ok {
		return false
	}

	if existing, ok := entry["pillColor"].(string); ok && strings.TrimSpace(existing) != "" {
		return false
	}

	entry["pillColor"] = color
	if textColor, ok := entry["textColor"].(string); !ok || strings.TrimSpace(textColor) == "" {
		entry["textColor"] = "default"
	}
	return true
}

func mapAnytypePrettyPropertiesColor(raw string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "red", "orange", "yellow", "green", "cyan", "blue", "purple", "pink", "none", "default":
		return strings.ToLower(strings.TrimSpace(raw)), true
	case "lime":
		return "green", true
	case "teal":
		return "cyan", true
	case "ice":
		return "blue", true
	case "grey", "gray":
		return "default", true
	default:
		return "", false
	}
}

func prettyPropertiesColorListForOption(rawRelationKey string, rel relationDef, hasRel bool) string {
	if isTagProperty(rawRelationKey, rel, hasRel) {
		return "tagColors"
	}
	if hasRel {
		if rel.Format == anytypedomain.RelationFormatStatus || rel.Max == 1 {
			return "propertyLongtextColors"
		}
	}
	return "propertyPillColors"
}

func exportIconizePluginData(inputDir string, outputDir string, objects []objectInfo, notePathByID map[string]string, fileObjects map[string]string) error {
	iconByPath := make(map[string]string)
	imageIconRefs := make(map[string]string)

	for _, obj := range objects {
		noteRelPath := strings.TrimSpace(notePathByID[obj.ID])
		if noteRelPath == "" {
			continue
		}

		iconValue := strings.TrimSpace(asString(obj.Details["iconEmoji"]))
		if iconValue == "" {
			imageID := strings.TrimSpace(asString(obj.Details["iconImage"]))
			if imageID != "" {
				imageIcon, err := ensureIconizeImageIcon(inputDir, outputDir, imageID, fileObjects, imageIconRefs)
				if err != nil {
					return err
				}
				iconValue = imageIcon
			}
		}

		if iconValue == "" {
			continue
		}
		iconByPath[noteRelPath] = iconValue
	}

	if len(iconByPath) == 0 {
		return nil
	}

	dataPath := filepath.Join(outputDir, ".obsidian", "plugins", "obsidian-icon-folder", "data.json")
	if err := os.MkdirAll(filepath.Dir(dataPath), 0o755); err != nil {
		return err
	}

	data := map[string]any{}
	if raw, err := os.ReadFile(dataPath); err == nil {
		if err := json.Unmarshal(raw, &data); err != nil {
			return fmt.Errorf("decode %s: %w", dataPath, err)
		}
	} else if !os.IsNotExist(err) {
		return err
	}

	if _, ok := data["settings"]; !ok {
		data["settings"] = defaultIconizeSettings()
	}

	for notePath, iconName := range iconByPath {
		data[notePath] = iconName
	}

	encoded, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(dataPath, encoded, 0o644)
}

func ensureIconizeImageIcon(inputDir string, outputDir string, imageID string, fileObjects map[string]string, refs map[string]string) (string, error) {
	if existing := strings.TrimSpace(refs[imageID]); existing != "" {
		return existing, nil
	}

	sourceRelPath := strings.TrimSpace(fileObjects[imageID])
	if sourceRelPath == "" {
		return "", nil
	}

	absoluteSourcePath := filepath.Join(inputDir, filepath.FromSlash(sourceRelPath))
	content, err := os.ReadFile(absoluteSourcePath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	if len(content) == 0 {
		return "", nil
	}

	iconDir := filepath.Join(outputDir, ".obsidian", "icons", iconizeAnytypePackName)
	if err := os.MkdirAll(iconDir, 0o755); err != nil {
		return "", err
	}

	iconName := iconizeImageIconName(imageID)
	iconSVG := wrapBinaryImageAsSVG(content, detectImageMIME(content, absoluteSourcePath))
	if err := os.WriteFile(filepath.Join(iconDir, iconName+".svg"), []byte(iconSVG), 0o644); err != nil {
		return "", err
	}

	iconRef := iconizeAnytypePackPrefix + iconName
	refs[imageID] = iconRef
	return iconRef, nil
}

func iconizeImageIconName(imageID string) string {
	hash := sha1.Sum([]byte(strings.TrimSpace(imageID)))
	encoded := strings.ToUpper(hex.EncodeToString(hash[:4]))
	return "AnytypeIcon" + encoded
}

func detectImageMIME(content []byte, sourcePath string) string {
	if len(content) > 0 {
		sniffLen := len(content)
		if sniffLen > 512 {
			sniffLen = 512
		}
		mime := strings.TrimSpace(http.DetectContentType(content[:sniffLen]))
		if mime != "" && mime != "application/octet-stream" {
			return mime
		}
	}

	ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(sourcePath), "."))
	switch ext {
	case "svg":
		return "image/svg+xml"
	case "ico":
		return "image/x-icon"
	case "png":
		return "image/png"
	case "jpg", "jpeg":
		return "image/jpeg"
	case "webp":
		return "image/webp"
	case "gif":
		return "image/gif"
	default:
		return "application/octet-stream"
	}
}

func wrapBinaryImageAsSVG(content []byte, mimeType string) string {
	encoded := base64.StdEncoding.EncodeToString(content)
	return fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 48 48"><image href="data:%s;base64,%s" width="48" height="48" preserveAspectRatio="xMidYMid meet" /></svg>`, mimeType, encoded)
}

func defaultIconizeSettings() map[string]any {
	return map[string]any{
		"migrated":                        6,
		"iconPacksPath":                   ".obsidian/icons",
		"fontSize":                        16,
		"emojiStyle":                      "native",
		"iconColor":                       nil,
		"recentlyUsedIcons":               []string{},
		"recentlyUsedIconsSize":           5,
		"rules":                           []any{},
		"extraMargin":                     map[string]any{"top": 0, "right": 4, "bottom": 0, "left": 0},
		"iconInTabsEnabled":               false,
		"iconInTitleEnabled":              false,
		"iconInTitlePosition":             "above",
		"iconInFrontmatterEnabled":        false,
		"iconInFrontmatterFieldName":      "icon",
		"iconColorInFrontmatterFieldName": "iconColor",
		"iconsBackgroundCheckEnabled":     false,
		"iconsInNotesEnabled":             true,
		"iconsInLinksEnabled":             true,
		"iconIdentifier":                  ":",
		"lucideIconPackType":              "native",
		"debugMode":                       false,
		"useInternalPlugins":              false,
	}
}
