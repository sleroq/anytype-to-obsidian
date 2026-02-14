package exporter

import (
	"bytes"
	"crypto/sha1"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/charmbracelet/bubbles/progress"
	anytypedomain "github.com/sleroq/anytype-to-obsidian/internal/domain/anytype"
	"github.com/sleroq/anytype-to-obsidian/internal/infra/anytypejson"
)

type Exporter struct {
	InputDir                  string
	OutputDir                 string
	DisableIconizeIcons       bool
	RunPrettier               bool
	FilenameEscaping          string
	IncludeDynamicProperties  bool
	IncludeArchivedProperties bool
	ExcludeEmptyProperties    bool
	ExcludePropertyKeys       []string
	ForceIncludePropertyKeys  []string
	LinkAsNotePropertyKeys    []string
}

type Stats struct {
	Notes int
	Files int
}

type block = anytypedomain.Block
type textBlock = anytypedomain.TextBlock
type fileBlock = anytypedomain.FileBlock
type bookmarkBlock = anytypedomain.BookmarkBlock
type latexBlock = anytypedomain.LatexBlock
type linkBlock = anytypedomain.LinkBlock
type layoutBlock = anytypedomain.LayoutBlock
type relationBlock = anytypedomain.RelationBlock
type relationDef = anytypedomain.RelationDef
type typeDef = anytypedomain.TypeDef
type relationOption = anytypedomain.RelationOption
type objectInfo = anytypedomain.ObjectInfo
type templateInfo = anytypedomain.TemplateInfo

type indexFile struct {
	Notes map[string]string `json:"notes"`
}

var prettierCommandRunner = func(outputDir string) error {
	targets := make([]string, 0, 3)
	for _, dir := range []string{"notes", "bases", "templates"} {
		abs := filepath.Join(outputDir, dir)
		info, err := os.Stat(abs)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return err
		}
		if info.IsDir() {
			targets = append(targets, dir)
		}
	}
	if len(targets) == 0 {
		return nil
	}

	args := []string{"--yes", "prettier", "--no-config", "--write", "--ignore-unknown"}
	args = append(args, targets...)
	cmd := exec.Command("npx", args...)
	cmd.Dir = outputDir
	out, err := cmd.CombinedOutput()
	if err == nil {
		return nil
	}
	msg := strings.TrimSpace(string(out))
	if msg == "" {
		return err
	}
	return fmt.Errorf("%w: %s", err, msg)
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

var defaultHiddenPropertyKeys = map[string]struct{}{
	"creator":                {},
	"coverX":                 {},
	"coverY":                 {},
	"coverType":              {},
	"coverScale":             {},
	"coverId":                {},
	"oldAnytypeID":           {},
	"origin":                 {},
	"createdDate":            {},
	"featuredRelations":      {},
	"id":                     {},
	"importType":             {},
	"internalFlags":          {},
	"layout":                 {},
	"layoutAlign":            {},
	"resolvedLayout":         {},
	"snippet":                {},
	"name":                   {},
	"restrictions":           {},
	"sourceObject":           {},
	"spaceId":                {},
	"anytype_id":             {},
	"anytype_template_id":    {},
	"anytype_target_type_id": {},
	"anytype_target_type":    {},
	"sourceFilePath":         {},
}

type propertyFilters struct {
	exclude      map[string]struct{}
	forceInclude map[string]struct{}
	linkAsNote   map[string]struct{}
	excludeEmpty bool
}

var createdDateKeys = []string{"createdDate", "addedDate"}
var changedDateKeys = []string{"changedDate"}
var modifiedDateKeys = []string{"lastModifiedDate", "modifiedDate"}

type exportProgressBar struct {
	enabled         bool
	total           int
	current         int
	lastRenderWidth int
	label           string
	bar             progress.Model
}

func newExportProgressBar(total int) exportProgressBar {
	if total <= 0 {
		total = 1
	}
	bar := progress.New(progress.WithDefaultGradient(), progress.WithoutPercentage())
	bar.Width = 36

	if cols, err := strconv.Atoi(strings.TrimSpace(os.Getenv("COLUMNS"))); err == nil && cols > 0 {
		width := cols - 40
		if width < 16 {
			width = 16
		}
		if width > 64 {
			width = 64
		}
		bar.Width = width
	}

	return exportProgressBar{
		enabled: isTerminal(os.Stderr),
		total:   total,
		bar:     bar,
	}
}

func (p *exportProgressBar) Advance(label string) {
	if !p.enabled {
		return
	}
	p.current++
	if p.current > p.total {
		p.current = p.total
	}
	p.label = label
	p.render()
}

func (p *exportProgressBar) Finish(label string) {
	if !p.enabled {
		return
	}
	p.current = p.total
	p.label = label
	p.render()
	fmt.Fprint(os.Stderr, "\n")
	p.lastRenderWidth = 0
}

func (p *exportProgressBar) Close() {
	if !p.enabled {
		return
	}
	if p.lastRenderWidth > 0 {
		fmt.Fprint(os.Stderr, "\n")
		p.lastRenderWidth = 0
	}
}

func (p *exportProgressBar) render() {
	percent := float64(p.current) / float64(p.total)
	if percent < 0 {
		percent = 0
	}
	if percent > 1 {
		percent = 1
	}
	line := fmt.Sprintf("%s %3.0f%% %d/%d %s", p.bar.ViewAs(percent), percent*100, p.current, p.total, strings.TrimSpace(p.label))
	pad := ""
	if p.lastRenderWidth > len(line) {
		pad = strings.Repeat(" ", p.lastRenderWidth-len(line))
	}
	fmt.Fprintf(os.Stderr, "\r%s%s", line, pad)
	p.lastRenderWidth = len(line)
}

func isTerminal(f *os.File) bool {
	if f == nil {
		return false
	}
	if strings.EqualFold(strings.TrimSpace(os.Getenv("TERM")), "dumb") {
		return false
	}
	info, err := f.Stat()
	if err != nil {
		return false
	}
	return (info.Mode() & os.ModeCharDevice) != 0
}

func (e Exporter) Run() (Stats, error) {
	if e.InputDir == "" || e.OutputDir == "" {
		return Stats{}, fmt.Errorf("input and output directories are required")
	}

	if err := os.MkdirAll(e.OutputDir, 0o755); err != nil {
		return Stats{}, fmt.Errorf("create output dir: %w", err)
	}

	filenameEscaping, err := resolveFilenameEscaping(e.FilenameEscaping)
	if err != nil {
		return Stats{}, err
	}

	exportData, err := anytypejson.ReadExport(e.InputDir)
	if err != nil {
		return Stats{}, err
	}
	objects := exportData.Objects
	relations := exportData.Relations
	optionsByID := exportData.OptionsByID
	fileObjects := exportData.FileObjects
	templates := exportData.Templates
	typesByID := exportData.TypesByID

	noteDir := filepath.Join(e.OutputDir, "notes")
	rawDir := filepath.Join(e.OutputDir, "_anytype", "raw")
	templateDir := filepath.Join(e.OutputDir, "templates")
	baseDir := filepath.Join(e.OutputDir, "bases")
	if err := os.MkdirAll(noteDir, 0o755); err != nil {
		return Stats{}, err
	}
	if err := os.MkdirAll(templateDir, 0o755); err != nil {
		return Stats{}, err
	}
	if err := os.MkdirAll(baseDir, 0o755); err != nil {
		return Stats{}, err
	}
	if err := os.MkdirAll(rawDir, 0o755); err != nil {
		return Stats{}, err
	}

	anytypeDir := filepath.Join(e.OutputDir, "_anytype")
	rawReadme := strings.TrimSpace(`This folder stores exporter metadata for this vault.

What is inside:
- index.json with deterministic object ID -> note path mapping
- raw/ with one JSON sidecar per exported object: <object-id>.json
- each raw sidecar keeps original Anytype fields: id, sbType, details

Why it exists:
- Preserves metadata that may not fit cleanly into Obsidian markdown/frontmatter
- Helps with debugging and future re-mapping without re-reading .pb.json snapshots

Can I delete this folder?
	- Yes, if you do not need exporter metadata.
- Deleting it will not break existing markdown notes in this export.
- If needed, you can restore it by running the exporter again.`) + "\n"
	if err := os.WriteFile(filepath.Join(anytypeDir, "README.md"), []byte(rawReadme), 0o644); err != nil {
		return Stats{}, fmt.Errorf("write raw metadata readme: %w", err)
	}

	copiedFiles, err := copyDir(filepath.Join(e.InputDir, "files"), filepath.Join(e.OutputDir, "files"))
	if err != nil {
		return Stats{}, err
	}

	filters := newPropertyFilters(e.ExcludePropertyKeys, e.ForceIncludePropertyKeys, e.LinkAsNotePropertyKeys, e.ExcludeEmptyProperties)
	syntheticObjects := buildSyntheticLinkObjects(objects, relations, optionsByID, typesByID, filters)

	allObjects := make([]objectInfo, 0, len(objects)+len(syntheticObjects))
	allObjects = append(allObjects, objects...)
	allObjects = append(allObjects, syntheticObjects...)

	progressBar := newExportProgressBar(len(objects) + len(templates) + len(allObjects) + 1)
	if e.RunPrettier {
		progressBar.total++
	}
	defer progressBar.Close()

	notePathByID := make(map[string]string, len(allObjects))
	used := map[string]int{}
	for _, obj := range allObjects {
		title := inferObjectTitle(obj)
		base := sanitizeName(title, filenameEscaping)
		if base == "" {
			base = obj.ID
		}
		usedKey := filenameCollisionKey(base, filenameEscaping)
		n := used[usedKey]
		used[usedKey] = n + 1
		if n > 0 {
			base = base + "-" + strconv.Itoa(n+1)
		}
		notePathByID[obj.ID] = filepath.ToSlash(filepath.Join("notes", base+".md"))
	}

	templatePathByID := make(map[string]string, len(templates))
	usedTemplateNames := map[string]int{}
	for _, tmpl := range templates {
		typeName := inferTemplateTypeName(tmpl.TargetTypeID, typesByID)
		templateName := inferTemplateTitle(tmpl)
		if strings.TrimSpace(templateName) == "" {
			templateName = "Template"
		}
		base := sanitizeName(typeName+" - "+templateName, filenameEscaping)
		if base == "" {
			base = sanitizeName(typeName+" - Template", filenameEscaping)
		}
		if base == "" {
			base = "Template"
		}
		usedKey := filenameCollisionKey(base, filenameEscaping)
		n := usedTemplateNames[usedKey]
		usedTemplateNames[usedKey] = n + 1
		if n > 0 {
			base = base + "-" + strconv.Itoa(n+1)
		}
		templatePathByID[tmpl.ID] = filepath.ToSlash(filepath.Join("templates", base+".md"))
	}

	idToObject := make(map[string]objectInfo, len(allObjects))
	objectNamesByID := make(map[string]string, len(allObjects)+len(typesByID)+len(optionsByID))
	for _, o := range allObjects {
		idToObject[o.ID] = o
		if name := strings.TrimSpace(o.Name); name != "" {
			objectNamesByID[o.ID] = name
		}
	}
	for id, typeInfo := range typesByID {
		name := strings.TrimSpace(typeInfo.Name)
		if name == "" {
			continue
		}
		if _, exists := objectNamesByID[id]; exists {
			continue
		}
		objectNamesByID[id] = name
	}

	for id, option := range optionsByID {
		name := strings.TrimSpace(option.Name)
		if name == "" {
			continue
		}
		if _, exists := objectNamesByID[id]; exists {
			continue
		}
		objectNamesByID[id] = name
	}

	optionNamesByID := make(map[string]string, len(optionsByID))
	for id, option := range optionsByID {
		optionNamesByID[id] = option.Name
	}

	usedBaseNames := map[string]int{}
	for _, obj := range objects {
		baseContent, ok := renderBaseFile(obj, relations, optionNamesByID, notePathByID, objectNamesByID, fileObjects)
		if !ok {
			progressBar.Advance("exporting bases")
			continue
		}
		title := inferObjectTitle(obj)
		baseName := sanitizeName(title, filenameEscaping)
		if baseName == "" {
			baseName = obj.ID
		}
		usedKey := filenameCollisionKey(baseName, filenameEscaping)
		n := usedBaseNames[usedKey]
		usedBaseNames[usedKey] = n + 1
		if n > 0 {
			baseName = baseName + "-" + strconv.Itoa(n+1)
		}
		basePath := filepath.Join(baseDir, baseName+".base")
		if err := os.WriteFile(basePath, []byte(baseContent), 0o644); err != nil {
			return Stats{}, fmt.Errorf("write base %s: %w", obj.ID, err)
		}
		if err := applyExportedFileTimes(basePath, obj.Details); err != nil {
			return Stats{}, fmt.Errorf("apply base timestamps %s: %w", obj.ID, err)
		}
		progressBar.Advance("exporting bases")
	}

	for _, tmpl := range templates {
		templateRelPath := templatePathByID[tmpl.ID]
		templateAbsPath := filepath.Join(e.OutputDir, filepath.FromSlash(templateRelPath))
		if err := os.MkdirAll(filepath.Dir(templateAbsPath), 0o755); err != nil {
			return Stats{}, err
		}
		content := renderTemplate(tmpl, relations, typesByID, idToObject, notePathByID, fileObjects)
		if err := os.WriteFile(templateAbsPath, []byte(content), 0o644); err != nil {
			return Stats{}, fmt.Errorf("write template %s: %w", tmpl.ID, err)
		}
		if err := applyExportedFileTimes(templateAbsPath, tmpl.Details); err != nil {
			return Stats{}, fmt.Errorf("apply template timestamps %s: %w", tmpl.ID, err)
		}
		progressBar.Advance("exporting templates")
	}

	for _, obj := range allObjects {
		noteRelPath := notePathByID[obj.ID]
		noteAbsPath := filepath.Join(e.OutputDir, filepath.FromSlash(noteRelPath))
		if err := os.MkdirAll(filepath.Dir(noteAbsPath), 0o755); err != nil {
			return Stats{}, err
		}

		fm := renderFrontmatter(
			obj,
			relations,
			typesByID,
			optionNamesByID,
			notePathByID,
			noteRelPath,
			objectNamesByID,
			fileObjects,
			e.IncludeDynamicProperties,
			e.IncludeArchivedProperties,
			filters,
		)
		body := renderBody(obj, idToObject, notePathByID, noteRelPath, fileObjects)
		if err := os.WriteFile(noteAbsPath, []byte(fm+body), 0o644); err != nil {
			return Stats{}, fmt.Errorf("write note %s: %w", obj.ID, err)
		}
		if err := applyExportedFileTimes(noteAbsPath, obj.Details); err != nil {
			return Stats{}, fmt.Errorf("apply note timestamps %s: %w", obj.ID, err)
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
		progressBar.Advance("exporting notes")
	}

	if !e.DisableIconizeIcons {
		if err := exportIconizePluginData(e.InputDir, e.OutputDir, allObjects, notePathByID, fileObjects); err != nil {
			return Stats{}, fmt.Errorf("export iconize plugin data: %w", err)
		}
	}

	idx := indexFile{Notes: notePathByID}
	indexBytes, _ := json.MarshalIndent(idx, "", "  ")
	if err := os.MkdirAll(anytypeDir, 0o755); err != nil {
		return Stats{}, err
	}
	if err := os.WriteFile(filepath.Join(anytypeDir, "index.json"), indexBytes, 0o644); err != nil {
		return Stats{}, err
	}
	progressBar.Advance("writing index")

	if e.RunPrettier {
		if err := tryRunPrettier(e.OutputDir); err != nil {
			fmt.Fprintf(os.Stderr, "warning: failed to apply prettier to export: %v\n", err)
		}
		progressBar.Advance("formatting with prettier")
	}

	progressBar.Finish("done")

	return Stats{Notes: len(allObjects), Files: copiedFiles}, nil
}

func tryRunPrettier(outputDir string) error {
	return prettierCommandRunner(outputDir)
}

func renderFrontmatter(obj objectInfo, relations map[string]relationDef, typesByID map[string]typeDef, optionsByID map[string]string, notes map[string]string, sourceNotePath string, objectNamesByID map[string]string, fileObjects map[string]string, includeDynamicProperties bool, includeArchivedProperties bool, filters propertyFilters) string {
	keys, includeByType, dateByType := orderedFrontmatterKeys(obj, relations, typesByID)

	var buf bytes.Buffer
	buf.WriteString("---\n")
	includeAnytypeID := shouldIncludeFrontmatterProperty("anytype_id", relationDef{}, false, false, includeDynamicProperties, includeArchivedProperties, filters)
	if includeAnytypeID {
		buf.WriteString("anytype_id: ")
		writeYAMLString(&buf, obj.ID)
		buf.WriteString("\n")
	}

	usedKeys := map[string]struct{}{}
	if includeAnytypeID {
		usedKeys["anytype_id"] = struct{}{}
	}
	for _, k := range keys {
		rel, hasRel := relations[k]
		if !shouldIncludeFrontmatterProperty(k, rel, hasRel, includeByType[k], includeDynamicProperties, includeArchivedProperties, filters) {
			continue
		}
		v := obj.Details[k]
		converted := convertPropertyValue(k, v, relations, optionsByID, notes, sourceNotePath, objectNamesByID, fileObjects, dateByType[k], filters.hasLinkAsNote(k, rel, hasRel))
		if filters.excludeEmpty && isEmptyFrontmatterValue(converted) {
			continue
		}
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

func orderedFrontmatterKeys(obj objectInfo, relations map[string]relationDef, typesByID map[string]typeDef) ([]string, map[string]bool, map[string]bool) {
	keys := make([]string, 0, len(obj.Details))
	for k := range obj.Details {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	ordered := make([]string, 0, len(keys))
	seen := make(map[string]struct{}, len(keys))
	includeByType := map[string]bool{}
	dateByType := map[string]bool{}

	appendUnique := func(k string, fromType bool) {
		if k == "" {
			return
		}
		if _, ok := obj.Details[k]; !ok {
			return
		}
		if _, ok := seen[k]; ok {
			if fromType {
				includeByType[k] = true
			}
			return
		}
		seen[k] = struct{}{}
		ordered = append(ordered, k)
		if fromType {
			includeByType[k] = true
		}
	}

	typeID := asString(obj.Details["type"])
	if typeID != "" {
		if typeInfo, ok := typesByID[typeID]; ok {
			visibleRefs := make([]string, 0, len(typeInfo.Featured)+len(typeInfo.Recommended)+len(typeInfo.RecommendedFile))
			visibleRefs = append(visibleRefs, typeInfo.Featured...)
			visibleRefs = append(visibleRefs, typeInfo.Recommended...)
			visibleRefs = append(visibleRefs, typeInfo.RecommendedFile...)
			for _, ref := range visibleRefs {
				resolved := resolveTypeRelationRefToDetailKey(ref, obj.Details, relations)
				appendUnique(resolved, true)
				if resolved != "" && isDateRelationRef(ref, relations) {
					dateByType[resolved] = true
				}
			}
			for _, ref := range typeInfo.Hidden {
				resolved := resolveTypeRelationRefToDetailKey(ref, obj.Details, relations)
				appendUnique(resolved, true)
				if resolved != "" && isDateRelationRef(ref, relations) {
					dateByType[resolved] = true
				}
			}
		}
	}

	for _, k := range keys {
		appendUnique(k, false)
	}

	return ordered, includeByType, dateByType
}

func isDateRelationRef(ref string, relations map[string]relationDef) bool {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return false
	}
	rel, ok := relations[ref]
	if !ok {
		return false
	}
	return rel.Format == 4
}

func resolveTypeRelationRefToDetailKey(ref string, details map[string]any, relations map[string]relationDef) string {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return ""
	}
	if _, ok := details[ref]; ok {
		return ref
	}

	rel, hasRel := relations[ref]
	if !hasRel {
		return ""
	}
	if rel.Key != "" {
		if _, ok := details[rel.Key]; ok {
			return rel.Key
		}
	}
	if rel.ID != "" {
		if _, ok := details[rel.ID]; ok {
			return rel.ID
		}
	}
	return ""
}

func newPropertyFilters(exclude []string, forceInclude []string, linkAsNote []string, excludeEmpty bool) propertyFilters {
	return propertyFilters{
		exclude:      normalizePropertyKeySet(exclude),
		forceInclude: normalizePropertyKeySet(forceInclude),
		linkAsNote:   normalizePropertyKeySet(linkAsNote),
		excludeEmpty: excludeEmpty,
	}
}

func normalizePropertyKeySet(keys []string) map[string]struct{} {
	out := make(map[string]struct{}, len(keys))
	for _, key := range keys {
		norm := normalizePropertyKey(key)
		if norm == "" {
			continue
		}
		out[norm] = struct{}{}
	}
	return out
}

func normalizePropertyKey(key string) string {
	return strings.ToLower(strings.TrimSpace(key))
}

func propertyCandidates(rawKey string, rel relationDef, hasRel bool) []string {
	candidates := make([]string, 0, 3)
	if rawKey != "" {
		candidates = append(candidates, rawKey)
	}
	if hasRel {
		if rel.Key != "" && rel.Key != rawKey {
			candidates = append(candidates, rel.Key)
		}
		if rel.Name != "" {
			candidates = append(candidates, rel.Name)
		}
	}
	return candidates
}

func (f propertyFilters) hasForceInclude(rawKey string, rel relationDef, hasRel bool) bool {
	for _, candidate := range propertyCandidates(rawKey, rel, hasRel) {
		if _, ok := f.forceInclude[normalizePropertyKey(candidate)]; ok {
			return true
		}
	}
	return false
}

func (f propertyFilters) hasExclude(rawKey string, rel relationDef, hasRel bool) bool {
	for _, candidate := range propertyCandidates(rawKey, rel, hasRel) {
		if _, ok := f.exclude[normalizePropertyKey(candidate)]; ok {
			return true
		}
	}
	return false
}

func (f propertyFilters) hasLinkAsNote(rawKey string, rel relationDef, hasRel bool) bool {
	for _, candidate := range propertyCandidates(rawKey, rel, hasRel) {
		if _, ok := f.linkAsNote[normalizePropertyKey(candidate)]; ok {
			return true
		}
	}
	return false
}

func shouldIncludeFrontmatterProperty(rawKey string, rel relationDef, hasRel bool, includeByType bool, includeDynamicProperties bool, includeArchivedProperties bool, filters propertyFilters) bool {
	if filters.hasForceInclude(rawKey, rel, hasRel) {
		return true
	}
	if filters.hasExclude(rawKey, rel, hasRel) {
		return false
	}
	if _, hidden := defaultHiddenPropertyKeys[rawKey]; hidden {
		return false
	}
	if hasRel {
		if _, hidden := defaultHiddenPropertyKeys[rel.Key]; hidden {
			return false
		}
	}
	if !includeDynamicProperties {
		if _, dynamic := dynamicPropertyKeys[rawKey]; dynamic {
			return false
		}
		if hasRel {
			if _, dynamic := dynamicPropertyKeys[rel.Key]; dynamic {
				return false
			}
		}
	}
	if !includeArchivedProperties && shouldSkipUnnamedProperty(rawKey, rel, hasRel) && !includeByType {
		return false
	}
	return true
}

func frontmatterKey(rawKey string, rel relationDef, hasRel bool) string {
	if isTagProperty(rawKey, rel, hasRel) {
		return "tags"
	}
	if !hasRel {
		return rawKey
	}
	if rel.Name == "" {
		return rawKey
	}
	if rawKey != rel.Key {
		return rel.Name
	}
	if isLikelyOpaqueAnytypeKey(rawKey) {
		return rel.Name
	}
	return rawKey
}

func isTagProperty(rawKey string, rel relationDef, hasRel bool) bool {
	if normalizePropertyKey(rawKey) == "tag" {
		return true
	}
	if !hasRel {
		return false
	}
	if normalizePropertyKey(rel.Key) == "tag" {
		return true
	}
	return normalizePropertyKey(rel.Name) == "tag"
}

func shouldSkipUnnamedProperty(key string, rel relationDef, hasRel bool) bool {
	if hasRel {
		return strings.TrimSpace(rel.Name) == ""
	}
	return isLikelyOpaqueAnytypeKey(key)
}

func isLikelyOpaqueAnytypeKey(s string) bool {
	return isLikelyAnytypeObjectID(s) || isLikelyCIDKey(s)
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

func isLikelyCIDKey(s string) bool {
	if len(s) < 20 || !strings.HasPrefix(s, "bafy") {
		return false
	}
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '2' && r <= '7') {
			continue
		}
		return false
	}
	return true
}

func convertPropertyValue(key string, value any, relations map[string]relationDef, optionsByID map[string]string, notes map[string]string, sourceNotePath string, objectNamesByID map[string]string, fileObjects map[string]string, dateByType bool, linkAsNote bool) any {
	rel, hasRel := relations[key]
	listValue := isListValue(value)
	if !hasRel {
		if dateByType {
			return formatDateValue(value)
		}
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
				out = append(out, "[["+relativeWikiTarget(sourceNotePath, note)+"]]")
			} else if name, ok := objectNamesByID[id]; ok && strings.TrimSpace(name) != "" {
				out = append(out, name)
			} else {
				out = append(out, id)
			}
		}
		if listValue {
			return out
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
			if linkAsNote {
				if note, ok := notes[id]; ok {
					out = append(out, "[["+relativeWikiTarget(sourceNotePath, note)+"]]")
					continue
				}
			}
			if n, ok := optionsByID[id]; ok && n != "" {
				out = append(out, n)
			} else {
				out = append(out, id)
			}
		}
		if listValue {
			return out
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
				out = append(out, relativePathTarget(sourceNotePath, src))
			} else {
				out = append(out, id)
			}
		}
		if listValue {
			return out
		}
		if len(out) == 1 {
			return out[0]
		}
		if len(out) > 1 {
			return out
		}
		return value
	case 4:
		return formatDateValue(value)
	default:
		return value
	}
}

func isListValue(v any) bool {
	switch v.(type) {
	case []any, []string:
		return true
	default:
		return false
	}
}

func buildSyntheticLinkObjects(objects []objectInfo, relations map[string]relationDef, optionsByID map[string]relationOption, typesByID map[string]typeDef, filters propertyFilters) []objectInfo {
	if len(filters.linkAsNote) == 0 {
		return nil
	}

	existingIDs := make(map[string]struct{}, len(objects))
	for _, obj := range objects {
		existingIDs[obj.ID] = struct{}{}
	}

	optionIDs := map[string]struct{}{}
	typeIDs := map[string]struct{}{}
	for _, obj := range objects {
		for key, raw := range obj.Details {
			rel, hasRel := relations[key]
			if !filters.hasLinkAsNote(key, rel, hasRel) {
				continue
			}
			ids := anyToStringSlice(raw)
			if len(ids) == 0 {
				if s := asString(raw); s != "" {
					ids = []string{s}
				}
			}
			if len(ids) == 0 {
				continue
			}
			for _, id := range ids {
				switch rel.Format {
				case 100:
					if _, ok := typesByID[id]; ok {
						typeIDs[id] = struct{}{}
					}
				case 11, 3:
					if _, ok := optionsByID[id]; ok {
						optionIDs[id] = struct{}{}
					}
				}
			}
		}
	}

	typeIDList := make([]string, 0, len(typeIDs))
	for id := range typeIDs {
		typeIDList = append(typeIDList, id)
	}
	sort.Strings(typeIDList)

	optionIDList := make([]string, 0, len(optionIDs))
	for id := range optionIDs {
		optionIDList = append(optionIDList, id)
	}
	sort.Strings(optionIDList)

	out := make([]objectInfo, 0, len(typeIDList)+len(optionIDList))
	for _, id := range typeIDList {
		if _, exists := existingIDs[id]; exists {
			continue
		}
		typeInfo, ok := typesByID[id]
		if !ok {
			continue
		}
		out = append(out, objectInfo{
			ID:      id,
			Name:    typeInfo.Name,
			SbType:  typeInfo.SbType,
			Details: typeInfo.Details,
			Blocks:  typeInfo.Blocks,
		})
		existingIDs[id] = struct{}{}
	}

	for _, id := range optionIDList {
		if _, exists := existingIDs[id]; exists {
			continue
		}
		option, ok := optionsByID[id]
		if !ok {
			continue
		}
		blocks := option.Blocks
		if len(blocks) == 0 {
			blocks = []block{
				{ID: option.ID, ChildrenID: []string{option.ID + "-title"}},
				{ID: option.ID + "-title", Text: &textBlock{Text: option.Name, Style: "Title"}},
			}
		}
		out = append(out, objectInfo{
			ID:      id,
			Name:    option.Name,
			SbType:  option.SbType,
			Details: option.Details,
			Blocks:  blocks,
		})
		existingIDs[id] = struct{}{}
	}

	return out
}

func formatDateValue(value any) any {
	toUnixSeconds := func(v float64) int64 {
		sec := int64(v)
		if sec > 1_000_000_000_000 || sec < -1_000_000_000_000 {
			sec = sec / 1000
		}
		return sec
	}

	switch t := value.(type) {
	case float64:
		return time.Unix(toUnixSeconds(t), 0).UTC().Format("2006-01-02")
	case int:
		return time.Unix(toUnixSeconds(float64(t)), 0).UTC().Format("2006-01-02")
	case string:
		s := strings.TrimSpace(t)
		if s == "" {
			return value
		}
		if i, err := strconv.ParseInt(s, 10, 64); err == nil {
			sec := i
			if sec > 1_000_000_000_000 || sec < -1_000_000_000_000 {
				sec = sec / 1000
			}
			return time.Unix(sec, 0).UTC().Format("2006-01-02")
		}
		if tm, err := time.Parse(time.RFC3339, s); err == nil {
			return tm.UTC().Format("2006-01-02")
		}
		if tm, err := time.Parse("2006-01-02", s); err == nil {
			return tm.Format("2006-01-02")
		}
		return value
	default:
		return value
	}
}

func applyExportedFileTimes(path string, details map[string]any) error {
	createdTime, hasCreated := firstParsedTimestamp(details, createdDateKeys)
	atime, mtime, ok := anytypeTimestamps(details)
	if !ok {
		return nil
	}
	if err := os.Chtimes(path, atime, mtime); err != nil {
		return err
	}
	if hasCreated {
		if err := setFileCreationTime(path, createdTime); err != nil {
			return err
		}
	}
	return nil
}

func anytypeTimestamps(details map[string]any) (time.Time, time.Time, bool) {
	created, hasCreated := firstParsedTimestamp(details, createdDateKeys)
	changed, _ := firstParsedTimestamp(details, changedDateKeys)
	modified, hasModified := firstParsedTimestamp(details, modifiedDateKeys)

	mtime := modified
	if !hasModified {
		mtime = changed
	}
	if mtime.IsZero() {
		mtime = created
	}

	atime := created
	if !hasCreated {
		atime = changed
	}
	if atime.IsZero() {
		atime = mtime
	}

	if atime.IsZero() || mtime.IsZero() {
		return time.Time{}, time.Time{}, false
	}
	return atime, mtime, true
}

func firstParsedTimestamp(details map[string]any, keys []string) (time.Time, bool) {
	if len(details) == 0 {
		return time.Time{}, false
	}
	for _, key := range keys {
		raw, ok := details[key]
		if !ok {
			continue
		}
		parsed, ok := parseAnytypeTimestamp(raw)
		if ok {
			return parsed, true
		}
	}
	return time.Time{}, false
}

func parseAnytypeTimestamp(value any) (time.Time, bool) {
	toUnixSeconds := func(v int64) int64 {
		if v > 1_000_000_000_000 || v < -1_000_000_000_000 {
			return v / 1000
		}
		return v
	}

	switch t := value.(type) {
	case float64:
		sec := toUnixSeconds(int64(t))
		return time.Unix(sec, 0).UTC(), true
	case int:
		sec := toUnixSeconds(int64(t))
		return time.Unix(sec, 0).UTC(), true
	case int64:
		sec := toUnixSeconds(t)
		return time.Unix(sec, 0).UTC(), true
	case string:
		s := strings.TrimSpace(t)
		if s == "" {
			return time.Time{}, false
		}
		if i, err := strconv.ParseInt(s, 10, 64); err == nil {
			return time.Unix(toUnixSeconds(i), 0).UTC(), true
		}
		if tm, err := time.Parse(time.RFC3339, s); err == nil {
			return tm.UTC(), true
		}
		if tm, err := time.Parse("2006-01-02", s); err == nil {
			return tm.UTC(), true
		}
	}

	return time.Time{}, false
}

func renderBody(obj objectInfo, objects map[string]objectInfo, notes map[string]string, sourceNotePath string, fileObjects map[string]string) string {
	byID := make(map[string]block, len(obj.Blocks))
	for _, b := range obj.Blocks {
		byID[b.ID] = b
	}

	root, ok := byID[obj.ID]
	if !ok {
		return ""
	}

	children := make([]string, 0, len(root.ChildrenID))
	for _, childID := range root.ChildrenID {
		child, exists := byID[childID]
		if exists && child.Text != nil && child.Text.Style == "Title" {
			continue
		}
		children = append(children, childID)
	}

	var buf bytes.Buffer
	renderChildren(&buf, byID, children, notes, sourceNotePath, fileObjects, 0, obj.ID)
	return strings.TrimLeft(buf.String(), "\n")
}

func renderChildren(buf *bytes.Buffer, byID map[string]block, children []string, notes map[string]string, sourceNotePath string, fileObjects map[string]string, depth int, rootID string) {
	numberedIndex := 0
	for _, id := range children {
		b, ok := byID[id]
		if ok && b.Text != nil && b.Text.Style == "Numbered" {
			numberedIndex++
		} else {
			numberedIndex = 0
		}
		renderBlock(buf, byID, id, notes, sourceNotePath, fileObjects, depth, rootID, numberedIndex)
	}
}

func renderTemplate(tmpl templateInfo, relations map[string]relationDef, typesByID map[string]typeDef, objects map[string]objectInfo, notes map[string]string, fileObjects map[string]string) string {
	typeName := inferTemplateTypeName(tmpl.TargetTypeID, typesByID)
	keys := collectTemplateRelationKeys(tmpl)

	var buf bytes.Buffer
	buf.WriteString("---\n")
	writeYAMLKeyValue(&buf, "anytype_template_id", tmpl.ID)
	if tmpl.TargetTypeID != "" {
		writeYAMLKeyValue(&buf, "anytype_target_type_id", tmpl.TargetTypeID)
	}
	if typeName != "" {
		writeYAMLKeyValue(&buf, "anytype_target_type", typeName)
	}

	used := map[string]struct{}{}
	for _, raw := range keys {
		rel, hasRel := relations[raw]
		outKey := frontmatterKey(raw, rel, hasRel)
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

	body := renderBody(objectInfo{ID: tmpl.ID, Name: tmpl.Name, Details: tmpl.Details, Blocks: tmpl.Blocks}, objects, notes, "", fileObjects)
	buf.WriteString(body)
	return buf.String()
}

func inferTemplateTypeName(typeID string, typesByID map[string]typeDef) string {
	typeID = strings.TrimSpace(typeID)
	if typeID == "" {
		return "Unknown Type"
	}
	if t, ok := typesByID[typeID]; ok {
		if name := strings.TrimSpace(t.Name); name != "" {
			return name
		}
	}
	return typeID
}

func inferTemplateTitle(tmpl templateInfo) string {
	if name := strings.TrimSpace(tmpl.Name); name != "" {
		return name
	}

	byID := make(map[string]block, len(tmpl.Blocks))
	for _, b := range tmpl.Blocks {
		byID[b.ID] = b
	}

	if root, ok := byID[tmpl.ID]; ok {
		for _, childID := range root.ChildrenID {
			child, exists := byID[childID]
			if !exists || child.Text == nil {
				continue
			}
			if child.Text.Style != "Title" {
				continue
			}
			title := strings.TrimSpace(child.Text.Text)
			if title != "" {
				return title
			}
		}
	}

	if title := strings.TrimSpace(asString(tmpl.Details["title"])); title != "" {
		return title
	}

	return ""
}

type baseViewSpec struct {
	Type    string
	Name    string
	Limit   int
	GroupBy *baseGroupSpec
	Filters *baseFilterNode
	Order   []string
	Select  []string
	Sort    []baseSortSpec
}

type baseGroupSpec struct {
	Property  string
	Direction string
}

type baseSortSpec struct {
	Property       string
	Direction      string
	EmptyPlacement string
	IncludeTime    bool
	NoCollate      bool
	CustomOrder    []string
}

type baseFilterNode struct {
	Expr  string
	Op    string
	Items []baseFilterNode
}

func renderBaseFile(obj objectInfo, relations map[string]relationDef, optionNamesByID map[string]string, notes map[string]string, objectNamesByID map[string]string, fileObjects map[string]string) (string, bool) {
	var views []baseViewSpec
	for _, b := range obj.Blocks {
		if len(b.Dataview) == 0 {
			continue
		}
		parsed := parseDataviewViews(b.Dataview, relations, optionNamesByID, notes, objectNamesByID, fileObjects)
		views = append(views, parsed...)
	}
	if len(views) == 0 {
		return "", false
	}

	var buf bytes.Buffer
	buf.WriteString("views:\n")
	for _, v := range views {
		buf.WriteString("  - type: ")
		writeYAMLString(&buf, v.Type)
		buf.WriteString("\n")
		buf.WriteString("    name: ")
		writeYAMLString(&buf, v.Name)
		buf.WriteString("\n")
		if v.Limit > 0 {
			buf.WriteString("    limit: ")
			buf.WriteString(strconv.Itoa(v.Limit))
			buf.WriteString("\n")
		}
		if v.GroupBy != nil {
			buf.WriteString("    groupBy:\n")
			buf.WriteString("      property: ")
			writeYAMLString(&buf, v.GroupBy.Property)
			buf.WriteString("\n")
			buf.WriteString("      direction: ")
			writeYAMLString(&buf, v.GroupBy.Direction)
			buf.WriteString("\n")
		}
		if v.Filters != nil {
			buf.WriteString("    filters:\n")
			writeBaseFilterNode(&buf, *v.Filters, 3)
		}
		order := v.Select
		if len(order) == 0 {
			order = v.Order
		}
		if len(order) > 0 {
			buf.WriteString("    order:\n")
			for _, prop := range order {
				buf.WriteString("      - ")
				writeYAMLString(&buf, prop)
				buf.WriteString("\n")
			}
		}
		if len(v.Sort) > 0 {
			buf.WriteString("    sort:\n")
			for _, s := range v.Sort {
				buf.WriteString("      - property: ")
				writeYAMLString(&buf, s.Property)
				buf.WriteString("\n")
				buf.WriteString("        direction: ")
				writeYAMLString(&buf, s.Direction)
				buf.WriteString("\n")
				buf.WriteString("        emptyPlacement: ")
				writeYAMLString(&buf, s.EmptyPlacement)
				buf.WriteString("\n")
				buf.WriteString("        includeTime: ")
				if s.IncludeTime {
					buf.WriteString("true\n")
				} else {
					buf.WriteString("false\n")
				}
				buf.WriteString("        noCollate: ")
				if s.NoCollate {
					buf.WriteString("true\n")
				} else {
					buf.WriteString("false\n")
				}
				if len(s.CustomOrder) > 0 {
					buf.WriteString("        customOrder:\n")
					for _, item := range s.CustomOrder {
						buf.WriteString("          - ")
						writeYAMLString(&buf, item)
						buf.WriteString("\n")
					}
				}
			}
		}
	}

	return buf.String(), true
}

func parseDataviewViews(raw map[string]any, relations map[string]relationDef, optionNamesByID map[string]string, notes map[string]string, objectNamesByID map[string]string, fileObjects map[string]string) []baseViewSpec {
	viewsRaw := asAnySlice(raw["views"])
	out := make([]baseViewSpec, 0, len(viewsRaw))
	for _, viewRaw := range viewsRaw {
		viewMap, ok := viewRaw.(map[string]any)
		if !ok {
			continue
		}
		viewType := strings.ToLower(strings.TrimSpace(asString(anyMapGet(viewMap, "type", "Type"))))
		if viewType == "" {
			viewType = "table"
		}
		if viewType == "kanban" {
			viewType = "table"
		}
		if viewType == "gallery" {
			viewType = "cards"
		}

		name := strings.TrimSpace(asString(anyMapGet(viewMap, "name", "Name")))
		if name == "" {
			name = strings.TrimSpace(asString(anyMapGet(viewMap, "id", "Id")))
		}
		if name == "" {
			name = "View"
		}

		view := baseViewSpec{Type: viewType, Name: name}
		view.Limit = asInt(anyMapGet(viewMap, "pageLimit", "PageLimit"))

		relationsRaw := asAnySlice(anyMapGet(viewMap, "relations", "Relations"))
		view.Select = make([]string, 0, len(relationsRaw))
		selectedSeen := make(map[string]struct{}, len(relationsRaw))
		for _, relationRaw := range relationsRaw {
			relationMap, ok := relationRaw.(map[string]any)
			if !ok {
				continue
			}
			visible := true
			if rawVisible, ok := relationMap["isVisible"]; ok {
				visible = asBool(rawVisible)
			} else if rawVisible, ok := relationMap["IsVisible"]; ok {
				visible = asBool(rawVisible)
			}
			if !visible {
				continue
			}
			relationKey := asString(anyMapGet(relationMap, "key", "Key"))
			property := basePropertyPath(relationKey, relations)
			if property == "" {
				continue
			}
			if _, exists := selectedSeen[property]; exists {
				continue
			}
			selectedSeen[property] = struct{}{}
			view.Select = append(view.Select, property)
		}

		sortsRaw := asAnySlice(anyMapGet(viewMap, "sorts", "Sorts"))
		view.Order = make([]string, 0, len(sortsRaw))
		view.Sort = make([]baseSortSpec, 0, len(sortsRaw))
		for _, sortRaw := range sortsRaw {
			sortMap, ok := sortRaw.(map[string]any)
			if !ok {
				continue
			}
			relationKey := asString(anyMapGet(sortMap, "RelationKey", "relationKey"))
			property := basePropertyPath(relationKey, relations)
			if property == "" {
				continue
			}
			view.Order = append(view.Order, property)
			customOrderRaw := asAnySlice(anyMapGet(sortMap, "customOrder", "CustomOrder"))
			customOrder := make([]string, 0, len(customOrderRaw))
			for _, item := range customOrderRaw {
				mapped := convertPropertyValue(relationKey, item, relations, optionNamesByID, notes, "", objectNamesByID, fileObjects, false, false)
				customOrder = append(customOrder, mappedToString(mapped))
			}
			view.Sort = append(view.Sort, baseSortSpec{
				Property:       property,
				Direction:      strings.ToUpper(strings.TrimSpace(asString(anyMapGet(sortMap, "type", "Type")))),
				EmptyPlacement: strings.ToUpper(strings.TrimSpace(asString(anyMapGet(sortMap, "emptyPlacement", "EmptyPlacement")))),
				IncludeTime:    asBool(anyMapGet(sortMap, "includeTime", "IncludeTime")),
				NoCollate:      asBool(anyMapGet(sortMap, "noCollate", "NoCollate")),
				CustomOrder:    customOrder,
			})
		}

		groupKey := asString(anyMapGet(viewMap, "groupRelationKey", "GroupRelationKey"))
		if groupKey != "" {
			direction := "ASC"
			if len(view.Sort) > 0 && strings.TrimSpace(view.Sort[0].Direction) != "" {
				direction = view.Sort[0].Direction
			}
			view.GroupBy = &baseGroupSpec{Property: basePropertyPath(groupKey, relations), Direction: direction}
		}

		filterNodes := make([]baseFilterNode, 0)
		for _, filterRaw := range asAnySlice(anyMapGet(viewMap, "filters", "Filters")) {
			filterMap, ok := filterRaw.(map[string]any)
			if !ok {
				continue
			}
			if node, ok := convertAnytypeFilterNode(filterMap, relations, optionNamesByID, notes, objectNamesByID, fileObjects); ok {
				filterNodes = append(filterNodes, node)
			}
		}
		if len(filterNodes) == 1 {
			view.Filters = &filterNodes[0]
		} else if len(filterNodes) > 1 {
			view.Filters = &baseFilterNode{Op: "and", Items: filterNodes}
		}

		out = append(out, view)
	}
	return out
}

func writeBaseFilterNode(buf *bytes.Buffer, node baseFilterNode, indent int) {
	pad := strings.Repeat("  ", indent)
	if strings.TrimSpace(node.Expr) != "" {
		buf.WriteString(pad)
		writeYAMLString(buf, node.Expr)
		buf.WriteString("\n")
		return
	}
	if len(node.Items) == 0 {
		buf.WriteString(pad)
		writeYAMLString(buf, "true")
		buf.WriteString("\n")
		return
	}
	buf.WriteString(pad)
	buf.WriteString(node.Op)
	buf.WriteString(":\n")
	for _, child := range node.Items {
		buf.WriteString(pad)
		buf.WriteString("  -")
		if strings.TrimSpace(child.Expr) != "" {
			buf.WriteString(" ")
			writeYAMLString(buf, child.Expr)
			buf.WriteString("\n")
			continue
		}
		buf.WriteString("\n")
		writeBaseFilterNode(buf, child, indent+2)
	}
}

func convertAnytypeFilterNode(raw map[string]any, relations map[string]relationDef, optionNamesByID map[string]string, notes map[string]string, objectNamesByID map[string]string, fileObjects map[string]string) (baseFilterNode, bool) {
	op := strings.TrimSpace(strings.ToLower(asString(anyMapGet(raw, "operator", "Operator"))))
	nestedRaw := asAnySlice(anyMapGet(raw, "nestedFilters", "NestedFilters"))
	if op == "and" || op == "or" {
		items := make([]baseFilterNode, 0, len(nestedRaw))
		for _, nested := range nestedRaw {
			nestedMap, ok := nested.(map[string]any)
			if !ok {
				continue
			}
			if node, ok := convertAnytypeFilterNode(nestedMap, relations, optionNamesByID, notes, objectNamesByID, fileObjects); ok {
				items = append(items, node)
			}
		}
		if len(items) == 0 {
			return baseFilterNode{}, false
		}
		return baseFilterNode{Op: op, Items: items}, true
	}
	if op == "no" && len(nestedRaw) > 0 {
		items := make([]baseFilterNode, 0, len(nestedRaw))
		for _, nested := range nestedRaw {
			nestedMap, ok := nested.(map[string]any)
			if !ok {
				continue
			}
			if node, ok := convertAnytypeFilterNode(nestedMap, relations, optionNamesByID, notes, objectNamesByID, fileObjects); ok {
				items = append(items, node)
			}
		}
		if len(items) == 1 {
			return items[0], true
		}
		if len(items) > 1 {
			return baseFilterNode{Op: "and", Items: items}, true
		}
	}

	expr := buildFilterExpression(raw, relations, optionNamesByID, notes, objectNamesByID, fileObjects)
	if strings.TrimSpace(expr) == "" {
		return baseFilterNode{}, false
	}
	return baseFilterNode{Expr: expr}, true
}

func buildFilterExpression(raw map[string]any, relations map[string]relationDef, optionNamesByID map[string]string, notes map[string]string, objectNamesByID map[string]string, fileObjects map[string]string) string {
	relationKey := strings.TrimSpace(asString(anyMapGet(raw, "RelationKey", "relationKey")))
	if relationKey == "" {
		return ""
	}
	condition := strings.TrimSpace(asString(anyMapGet(raw, "condition", "Condition")))
	if condition == "" {
		return ""
	}
	prop := basePropertyPath(relationKey, relations)
	if prop == "" {
		return ""
	}
	value := anyMapGet(raw, "value", "Value")

	includeTime := asBool(anyMapGet(raw, "includeTime", "IncludeTime"))
	quickOption := strings.TrimSpace(asString(anyMapGet(raw, "quickOption", "QuickOption")))
	if isDateCondition(relationKey, raw, relations) && (quickOption != "" || !includeTime) {
		condition, value = normalizeDateFilterCondition(condition, value, quickOption, includeTime)
	}

	mapped := convertPropertyValue(relationKey, value, relations, optionNamesByID, notes, "", objectNamesByID, fileObjects, false, false)

	switch condition {
	case "AndRange":
		return buildComparableExpression(prop, mapped, "AndRange", true, includeTime)
	case "Equal":
		if values, ok := valueAsSlice(mapped); ok {
			return buildContainsAnyExpression(prop, values)
		}
		return prop + " == " + renderFilterLiteral(mapped)
	case "NotEqual":
		if values, ok := valueAsSlice(mapped); ok {
			return "!(" + buildContainsAnyExpression(prop, values) + ")"
		}
		return prop + " != " + renderFilterLiteral(mapped)
	case "Greater":
		return buildComparableExpression(prop, mapped, ">", isDateCondition(relationKey, raw, relations), includeTime)
	case "Less":
		return buildComparableExpression(prop, mapped, "<", isDateCondition(relationKey, raw, relations), includeTime)
	case "GreaterOrEqual":
		return buildComparableExpression(prop, mapped, ">=", isDateCondition(relationKey, raw, relations), includeTime)
	case "LessOrEqual":
		return buildComparableExpression(prop, mapped, "<=", isDateCondition(relationKey, raw, relations), includeTime)
	case "Like":
		return "(" + prop + ".toString().contains(" + renderFilterLiteral(mapped) + "))"
	case "NotLike":
		return "!(" + prop + ".toString().contains(" + renderFilterLiteral(mapped) + "))"
	case "In":
		if values, ok := valueAsSlice(mapped); ok {
			return buildContainsAnyExpression(prop, values)
		}
		return buildContainsAnyExpression(prop, []string{renderFilterLiteral(mapped)})
	case "NotIn":
		if values, ok := valueAsSlice(mapped); ok {
			return "!(" + buildContainsAnyExpression(prop, values) + ")"
		}
		return "!(" + buildContainsAnyExpression(prop, []string{renderFilterLiteral(mapped)}) + ")"
	case "Empty":
		return "(" + prop + " == null || " + prop + " == \"\")"
	case "NotEmpty":
		return "!(" + prop + " == null || " + prop + " == \"\")"
	case "AllIn":
		if values, ok := valueAsSlice(mapped); ok {
			return buildContainsAllExpression(prop, values)
		}
		return buildContainsAllExpression(prop, []string{renderFilterLiteral(mapped)})
	case "NotAllIn":
		if values, ok := valueAsSlice(mapped); ok {
			return "!(" + buildContainsAllExpression(prop, values) + ")"
		}
		return "!(" + buildContainsAllExpression(prop, []string{renderFilterLiteral(mapped)}) + ")"
	case "ExactIn":
		if values, ok := valueAsSlice(mapped); ok {
			return "(" + buildContainsAllExpression(prop, values) + " && list(" + prop + ").length == " + strconv.Itoa(len(values)) + ")"
		}
		return "(" + prop + " == " + renderFilterLiteral(mapped) + ")"
	case "NotExactIn":
		if values, ok := valueAsSlice(mapped); ok {
			return "!(" + buildContainsAllExpression(prop, values) + " && list(" + prop + ").length == " + strconv.Itoa(len(values)) + ")"
		}
		return "!(" + prop + " == " + renderFilterLiteral(mapped) + ")"
	case "Exists":
		return prop + " != null"
	default:
		return ""
	}
}

func normalizeDateFilterCondition(condition string, value any, quickOption string, includeTime bool) (string, any) {
	if strings.TrimSpace(quickOption) == "" && includeTime {
		return condition, value
	}
	from, to := dateRangeFromQuickOption(quickOption, value, time.Now())
	switch condition {
	case "Equal", "In":
		return "AndRange", []any{from.Unix(), to.Unix()}
	case "Less":
		return "Less", from.Unix()
	case "Greater":
		return "Greater", to.Unix()
	case "LessOrEqual":
		return "LessOrEqual", to.Unix()
	case "GreaterOrEqual":
		return "GreaterOrEqual", from.Unix()
	default:
		return condition, value
	}
}

func dateRangeFromQuickOption(quickOption string, value any, now time.Time) (time.Time, time.Time) {
	startOfDay := func(t time.Time) time.Time {
		y, m, d := t.Date()
		return time.Date(y, m, d, 0, 0, 0, 0, t.Location())
	}
	endOfDay := func(t time.Time) time.Time {
		return startOfDay(t).Add(24*time.Hour - time.Second)
	}
	weekStart := func(t time.Time) time.Time {
		wd := int(t.Weekday())
		if wd == 0 {
			wd = 7
		}
		return startOfDay(t).AddDate(0, 0, -(wd - 1))
	}
	weekEnd := func(t time.Time) time.Time {
		return weekStart(t).AddDate(0, 0, 7).Add(-time.Second)
	}
	monthStart := func(t time.Time) time.Time {
		y, m, _ := t.Date()
		return time.Date(y, m, 1, 0, 0, 0, 0, t.Location())
	}
	monthEnd := func(t time.Time) time.Time {
		return monthStart(t).AddDate(0, 1, 0).Add(-time.Second)
	}
	yearStart := func(t time.Time) time.Time {
		y, _, _ := t.Date()
		return time.Date(y, time.January, 1, 0, 0, 0, 0, t.Location())
	}
	yearEnd := func(t time.Time) time.Time {
		return yearStart(t).AddDate(1, 0, 0).Add(-time.Second)
	}

	switch quickOption {
	case "Yesterday":
		t := now.AddDate(0, 0, -1)
		return startOfDay(t), endOfDay(t)
	case "Today":
		return startOfDay(now), endOfDay(now)
	case "Tomorrow":
		t := now.AddDate(0, 0, 1)
		return startOfDay(t), endOfDay(t)
	case "LastWeek":
		t := now.AddDate(0, 0, -7)
		return weekStart(t), weekEnd(t)
	case "CurrentWeek":
		return weekStart(now), weekEnd(now)
	case "NextWeek":
		t := now.AddDate(0, 0, 7)
		return weekStart(t), weekEnd(t)
	case "LastMonth":
		t := now.AddDate(0, -1, 0)
		return monthStart(t), monthEnd(t)
	case "CurrentMonth":
		return monthStart(now), monthEnd(now)
	case "NextMonth":
		t := now.AddDate(0, 1, 0)
		return monthStart(t), monthEnd(t)
	case "NumberOfDaysAgo":
		days := asInt(value)
		t := now.AddDate(0, 0, -days)
		return startOfDay(t), endOfDay(t)
	case "NumberOfDaysNow":
		days := asInt(value)
		t := now.AddDate(0, 0, days)
		return startOfDay(t), endOfDay(t)
	case "LastYear":
		t := now.AddDate(-1, 0, 0)
		return yearStart(t), yearEnd(t)
	case "CurrentYear":
		return yearStart(now), yearEnd(now)
	case "NextYear":
		t := now.AddDate(1, 0, 0)
		return yearStart(t), yearEnd(t)
	default:
		if ts, ok := parseAnytypeTimestamp(value); ok {
			return startOfDay(ts), endOfDay(ts)
		}
		return startOfDay(now), endOfDay(now)
	}
}

func isDateCondition(relationKey string, raw map[string]any, relations map[string]relationDef) bool {
	if rel, ok := relations[relationKey]; ok && rel.Format == 4 {
		return true
	}
	format := strings.ToLower(strings.TrimSpace(asString(anyMapGet(raw, "format", "Format"))))
	return format == "date"
}

func buildComparableExpression(prop string, mapped any, op string, isDate bool, includeTime bool) string {
	if mappedList, ok := mapped.([]any); ok && len(mappedList) == 2 && op == "AndRange" {
		lower := buildComparableExpression(prop, mappedList[0], ">=", true, includeTime)
		upper := buildComparableExpression(prop, mappedList[1], "<=", true, includeTime)
		return "(" + lower + " && " + upper + ")"
	}
	if isDate {
		if ts, ok := parseAnytypeTimestamp(mapped); ok {
			if includeTime {
				return "date(" + prop + ") " + op + " date(\"" + ts.UTC().Format("2006-01-02 15:04:05") + "\")"
			}
			return "date(" + prop + ") " + op + " date(\"" + ts.UTC().Format("2006-01-02") + "\")"
		}
		s := asString(mapped)
		if s != "" {
			return "date(" + prop + ") " + op + " date(" + renderFilterLiteral(s) + ")"
		}
	}
	return prop + " " + op + " " + renderFilterLiteral(mapped)
}

func buildContainsAnyExpression(prop string, values []string) string {
	if len(values) == 0 {
		return "false"
	}
	parts := make([]string, 0, len(values))
	for _, value := range values {
		parts = append(parts, "list("+prop+").contains("+value+")")
	}
	if len(parts) == 1 {
		return parts[0]
	}
	return "(" + strings.Join(parts, " || ") + ")"
}

func buildContainsAllExpression(prop string, values []string) string {
	if len(values) == 0 {
		return "true"
	}
	parts := make([]string, 0, len(values))
	for _, value := range values {
		parts = append(parts, "list("+prop+").contains("+value+")")
	}
	if len(parts) == 1 {
		return parts[0]
	}
	return "(" + strings.Join(parts, " && ") + ")"
}

func valueAsSlice(value any) ([]string, bool) {
	switch t := value.(type) {
	case []string:
		out := make([]string, 0, len(t))
		for _, item := range t {
			out = append(out, renderFilterLiteral(item))
		}
		return out, true
	case []any:
		out := make([]string, 0, len(t))
		for _, item := range t {
			out = append(out, renderFilterLiteral(item))
		}
		return out, true
	default:
		return nil, false
	}
}

func renderFilterLiteral(value any) string {
	switch t := value.(type) {
	case nil:
		return "null"
	case string:
		return strconv.Quote(t)
	case bool:
		if t {
			return "true"
		}
		return "false"
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
	case int:
		return strconv.Itoa(t)
	case int64:
		return strconv.FormatInt(t, 10)
	default:
		if s := asString(value); s != "" {
			return strconv.Quote(s)
		}
		b, _ := json.Marshal(value)
		return strconv.Quote(string(b))
	}
}

func mappedToString(value any) string {
	switch t := value.(type) {
	case string:
		return t
	case []string:
		if len(t) == 1 {
			return t[0]
		}
		b, _ := json.Marshal(t)
		return string(b)
	case []any:
		if len(t) == 1 {
			return asString(t[0])
		}
		b, _ := json.Marshal(t)
		return string(b)
	default:
		if s := asString(value); s != "" {
			return s
		}
		b, _ := json.Marshal(value)
		return string(b)
	}
}

var identifierPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

func basePropertyPath(rawKey string, relations map[string]relationDef) string {
	rawKey = strings.TrimSpace(rawKey)
	if rawKey == "" {
		return ""
	}
	switch rawKey {
	case "name":
		return "file.name"
	case "createdDate", "addedDate":
		return "file.ctime"
	case "lastModifiedDate", "modifiedDate", "changedDate":
		return "file.mtime"
	}
	rel, hasRel := relations[rawKey]
	frontKey := frontmatterKey(rawKey, rel, hasRel)
	if frontKey == "" {
		frontKey = rawKey
	}
	if identifierPattern.MatchString(frontKey) {
		return "note." + frontKey
	}
	return "note[" + strconv.Quote(frontKey) + "]"
}

func anyMapGet(m map[string]any, keys ...string) any {
	for _, key := range keys {
		if v, ok := m[key]; ok {
			return v
		}
	}
	return nil
}

func asAnySlice(v any) []any {
	if v == nil {
		return nil
	}
	if out, ok := v.([]any); ok {
		return out
	}
	return nil
}

func asBool(v any) bool {
	switch t := v.(type) {
	case bool:
		return t
	case string:
		return strings.EqualFold(strings.TrimSpace(t), "true")
	default:
		return false
	}
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

func renderBlock(buf *bytes.Buffer, byID map[string]block, id string, notes map[string]string, sourceNotePath string, fileObjects map[string]string, depth int, rootID string, numberedIndex int) {
	b, ok := byID[id]
	if !ok {
		return
	}

	if isSystemTitleBlock(b) {
		return
	}

	if b.Text != nil && (b.Text.Style == "Callout" || b.Text.Style == "Toggle") {
		renderCalloutBlock(buf, byID, b, notes, sourceNotePath, fileObjects, depth, rootID)
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
		if strings.TrimSpace(b.Latex.Text) != "" {
			buf.WriteString("$$\n" + b.Latex.Text + "\n$$\n")
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

	renderChildren(buf, byID, b.ChildrenID, notes, sourceNotePath, fileObjects, depth+1, rootID)
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
	if strings.TrimSpace(text) == "" || marks == nil || len(marks.Marks) == 0 || len(notes) == 0 {
		return text
	}

	type mentionMark struct {
		from int
		to   int
		note string
	}

	runes := []rune(text)
	mentions := make([]mentionMark, 0, len(marks.Marks))
	for _, mark := range marks.Marks {
		if !strings.EqualFold(strings.TrimSpace(mark.Type), "mention") {
			continue
		}
		note := notes[strings.TrimSpace(mark.Param)]
		if note == "" {
			continue
		}
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
		mentions = append(mentions, mentionMark{from: from, to: to, note: note})
	}
	if len(mentions) == 0 {
		return text
	}

	sort.Slice(mentions, func(i, j int) bool {
		if mentions[i].from == mentions[j].from {
			return mentions[i].to < mentions[j].to
		}
		return mentions[i].from < mentions[j].from
	})

	var out strings.Builder
	cursor := 0
	for _, mention := range mentions {
		if mention.from < cursor {
			continue
		}
		out.WriteString(string(runes[cursor:mention.from]))
		out.WriteString("[[")
		out.WriteString(relativeWikiTarget(sourceNotePath, mention.note))
		out.WriteString("]]")
		cursor = mention.to
	}
	out.WriteString(string(runes[cursor:]))
	return out.String()
}

func renderCalloutBlock(buf *bytes.Buffer, byID map[string]block, b block, notes map[string]string, sourceNotePath string, fileObjects map[string]string, depth int, rootID string) {
	if b.Text == nil {
		return
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
	renderChildren(&child, byID, b.ChildrenID, notes, sourceNotePath, fileObjects, depth+1, rootID)
	body := strings.TrimRight(child.String(), "\n")
	if body == "" {
		return
	}
	buf.WriteString(prefixLines(body, "> "))
	buf.WriteString("\n\n")
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

func relativeWikiTarget(sourceNotePath string, targetNotePath string) string {
	return relativePathTarget(sourceNotePath, targetNotePath)
}

func relativePathTarget(sourcePath string, targetPath string) string {
	targetPath = filepath.ToSlash(strings.TrimSpace(targetPath))
	if targetPath == "" {
		return ""
	}
	sourcePath = filepath.ToSlash(strings.TrimSpace(sourcePath))
	if sourcePath == "" {
		return targetPath
	}

	sourceDir := filepath.ToSlash(filepath.Dir(sourcePath))
	rel, err := filepath.Rel(sourceDir, targetPath)
	if err != nil {
		return targetPath
	}
	rel = filepath.ToSlash(strings.TrimSpace(rel))
	if rel == "" || rel == "." {
		return targetPath
	}
	return rel
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

func isEmptyFrontmatterValue(value any) bool {
	switch v := value.(type) {
	case nil:
		return true
	case string:
		return strings.TrimSpace(v) == ""
	case []string:
		return len(v) == 0
	case []any:
		return len(v) == 0
	case map[string]any:
		return len(v) == 0
	default:
		return false
	}
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

func sanitizeName(s string, mode string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	var b strings.Builder
	for _, r := range s {
		if isForbiddenFileNameRune(r, mode) {
			b.WriteRune('-')
			continue
		}
		b.WriteRune(r)
	}
	out := strings.TrimSpace(b.String())
	if mode == "windows" {
		out = strings.TrimRight(out, ". ")
	}
	out = strings.Trim(out, "/")
	if out == "." || out == ".." {
		out = ""
	}
	if mode == "windows" && isWindowsReservedName(out) {
		out = out + "-file"
	}
	if out == "" {
		return "untitled"
	}
	return out
}

func inferObjectTitle(obj objectInfo) string {
	if name := strings.TrimSpace(obj.Name); name != "" {
		return name
	}

	byID := make(map[string]block, len(obj.Blocks))
	for _, b := range obj.Blocks {
		byID[b.ID] = b
	}

	if root, ok := byID[obj.ID]; ok {
		for _, childID := range root.ChildrenID {
			child, exists := byID[childID]
			if !exists || child.Text == nil {
				continue
			}
			if child.Text.Style != "Title" {
				continue
			}
			title := strings.TrimSpace(child.Text.Text)
			if title != "" {
				return title
			}
		}
	}

	if title := strings.TrimSpace(asString(obj.Details["title"])); title != "" {
		return title
	}

	return obj.ID
}

func resolveFilenameEscaping(mode string) (string, error) {
	mode = strings.TrimSpace(strings.ToLower(mode))
	if mode == "" || mode == "auto" {
		if runtime.GOOS == "windows" {
			return "windows", nil
		}
		return "posix", nil
	}
	if mode == "posix" || mode == "windows" {
		return mode, nil
	}
	return "", fmt.Errorf("invalid filename escaping mode %q: expected auto, posix, or windows", mode)
}

func filenameCollisionKey(name string, mode string) string {
	if mode == "windows" {
		return strings.ToLower(name)
	}
	return name
}

func isForbiddenFileNameRune(r rune, mode string) bool {
	if r == 0 || r == '/' || unicode.IsControl(r) {
		return true
	}
	if mode != "windows" {
		return false
	}
	switch r {
	case '<', '>', ':', '"', '\\', '|', '?', '*':
		return true
	default:
		return false
	}
}

func isWindowsReservedName(name string) bool {
	if name == "" {
		return false
	}
	upper := strings.ToUpper(strings.TrimSpace(name))
	if idx := strings.IndexRune(upper, '.'); idx >= 0 {
		upper = upper[:idx]
	}
	switch upper {
	case "CON", "PRN", "AUX", "NUL",
		"COM1", "COM2", "COM3", "COM4", "COM5", "COM6", "COM7", "COM8", "COM9",
		"LPT1", "LPT2", "LPT3", "LPT4", "LPT5", "LPT6", "LPT7", "LPT8", "LPT9":
		return true
	default:
		return false
	}
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

const iconizeAnytypePackName = "anytype"
const iconizeAnytypePackPrefix = "An"

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
