package exporter

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/progress"
	anytypedomain "github.com/sleroq/anytype-to-obsidian/internal/domain/anytype"
	"github.com/sleroq/anytype-to-obsidian/internal/infra/anytypejson"
)

type Exporter struct {
	InputDir                  string
	OutputDir                 string
	DisableIconizeIcons       bool
	DisablePrettyPropertyIcon bool
	DisablePictureToCover     bool
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

type exportDirs struct {
	noteDir       string
	rawDir        string
	templateDir   string
	baseDir       string
	excalidrawDir string
	anytypeDir    string
}

func (e Exporter) prepareExportDirs() (exportDirs, error) {
	dirs := exportDirs{
		noteDir:       filepath.Join(e.OutputDir, "notes"),
		rawDir:        filepath.Join(e.OutputDir, "_anytype", "raw"),
		templateDir:   filepath.Join(e.OutputDir, "templates"),
		baseDir:       filepath.Join(e.OutputDir, "bases"),
		excalidrawDir: filepath.Join(e.OutputDir, "Excalidraw"),
		anytypeDir:    filepath.Join(e.OutputDir, "_anytype"),
	}
	for _, dir := range []string{dirs.noteDir, dirs.templateDir, dirs.baseDir, dirs.excalidrawDir, dirs.rawDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return exportDirs{}, err
		}
	}
	return dirs, nil
}

func writeAnytypeReadme(anytypeDir string) error {
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
	if err := os.MkdirAll(anytypeDir, 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(anytypeDir, "README.md"), []byte(rawReadme), 0o644); err != nil {
		return fmt.Errorf("write raw metadata readme: %w", err)
	}
	return nil
}

func buildNotePathIndex(allObjects []objectInfo, filenameEscaping string) map[string]string {
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
	return notePathByID
}

func buildTemplatePathIndex(templates []templateInfo, typesByID map[string]typeDef, filenameEscaping string) map[string]string {
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
	return templatePathByID
}

func buildLinkTargetIndex(notePathByID map[string]string, basePathByID map[string]string) map[string]string {
	linkPathByID := make(map[string]string, len(notePathByID)+len(basePathByID))
	for id, path := range notePathByID {
		linkPathByID[id] = path
	}
	for id, path := range basePathByID {
		if strings.TrimSpace(path) == "" {
			continue
		}
		linkPathByID[id] = path
	}
	return linkPathByID
}

func buildObjectNameIndexes(allObjects []objectInfo, typesByID map[string]typeDef, optionsByID map[string]relationOption) (map[string]objectInfo, map[string]string, map[string]string) {
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

	optionNamesByID := make(map[string]string, len(optionsByID))
	for id, option := range optionsByID {
		optionNamesByID[id] = option.Name
		name := strings.TrimSpace(option.Name)
		if name == "" {
			continue
		}
		if _, exists := objectNamesByID[id]; exists {
			continue
		}
		objectNamesByID[id] = name
	}
	return idToObject, objectNamesByID, optionNamesByID
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

	dirs, err := e.prepareExportDirs()
	if err != nil {
		return Stats{}, err
	}
	if err := writeAnytypeReadme(dirs.anytypeDir); err != nil {
		return Stats{}, err
	}

	copiedFiles, err := copyDir(filepath.Join(e.InputDir, "files"), filepath.Join(e.OutputDir, "files"))
	if err != nil {
		return Stats{}, err
	}
	if err := normalizeExportedFileObjectPaths(e.InputDir, e.OutputDir, fileObjects); err != nil {
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

	notePathByID := buildNotePathIndex(allObjects, filenameEscaping)
	templatePathByID := buildTemplatePathIndex(templates, typesByID, filenameEscaping)
	idToObject, objectNamesByID, optionNamesByID := buildObjectNameIndexes(allObjects, typesByID, optionsByID)

	usedExcalidrawNames := map[string]int{}

	basePathByID := map[string]string{}
	usedBaseNames := map[string]int{}
	for _, obj := range objects {
		baseContent, ok := renderBaseFile(obj, relations, optionNamesByID, notePathByID, objectNamesByID, fileObjects, !e.DisablePictureToCover)
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
		basePathByID[obj.ID] = filepath.ToSlash(filepath.Join("bases", baseName+".base"))
		basePath := filepath.Join(dirs.baseDir, baseName+".base")
		if err := os.WriteFile(basePath, []byte(baseContent), 0o644); err != nil {
			return Stats{}, fmt.Errorf("write base %s: %w", obj.ID, err)
		}
		if err := applyExportedFileTimes(basePath, obj.Details); err != nil {
			return Stats{}, fmt.Errorf("apply base timestamps %s: %w", obj.ID, err)
		}
		progressBar.Advance("exporting bases")
	}

	linkPathByID := buildLinkTargetIndex(notePathByID, basePathByID)

	for _, tmpl := range templates {
		templateRelPath := templatePathByID[tmpl.ID]
		templateAbsPath := filepath.Join(e.OutputDir, filepath.FromSlash(templateRelPath))
		if err := os.MkdirAll(filepath.Dir(templateAbsPath), 0o755); err != nil {
			return Stats{}, err
		}
		content := renderTemplate(tmpl, relations, idToObject, linkPathByID, fileObjects, !e.DisablePictureToCover)
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

		excalidrawEmbeds, err := exportExcalidrawDrawings(obj, noteRelPath, dirs.excalidrawDir, filenameEscaping, usedExcalidrawNames)
		if err != nil {
			return Stats{}, fmt.Errorf("export excalidraw %s: %w", obj.ID, err)
		}

		fm := renderFrontmatter(
			obj,
			relations,
			typesByID,
			optionNamesByID,
			linkPathByID,
			noteRelPath,
			objectNamesByID,
			fileObjects,
			e.IncludeDynamicProperties,
			e.IncludeArchivedProperties,
			filters,
			!e.DisablePrettyPropertyIcon,
			!e.DisablePictureToCover,
		)
		body := renderBody(obj, idToObject, linkPathByID, noteRelPath, fileObjects, excalidrawEmbeds)
		if err := os.WriteFile(noteAbsPath, []byte(fm+body), 0o644); err != nil {
			return Stats{}, fmt.Errorf("write note %s: %w", obj.ID, err)
		}
		if err := applyExportedFileTimes(noteAbsPath, obj.Details); err != nil {
			return Stats{}, fmt.Errorf("apply note timestamps %s: %w", obj.ID, err)
		}

		rawPath := filepath.Join(dirs.rawDir, obj.ID+".json")
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

	if err := exportPrettyPropertiesPluginData(e.OutputDir, relations, optionsByID); err != nil {
		return Stats{}, fmt.Errorf("export pretty properties plugin data: %w", err)
	}

	idx := indexFile{Notes: notePathByID}
	indexBytes, _ := json.MarshalIndent(idx, "", "  ")
	if err := os.MkdirAll(dirs.anytypeDir, 0o755); err != nil {
		return Stats{}, err
	}
	if err := os.WriteFile(filepath.Join(dirs.anytypeDir, "index.json"), indexBytes, 0o644); err != nil {
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
