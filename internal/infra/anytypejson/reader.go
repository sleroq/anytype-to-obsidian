package anytypejson

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	anytypedomain "github.com/sleroq/anytype-to-obsidian/internal/domain/anytype"
)

func ReadExport(inputDir string) (anytypedomain.ExportData, error) {
	objects, err := readObjects(filepath.Join(inputDir, "objects"))
	if err != nil {
		return anytypedomain.ExportData{}, err
	}
	relations, err := readRelations(filepath.Join(inputDir, "relations"))
	if err != nil {
		return anytypedomain.ExportData{}, err
	}
	optionsByID, err := readOptions(filepath.Join(inputDir, "relationsOptions"))
	if err != nil {
		return anytypedomain.ExportData{}, err
	}
	fileObjects, err := readFileObjects(filepath.Join(inputDir, "filesObjects"))
	if err != nil {
		return anytypedomain.ExportData{}, err
	}
	templates, err := readTemplates(filepath.Join(inputDir, "templates"))
	if err != nil {
		return anytypedomain.ExportData{}, err
	}
	typesByID, err := readTypes(filepath.Join(inputDir, "types"))
	if err != nil {
		return anytypedomain.ExportData{}, err
	}

	return anytypedomain.ExportData{
		Objects:     objects,
		Relations:   relations,
		OptionsByID: optionsByID,
		FileObjects: fileObjects,
		Templates:   templates,
		TypesByID:   typesByID,
	}, nil
}

func readObjects(dir string) ([]anytypedomain.ObjectInfo, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read objects dir: %w", err)
	}
	var out []anytypedomain.ObjectInfo
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
		out = append(out, anytypedomain.ObjectInfo{
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

func readRelations(dir string) (map[string]anytypedomain.RelationDef, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read relations dir: %w", err)
	}
	out := make(map[string]anytypedomain.RelationDef)
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
		def := anytypedomain.RelationDef{
			ID:     id,
			Key:    key,
			Name:   asString(f.Snapshot.Data.Details["name"]),
			Format: asInt(f.Snapshot.Data.Details["relationFormat"]),
			Max:    asInt(f.Snapshot.Data.Details["relationMaxCount"]),
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

func readOptions(dir string) (map[string]anytypedomain.RelationOption, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read relation options dir: %w", err)
	}
	out := make(map[string]anytypedomain.RelationOption)
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
		out[id] = anytypedomain.RelationOption{
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

func readTypes(dir string) (map[string]anytypedomain.TypeDef, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]anytypedomain.TypeDef{}, nil
		}
		return nil, fmt.Errorf("read dir %s: %w", dir, err)
	}
	out := make(map[string]anytypedomain.TypeDef)
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
		out[id] = anytypedomain.TypeDef{
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

func readTemplates(dir string) ([]anytypedomain.TemplateInfo, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read templates dir: %w", err)
	}

	out := make([]anytypedomain.TemplateInfo, 0)
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
		out = append(out, anytypedomain.TemplateInfo{
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

func readSnapshot(path string) (anytypedomain.SnapshotFile, error) {
	var s anytypedomain.SnapshotFile
	b, err := os.ReadFile(path)
	if err != nil {
		return s, fmt.Errorf("read %s: %w", path, err)
	}
	if err := json.Unmarshal(b, &s); err != nil {
		return s, fmt.Errorf("decode %s: %w", path, err)
	}
	return s, nil
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
