package anytype

import (
	"strconv"
	"strings"
	"time"
)

const (
	RelationFormatDate      = 4
	RelationFormatFile      = 5
	RelationFormatStatus    = 11
	RelationFormatTag       = 3
	RelationFormatObjectRef = 100
)

func ConvertPropertyValue(key string, value any, relations map[string]RelationDef, optionsByID map[string]string, notes map[string]string, sourceNotePath string, objectNamesByID map[string]string, fileObjects map[string]string, dateByType bool, linkAsNote bool, relativeWikiTarget func(sourceNotePath string, targetNotePath string) string, relativePathTarget func(sourcePath string, targetPath string) string) any {
	rel, hasRel := relations[key]
	listValue := isListValue(value)
	if !hasRel {
		if dateByType {
			return FormatDateValue(value)
		}
		return value
	}

	switch rel.Format {
	case RelationFormatObjectRef:
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
	case RelationFormatStatus, RelationFormatTag:
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
	case RelationFormatFile:
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
	case RelationFormatDate:
		return FormatDateValue(value)
	default:
		return value
	}
}

func FormatDateValue(value any) any {
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

func AnytypeTimestamps(details map[string]any, createdDateKeys []string, changedDateKeys []string, modifiedDateKeys []string) (time.Time, time.Time, bool) {
	created, hasCreated := FirstParsedTimestamp(details, createdDateKeys)
	changed, _ := FirstParsedTimestamp(details, changedDateKeys)
	modified, hasModified := FirstParsedTimestamp(details, modifiedDateKeys)

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

func FirstParsedTimestamp(details map[string]any, keys []string) (time.Time, bool) {
	if len(details) == 0 {
		return time.Time{}, false
	}
	for _, key := range keys {
		raw, ok := details[key]
		if !ok {
			continue
		}
		parsed, ok := ParseAnytypeTimestamp(raw)
		if ok {
			return parsed, true
		}
	}
	return time.Time{}, false
}

func ParseAnytypeTimestamp(value any) (time.Time, bool) {
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

func isListValue(v any) bool {
	switch v.(type) {
	case []any, []string:
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
