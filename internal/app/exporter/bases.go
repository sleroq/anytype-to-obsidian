package exporter

import (
	"bytes"
	"encoding/json"
	"regexp"
	"strconv"
	"strings"
	"time"

	anytypedomain "github.com/sleroq/anytype-to-obsidian/internal/domain/anytype"
)

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

var identifierPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

func renderBaseFile(obj objectInfo, relations map[string]relationDef, optionNamesByID map[string]string, notes map[string]string, objectNamesByID map[string]string, fileObjects map[string]string, pictureToCover bool) (string, bool) {
	var views []baseViewSpec
	for _, b := range obj.Blocks {
		if len(b.Dataview) == 0 {
			continue
		}
		targetID := strings.TrimSpace(asString(anyMapGet(b.Dataview, "TargetObjectId", "targetObjectId")))
		if targetID != "" && targetID != obj.ID {
			continue
		}
		parsed := parseDataviewViews(b.Dataview, relations, optionNamesByID, notes, objectNamesByID, fileObjects, pictureToCover)
		views = append(views, parsed...)
	}
	if len(views) == 0 {
		return "", false
	}

	if isCollectionObject(obj) {
		for i := range views {
			views[i].Filters = andBaseFilters(
				views[i].Filters,
				&baseFilterNode{Expr: buildCollectionCreatedInContextFilter(obj.ID)},
			)
		}
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

func buildCollectionCreatedInContextFilter(collectionID string) string {
	quoted := renderFilterLiteral(collectionID)
	contains := buildContainsAnyExpression("note.createdInContext", []string{quoted})
	return "(note.createdInContext == " + quoted + " || " + contains + ")"
}

func andBaseFilters(left *baseFilterNode, right *baseFilterNode) *baseFilterNode {
	if left == nil {
		return right
	}
	if right == nil {
		return left
	}
	return &baseFilterNode{Op: "and", Items: []baseFilterNode{*left, *right}}
}

func parseDataviewViews(raw map[string]any, relations map[string]relationDef, optionNamesByID map[string]string, notes map[string]string, objectNamesByID map[string]string, fileObjects map[string]string, pictureToCover bool) []baseViewSpec {
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
			property := basePropertyPath(relationKey, relations, pictureToCover)
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
			property := basePropertyPath(relationKey, relations, pictureToCover)
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
			view.GroupBy = &baseGroupSpec{Property: basePropertyPath(groupKey, relations, pictureToCover), Direction: direction}
		}

		filterNodes := make([]baseFilterNode, 0)
		for _, filterRaw := range asAnySlice(anyMapGet(viewMap, "filters", "Filters")) {
			filterMap, ok := filterRaw.(map[string]any)
			if !ok {
				continue
			}
			if node, ok := convertAnytypeFilterNode(filterMap, relations, optionNamesByID, notes, objectNamesByID, fileObjects, pictureToCover); ok {
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

func convertAnytypeFilterNode(raw map[string]any, relations map[string]relationDef, optionNamesByID map[string]string, notes map[string]string, objectNamesByID map[string]string, fileObjects map[string]string, pictureToCover bool) (baseFilterNode, bool) {
	op := strings.TrimSpace(strings.ToLower(asString(anyMapGet(raw, "operator", "Operator"))))
	nestedRaw := asAnySlice(anyMapGet(raw, "nestedFilters", "NestedFilters"))
	if op == "and" || op == "or" {
		items := make([]baseFilterNode, 0, len(nestedRaw))
		for _, nested := range nestedRaw {
			nestedMap, ok := nested.(map[string]any)
			if !ok {
				continue
			}
			if node, ok := convertAnytypeFilterNode(nestedMap, relations, optionNamesByID, notes, objectNamesByID, fileObjects, pictureToCover); ok {
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
			if node, ok := convertAnytypeFilterNode(nestedMap, relations, optionNamesByID, notes, objectNamesByID, fileObjects, pictureToCover); ok {
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

	expr := buildFilterExpression(raw, relations, optionNamesByID, notes, objectNamesByID, fileObjects, pictureToCover)
	if strings.TrimSpace(expr) == "" {
		return baseFilterNode{}, false
	}
	return baseFilterNode{Expr: expr}, true
}

func buildFilterExpression(raw map[string]any, relations map[string]relationDef, optionNamesByID map[string]string, notes map[string]string, objectNamesByID map[string]string, fileObjects map[string]string, pictureToCover bool) string {
	relationKey := strings.TrimSpace(asString(anyMapGet(raw, "RelationKey", "relationKey")))
	if relationKey == "" {
		return ""
	}
	condition := strings.TrimSpace(asString(anyMapGet(raw, "condition", "Condition")))
	if condition == "" {
		return ""
	}
	prop := basePropertyPath(relationKey, relations, pictureToCover)
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
	mappedString := strings.TrimSpace(asString(mapped))

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
		if mappedString == "" {
			return ""
		}
		return "(" + prop + ".toString().contains(" + renderFilterLiteral(mapped) + "))"
	case "NotLike":
		if mappedString == "" {
			return ""
		}
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
	if rel, ok := relations[relationKey]; ok && rel.Format == anytypedomain.RelationFormatDate {
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

func basePropertyPath(rawKey string, relations map[string]relationDef, pictureToCover bool) string {
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
	frontKey := frontmatterKey(rawKey, rel, hasRel, pictureToCover)
	if frontKey == "" {
		frontKey = rawKey
	}
	if identifierPattern.MatchString(frontKey) {
		return "note." + frontKey
	}
	return "note[" + strconv.Quote(frontKey) + "]"
}
