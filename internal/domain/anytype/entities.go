package anytype

type SnapshotFile struct {
	SbType   string `json:"sbType"`
	Snapshot struct {
		Data struct {
			Blocks  []Block        `json:"blocks"`
			Details map[string]any `json:"details"`
		} `json:"data"`
	} `json:"snapshot"`
}

type Block struct {
	ID         string         `json:"id"`
	ChildrenID []string       `json:"childrenIds"`
	Fields     map[string]any `json:"fields"`

	Text     *TextBlock     `json:"text"`
	File     *FileBlock     `json:"file"`
	Bookmark *BookmarkBlock `json:"bookmark"`
	Latex    *LatexBlock    `json:"latex"`
	Link     *LinkBlock     `json:"link"`
	Relation *RelationBlock `json:"relation"`
	Layout   *LayoutBlock   `json:"layout"`
	Dataview map[string]any `json:"dataview"`
	Table    map[string]any `json:"table"`
	Div      map[string]any `json:"div"`
	TOC      map[string]any `json:"tableOfContents"`
}

type TextBlock struct {
	Text    string     `json:"text"`
	Style   string     `json:"style"`
	Checked bool       `json:"checked"`
	Marks   *TextMarks `json:"marks"`
}

type TextMarks struct {
	Marks []TextMark `json:"marks"`
}

type TextMark struct {
	Range TextMarkRange `json:"range"`
	Type  string        `json:"type"`
	Param string        `json:"param"`
}

type TextMarkRange struct {
	From int `json:"from"`
	To   int `json:"to"`
}

type FileBlock struct {
	Name           string `json:"name"`
	Type           string `json:"type"`
	TargetObjectID string `json:"targetObjectId"`
}

type BookmarkBlock struct {
	URL   string `json:"url"`
	Title string `json:"title"`
}

type LatexBlock struct {
	Text      string `json:"text"`
	Processor string `json:"processor"`
}

type LinkBlock struct {
	TargetBlockID string `json:"targetBlockId"`
}

type LayoutBlock struct {
	Style string `json:"style"`
}

type RelationBlock struct {
	Key string `json:"key"`
}

type RelationDef struct {
	ID     string
	Key    string
	Name   string
	Format int
}

type TypeDef struct {
	ID              string
	Name            string
	SbType          string
	Details         map[string]any
	Blocks          []Block
	Featured        []string
	Recommended     []string
	RecommendedFile []string
	Hidden          []string
}

type RelationOption struct {
	ID      string
	Name    string
	SbType  string
	Details map[string]any
	Blocks  []Block
}

type ObjectInfo struct {
	ID      string
	Name    string
	SbType  string
	Details map[string]any
	Blocks  []Block
}

type TemplateInfo struct {
	ID           string
	Name         string
	SbType       string
	Details      map[string]any
	Blocks       []Block
	TargetTypeID string
}

type ExportData struct {
	Objects     []ObjectInfo
	Relations   map[string]RelationDef
	OptionsByID map[string]RelationOption
	FileObjects map[string]string
	Templates   []TemplateInfo
	TypesByID   map[string]TypeDef
}
