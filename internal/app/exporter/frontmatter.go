package exporter

import (
	"bytes"
	"encoding/json"
	"fmt"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	anytypedomain "github.com/sleroq/anytype-to-obsidian/internal/domain/anytype"
	"github.com/sleroq/anytype-to-obsidian/internal/infra/exportfs"
)

func renderFrontmatter(obj objectInfo, relations map[string]relationDef, typesByID map[string]typeDef, optionsByID map[string]string, notes map[string]string, sourceNotePath string, objectNamesByID map[string]string, fileObjects map[string]string, includeDynamicProperties bool, includeArchivedProperties bool, filters propertyFilters, prettyPropertyIcon bool, pictureToCover bool) string {
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
	if prettyPropertyIcon {
		if iconValue, ok := prettyPropertyIconValue(obj.Details, fileObjects, sourceNotePath); ok {
			writeYAMLKeyValue(&buf, "icon", iconValue)
			usedKeys["icon"] = struct{}{}
		}
	}
	for _, k := range keys {
		rel, hasRel := relations[k]
		if prettyPropertyIcon && isAnytypeIconProperty(k, rel, hasRel) {
			continue
		}
		if !shouldIncludeFrontmatterProperty(k, rel, hasRel, includeByType[k], includeDynamicProperties, includeArchivedProperties, filters) {
			continue
		}
		v := obj.Details[k]
		converted := convertPropertyValue(k, v, relations, optionsByID, notes, sourceNotePath, objectNamesByID, fileObjects, dateByType[k], filters.hasLinkAsNote(k, rel, hasRel))
		outKey := frontmatterKey(k, rel, hasRel, pictureToCover)
		if outKey == "tags" {
			converted = sanitizeObsidianTagValue(converted)
		}
		if filters.excludeEmpty && isEmptyFrontmatterValue(converted) {
			continue
		}
		if _, exists := usedKeys[outKey]; exists {
			outKey = k
		}
		usedKeys[outKey] = struct{}{}
		writeYAMLKeyValue(&buf, outKey, converted)
	}

	if banner, ok := coverBannerValue(obj.Details, fileObjects); ok {
		if _, exists := usedKeys["banner"]; !exists {
			usedKeys["banner"] = struct{}{}
			writeYAMLKeyValue(&buf, "banner", banner)
		}
	}

	buf.WriteString("---\n\n")
	return buf.String()
}

func coverBannerValue(details map[string]any, fileObjects map[string]string) (string, bool) {
	coverID := strings.TrimSpace(asString(details["coverId"]))
	if coverID == "" {
		return "", false
	}

	coverSource := strings.TrimSpace(fileObjects[coverID])
	if coverSource == "" {
		return "", false
	}

	banner := strings.TrimSpace(filepath.Base(filepath.ToSlash(coverSource)))
	if banner == "" {
		return "", false
	}

	return "[[" + banner + "]]", true
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
	return rel.Format == anytypedomain.RelationFormatDate
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

func frontmatterKey(rawKey string, rel relationDef, hasRel bool, pictureToCover bool) string {
	if pictureToCover && isPictureProperty(rawKey, rel, hasRel) {
		return "cover"
	}
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

func isPictureProperty(rawKey string, rel relationDef, hasRel bool) bool {
	if normalizePropertyKey(rawKey) == "picture" {
		return true
	}
	if !hasRel {
		return false
	}
	return normalizePropertyKey(rel.Key) == "picture"
}

func isAnytypeIconProperty(rawKey string, rel relationDef, hasRel bool) bool {
	rawNorm := normalizePropertyKey(rawKey)
	if rawNorm == "iconemoji" || rawNorm == "iconimage" {
		return true
	}
	if !hasRel {
		return false
	}
	relNorm := normalizePropertyKey(rel.Key)
	return relNorm == "iconemoji" || relNorm == "iconimage"
}

func prettyPropertyIconValue(details map[string]any, fileObjects map[string]string, sourceNotePath string) (any, bool) {
	imageID := strings.TrimSpace(asString(details["iconImage"]))
	if imageID != "" {
		if source := strings.TrimSpace(fileObjects[imageID]); source != "" {
			return shortestPathTarget(sourceNotePath, source), true
		}
		return imageID, true
	}

	emoji := strings.TrimSpace(asString(details["iconEmoji"]))
	if emoji != "" {
		return emoji, true
	}

	return nil, false
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
	return anytypedomain.ConvertPropertyValue(
		key,
		value,
		relations,
		optionsByID,
		notes,
		sourceNotePath,
		objectNamesByID,
		fileObjects,
		dateByType,
		linkAsNote,
		relativeWikiTarget,
		relativePathTarget,
	)
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
				case anytypedomain.RelationFormatObjectRef:
					if _, ok := typesByID[id]; ok {
						typeIDs[id] = struct{}{}
					}
				case anytypedomain.RelationFormatStatus, anytypedomain.RelationFormatTag:
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
	return anytypedomain.FormatDateValue(value)
}

func applyExportedFileTimes(path string, details map[string]any) error {
	return exportfs.ApplyExportedFileTimes(path, details, createdDateKeys, changedDateKeys, modifiedDateKeys, setFileCreationTime)
}

func anytypeTimestamps(details map[string]any) (time.Time, time.Time, bool) {
	return anytypedomain.AnytypeTimestamps(details, createdDateKeys, changedDateKeys, modifiedDateKeys)
}

func parseAnytypeTimestamp(value any) (time.Time, bool) {
	return anytypedomain.ParseAnytypeTimestamp(value)
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

func sanitizeObsidianTagValue(value any) any {
	sanitizeSlice := func(items []string) []string {
		out := make([]string, 0, len(items))
		for _, item := range items {
			tag := sanitizeObsidianTag(item)
			if tag == "" {
				continue
			}
			out = append(out, tag)
		}
		return out
	}

	switch v := value.(type) {
	case string:
		return sanitizeObsidianTag(v)
	case []string:
		return sanitizeSlice(v)
	case []any:
		items := make([]string, 0, len(v))
		for _, item := range v {
			s := asString(item)
			if s == "" {
				continue
			}
			items = append(items, s)
		}
		return sanitizeSlice(items)
	default:
		return value
	}
}

func sanitizeObsidianTag(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if strings.HasPrefix(raw, "[[") && strings.HasSuffix(raw, "]]") {
		return raw
	}

	parts := strings.Split(raw, "/")
	cleanedParts := make([]string, 0, len(parts))
	for _, part := range parts {
		cleaned := sanitizeObsidianTagPart(part)
		if cleaned == "" {
			continue
		}
		cleanedParts = append(cleanedParts, cleaned)
	}

	if len(cleanedParts) == 0 {
		return ""
	}

	tag := strings.Join(cleanedParts, "/")
	hasNonDigit := false
	for _, r := range tag {
		if r == '/' {
			continue
		}
		if !unicode.IsDigit(r) {
			hasNonDigit = true
			break
		}
	}
	if !hasNonDigit {
		tag = "y" + tag
	}

	return tag
}

func sanitizeObsidianTagPart(part string) string {
	part = strings.TrimSpace(part)
	if part == "" {
		return ""
	}

	var b strings.Builder
	lastHyphen := false
	for _, r := range part {
		switch {
		case unicode.IsLetter(r), unicode.IsDigit(r), r == '_', r == '-':
			b.WriteRune(r)
			lastHyphen = r == '-'
		case unicode.IsSpace(r):
			if !lastHyphen && b.Len() > 0 {
				b.WriteRune('-')
				lastHyphen = true
			}
		default:
			if !lastHyphen && b.Len() > 0 {
				b.WriteRune('-')
				lastHyphen = true
			}
		}
	}

	return strings.Trim(b.String(), "-")
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

func copyDir(src, dst string) (int, error) {
	return exportfs.CopyDir(src, dst)
}

func normalizeExportedFileObjectPaths(inputDir, outputDir string, fileObjects map[string]string) error {
	return exportfs.NormalizeExportedFileObjectPaths(inputDir, outputDir, fileObjects)
}
