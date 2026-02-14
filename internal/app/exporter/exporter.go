package exporter

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
)

type Exporter struct {
	InputDir                  string
	OutputDir                 string
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
	Relation *relationBlock `json:"relation"`
	Layout   *layoutBlock   `json:"layout"`
	Table    map[string]any `json:"table"`
	Div      map[string]any `json:"div"`
	TOC      map[string]any `json:"tableOfContents"`
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
	Text      string `json:"text"`
	Processor string `json:"processor"`
}

type linkBlock struct {
	TargetBlockID string `json:"targetBlockId"`
}

type layoutBlock struct {
	Style string `json:"style"`
}

type relationBlock struct {
	Key string `json:"key"`
}

type relationDef struct {
	ID     string
	Key    string
	Name   string
	Format int
}

type typeDef struct {
	ID              string
	Name            string
	SbType          string
	Details         map[string]any
	Blocks          []block
	Featured        []string
	Recommended     []string
	RecommendedFile []string
	Hidden          []string
}

type relationOption struct {
	ID      string
	Name    string
	SbType  string
	Details map[string]any
	Blocks  []block
}

type objectInfo struct {
	ID      string
	Name    string
	SbType  string
	Details map[string]any
	Blocks  []block
}

type templateInfo struct {
	ID           string
	Name         string
	SbType       string
	Details      map[string]any
	Blocks       []block
	TargetTypeID string
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
	templates, err := readTemplates(filepath.Join(e.InputDir, "templates"))
	if err != nil {
		return Stats{}, err
	}
	typesByID, err := readTypes(filepath.Join(e.InputDir, "types"))
	if err != nil {
		return Stats{}, err
	}

	noteDir := filepath.Join(e.OutputDir, "notes")
	rawDir := filepath.Join(e.OutputDir, "_anytype", "raw")
	templateDir := filepath.Join(e.OutputDir, "templates")
	if err := os.MkdirAll(noteDir, 0o755); err != nil {
		return Stats{}, err
	}
	if err := os.MkdirAll(templateDir, 0o755); err != nil {
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
			objectNamesByID,
			fileObjects,
			e.IncludeDynamicProperties,
			e.IncludeArchivedProperties,
			filters,
		)
		body := renderBody(obj, idToObject, notePathByID, fileObjects)
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
	}

	idx := indexFile{Notes: notePathByID}
	indexBytes, _ := json.MarshalIndent(idx, "", "  ")
	if err := os.MkdirAll(anytypeDir, 0o755); err != nil {
		return Stats{}, err
	}
	if err := os.WriteFile(filepath.Join(anytypeDir, "index.json"), indexBytes, 0o644); err != nil {
		return Stats{}, err
	}

	return Stats{Notes: len(allObjects), Files: copiedFiles}, nil
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

func readOptions(dir string) (map[string]relationOption, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read relation options dir: %w", err)
	}
	out := make(map[string]relationOption)
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
		out[id] = relationOption{
			ID:      id,
			Name:    strings.TrimSpace(asString(f.Snapshot.Data.Details["name"])),
			SbType:  f.SbType,
			Details: f.Snapshot.Data.Details,
			Blocks:  f.Snapshot.Data.Blocks,
		}
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

func readTypes(dir string) (map[string]typeDef, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]typeDef{}, nil
		}
		return nil, fmt.Errorf("read dir %s: %w", dir, err)
	}
	out := make(map[string]typeDef)
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
		out[id] = typeDef{
			ID:              id,
			Name:            strings.TrimSpace(asString(f.Snapshot.Data.Details["name"])),
			SbType:          f.SbType,
			Details:         f.Snapshot.Data.Details,
			Blocks:          f.Snapshot.Data.Blocks,
			Featured:        anyToStringSlice(f.Snapshot.Data.Details["recommendedFeaturedRelations"]),
			Recommended:     anyToStringSlice(f.Snapshot.Data.Details["recommendedRelations"]),
			RecommendedFile: anyToStringSlice(f.Snapshot.Data.Details["recommendedFileRelations"]),
			Hidden:          anyToStringSlice(f.Snapshot.Data.Details["recommendedHiddenRelations"]),
		}
	}
	return out, nil
}

func readTemplates(dir string) ([]templateInfo, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read templates dir: %w", err)
	}

	out := make([]templateInfo, 0)
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
		out = append(out, templateInfo{
			ID:           id,
			Name:         strings.TrimSpace(asString(f.Snapshot.Data.Details["name"])),
			SbType:       f.SbType,
			Details:      f.Snapshot.Data.Details,
			Blocks:       f.Snapshot.Data.Blocks,
			TargetTypeID: strings.TrimSpace(asString(f.Snapshot.Data.Details["targetObjectType"])),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
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

func renderFrontmatter(obj objectInfo, relations map[string]relationDef, typesByID map[string]typeDef, optionsByID map[string]string, notes map[string]string, objectNamesByID map[string]string, fileObjects map[string]string, includeDynamicProperties bool, includeArchivedProperties bool, filters propertyFilters) string {
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
		converted := convertPropertyValue(k, v, relations, optionsByID, notes, objectNamesByID, fileObjects, dateByType[k], filters.hasLinkAsNote(k, rel, hasRel))
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

func convertPropertyValue(key string, value any, relations map[string]relationDef, optionsByID map[string]string, notes map[string]string, objectNamesByID map[string]string, fileObjects map[string]string, dateByType bool, linkAsNote bool) any {
	rel, hasRel := relations[key]
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
				out = append(out, "[["+note+"]]")
			} else if name, ok := objectNamesByID[id]; ok && strings.TrimSpace(name) != "" {
				out = append(out, name)
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
			if linkAsNote {
				if note, ok := notes[id]; ok {
					out = append(out, "[["+note+"]]")
					continue
				}
			}
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
	case 4:
		return formatDateValue(value)
	default:
		return value
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
		renderBlock(&buf, byID, id, notes, fileObjects, 0, obj.ID)
	}
	return strings.TrimLeft(buf.String(), "\n")
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

	body := renderBody(objectInfo{ID: tmpl.ID, Name: tmpl.Name, Details: tmpl.Details, Blocks: tmpl.Blocks}, objects, notes, fileObjects)
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

func renderBlock(buf *bytes.Buffer, byID map[string]block, id string, notes map[string]string, fileObjects map[string]string, depth int, rootID string) {
	b, ok := byID[id]
	if !ok {
		return
	}

	if b.Text != nil && (b.Text.Style == "Callout" || b.Text.Style == "Toggle") {
		renderCalloutBlock(buf, byID, b, notes, fileObjects, depth, rootID)
		return
	}

	if b.Text != nil {
		line := renderTextBlock(*b.Text, depth, b.Fields)
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

	for _, cid := range b.ChildrenID {
		renderBlock(buf, byID, cid, notes, fileObjects, depth+1, rootID)
	}
}

func renderTextBlock(t textBlock, depth int, fields map[string]any) string {
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

func renderCalloutBlock(buf *bytes.Buffer, byID map[string]block, b block, notes map[string]string, fileObjects map[string]string, depth int, rootID string) {
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
	for _, cid := range b.ChildrenID {
		renderBlock(&child, byID, cid, notes, fileObjects, depth+1, rootID)
	}
	body := strings.TrimRight(child.String(), "\n")
	if body == "" {
		return
	}
	buf.WriteString(prefixLines(body, "> "))
	buf.WriteString("\n")
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
		indent := strings.Repeat("  ", max(0, h.level-1))
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
