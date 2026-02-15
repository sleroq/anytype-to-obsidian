package exporter

import (
	"path/filepath"
	"strconv"
	"strings"
)

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

func relativeWikiTarget(sourceNotePath string, targetNotePath string) string {
	targetNotePath = filepath.ToSlash(strings.TrimSpace(targetNotePath))
	if strings.HasPrefix(targetNotePath, "bases/") {
		return targetNotePath
	}
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

func shortestPathTarget(sourcePath string, targetPath string) string {
	full := filepath.ToSlash(strings.TrimSpace(targetPath))
	if full == "" {
		return ""
	}
	rel := relativePathTarget(sourcePath, full)
	if rel == "" {
		return full
	}
	if len(rel) < len(full) {
		return rel
	}
	return full
}
