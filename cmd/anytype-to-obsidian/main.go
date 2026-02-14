package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/sleroq/anytype-to-obsidian/internal/app/exporter"
)

var (
	focusedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))
	blurredStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	noStyle      = lipgloss.NewStyle()
)

type cliOptions struct {
	Input                     string
	Output                    string
	FilenameEscaping          string
	RunPrettier               bool
	IncludeDynamicProperties  bool
	IncludeArchivedProperties bool
	ExcludeEmptyProperties    bool
	ExcludeProperties         string
	IncludeProperties         string
	LinkAsNoteProperties      string
}

type cliField struct {
	key         string
	label       string
	description string
	value       string
}

type cliModel struct {
	focusIndex int
	fields     []cliField
	inputs     []textinput.Model
	cancelled  bool
}

func main() {
	opts := defaultCLIOptions()

	if len(os.Args) == 1 {
		interactiveOpts, err := runInteractiveOptions(opts)
		if err != nil {
			fmt.Fprintf(os.Stderr, "interactive setup failed: %v\n", err)
			os.Exit(1)
		}
		opts = interactiveOpts
	} else {
		flag.StringVar(&opts.Input, "input", opts.Input, "Path to Anytype-json export directory")
		flag.StringVar(&opts.Output, "output", opts.Output, "Path to output Obsidian vault")
		flag.BoolVar(&opts.RunPrettier, "prettier", opts.RunPrettier, "Try to run npx prettier on exported files (set to false to disable)")
		flag.StringVar(&opts.FilenameEscaping, "filename-escaping", opts.FilenameEscaping, "Filename escaping mode: auto, posix, windows")
		flag.BoolVar(&opts.IncludeDynamicProperties, "include-dynamic-properties", opts.IncludeDynamicProperties, "Include dynamic/system-managed Anytype properties (e.g. backlinks, lastModifiedDate)")
		flag.BoolVar(&opts.IncludeArchivedProperties, "include-archived-properties", opts.IncludeArchivedProperties, "Include archived/unresolved Anytype relation properties that have no readable relation name")
		flag.BoolVar(&opts.ExcludeEmptyProperties, "exclude-empty-properties", opts.ExcludeEmptyProperties, "Exclude frontmatter properties with empty values (nil, empty strings, empty arrays, empty objects)")
		flag.StringVar(&opts.ExcludeProperties, "exclude-properties", opts.ExcludeProperties, "Comma-separated property keys/names to always exclude from frontmatter")
		flag.StringVar(&opts.IncludeProperties, "force-include-properties", opts.IncludeProperties, "Comma-separated property keys/names to always include in frontmatter")
		flag.StringVar(&opts.LinkAsNoteProperties, "link-as-note-properties", opts.LinkAsNoteProperties, "Comma-separated property keys/names to render relation values as note links when possible (e.g. type,tag,status)")
		flag.Parse()
	}

	exp := exporter.Exporter{
		InputDir:                  opts.Input,
		OutputDir:                 opts.Output,
		RunPrettier:               opts.RunPrettier,
		FilenameEscaping:          opts.FilenameEscaping,
		IncludeDynamicProperties:  opts.IncludeDynamicProperties,
		IncludeArchivedProperties: opts.IncludeArchivedProperties,
		ExcludeEmptyProperties:    opts.ExcludeEmptyProperties,
		ExcludePropertyKeys:       parseCommaSeparatedList(opts.ExcludeProperties),
		ForceIncludePropertyKeys:  parseCommaSeparatedList(opts.IncludeProperties),
		LinkAsNotePropertyKeys:    parseCommaSeparatedList(opts.LinkAsNoteProperties),
	}

	stats, err := exp.Run()
	if err != nil {
		fmt.Fprintf(os.Stderr, "export failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("exported %d notes, copied %d files\n", stats.Notes, stats.Files)
}

func defaultCLIOptions() cliOptions {
	return cliOptions{
		Input:                     "./Anytype-json",
		Output:                    "./obsidian-vault",
		FilenameEscaping:          "auto",
		RunPrettier:               true,
		IncludeDynamicProperties:  false,
		IncludeArchivedProperties: false,
		ExcludeEmptyProperties:    false,
		ExcludeProperties:         "",
		IncludeProperties:         "",
		LinkAsNoteProperties:      "",
	}
}

func runInteractiveOptions(defaults cliOptions) (cliOptions, error) {
	m := newCLIModel(defaults)
	result, err := tea.NewProgram(m).Run()
	if err != nil {
		return defaults, err
	}
	finalModel, ok := result.(*cliModel)
	if !ok {
		return defaults, fmt.Errorf("failed to parse TUI result")
	}
	if finalModel.cancelled {
		return defaults, fmt.Errorf("cancelled by user")
	}
	return finalModel.resolveOptions()
}

func newCLIModel(defaults cliOptions) *cliModel {
	fields := []cliField{
		{key: "input", label: "Input directory", description: "Path to Anytype JSON export folder.", value: defaults.Input},
		{key: "output", label: "Output vault directory", description: "Path where the Obsidian vault will be written.", value: defaults.Output},
		{key: "prettier", label: "Run Prettier", description: "Format exported markdown with npx prettier when available.", value: fmt.Sprintf("%t", defaults.RunPrettier)},
		{key: "filenameEscaping", label: "Filename escaping mode", description: "How to sanitize filenames: auto, posix, or windows.", value: defaults.FilenameEscaping},
		{key: "includeDynamicProperties", label: "Include dynamic properties", description: "Include system-managed fields like backlinks and timestamps.", value: fmt.Sprintf("%t", defaults.IncludeDynamicProperties)},
		{key: "includeArchivedProperties", label: "Include archived properties", description: "Include unresolved relation fields without readable names.", value: fmt.Sprintf("%t", defaults.IncludeArchivedProperties)},
		{key: "excludeEmptyProperties", label: "Exclude empty properties", description: "Skip empty frontmatter values (empty strings, lists, objects).", value: fmt.Sprintf("%t", defaults.ExcludeEmptyProperties)},
		{key: "excludeProperties", label: "Always exclude properties", description: "Comma-separated property keys or names to exclude.", value: defaults.ExcludeProperties},
		{key: "includeProperties", label: "Always include properties", description: "Comma-separated property keys or names to force include.", value: defaults.IncludeProperties},
		{key: "linkAsNoteProperties", label: "Link as notes properties", description: "Comma-separated relation keys to render as note links (e.g. type,tag,status).", value: defaults.LinkAsNoteProperties},
	}

	inputs := make([]textinput.Model, len(fields))
	for i := range fields {
		input := textinput.New()
		input.CharLimit = 256
		input.SetValue(fields[i].value)
		input.Prompt = "> "
		if i == 0 {
			input.Focus()
			input.PromptStyle = focusedStyle
			input.TextStyle = focusedStyle
		} else {
			input.PromptStyle = noStyle
			input.TextStyle = noStyle
		}
		inputs[i] = input
	}

	return &cliModel{
		fields: fields,
		inputs: inputs,
	}
}

func (m *cliModel) Init() tea.Cmd {
	return textinput.Blink
}

func (m *cliModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "esc":
			m.cancelled = true
			return m, tea.Quit
		case "up", "shift+tab":
			m.moveFocus(-1)
			return m, nil
		case "down", "tab":
			m.moveFocus(1)
			return m, nil
		case "enter":
			if m.focusIndex == len(m.inputs) {
				return m, tea.Quit
			}
			m.moveFocus(1)
			return m, nil
		}
	}

	if m.focusIndex < len(m.inputs) {
		var cmd tea.Cmd
		m.inputs[m.focusIndex], cmd = m.inputs[m.focusIndex].Update(msg)
		return m, cmd
	}

	return m, nil
}

func (m *cliModel) View() string {
	if m.cancelled {
		return ""
	}

	var b strings.Builder
	b.WriteString("Anytype to Obsidian interactive setup\n\n")
	b.WriteString("Tab/Shift+Tab or arrows: move, Enter: next/run, Esc: cancel\n")
	b.WriteString("Boolean fields accept: true/false, yes/no, 1/0\n\n")

	for i := range m.fields {
		label := m.fields[i].label
		if m.focusIndex == i {
			label = focusedStyle.Render(label)
		}
		fmt.Fprintf(&b, "%s\n%s\n", label, m.inputs[i].View())
		if m.focusIndex == i {
			fmt.Fprintf(&b, "%s\n", blurredStyle.Render(m.fields[i].description))
		}
		b.WriteString("\n")
	}

	button := fmt.Sprintf("[ %s ]", blurredStyle.Render("Run export"))
	if m.focusIndex == len(m.inputs) {
		button = focusedStyle.Render("[ Run export ]")
		b.WriteString("\n")
	}
	b.WriteString(button)
	b.WriteString("\n")

	return b.String()
}

func (m *cliModel) moveFocus(step int) {
	m.focusIndex += step
	max := len(m.inputs)
	if m.focusIndex < 0 {
		m.focusIndex = max
	}
	if m.focusIndex > max {
		m.focusIndex = 0
	}

	for i := range m.inputs {
		if i == m.focusIndex {
			m.inputs[i].Focus()
			m.inputs[i].PromptStyle = focusedStyle
			m.inputs[i].TextStyle = focusedStyle
			continue
		}
		m.inputs[i].Blur()
		m.inputs[i].PromptStyle = noStyle
		m.inputs[i].TextStyle = noStyle
	}
}

func (m *cliModel) resolveOptions() (cliOptions, error) {
	opts := defaultCLIOptions()

	for i := range m.fields {
		value := strings.TrimSpace(m.inputs[i].Value())
		if value == "" {
			value = strings.TrimSpace(m.fields[i].value)
		}
		switch m.fields[i].key {
		case "input":
			opts.Input = value
		case "output":
			opts.Output = value
		case "prettier":
			parsed, err := parseInteractiveBool(value)
			if err != nil {
				return opts, fmt.Errorf("field prettier: %w", err)
			}
			opts.RunPrettier = parsed
		case "filenameEscaping":
			opts.FilenameEscaping = value
		case "includeDynamicProperties":
			parsed, err := parseInteractiveBool(value)
			if err != nil {
				return opts, fmt.Errorf("field include-dynamic-properties: %w", err)
			}
			opts.IncludeDynamicProperties = parsed
		case "includeArchivedProperties":
			parsed, err := parseInteractiveBool(value)
			if err != nil {
				return opts, fmt.Errorf("field include-archived-properties: %w", err)
			}
			opts.IncludeArchivedProperties = parsed
		case "excludeEmptyProperties":
			parsed, err := parseInteractiveBool(value)
			if err != nil {
				return opts, fmt.Errorf("field exclude-empty-properties: %w", err)
			}
			opts.ExcludeEmptyProperties = parsed
		case "excludeProperties":
			opts.ExcludeProperties = value
		case "includeProperties":
			opts.IncludeProperties = value
		case "linkAsNoteProperties":
			opts.LinkAsNoteProperties = value
		}
	}

	return opts, nil
}

func parseInteractiveBool(value string) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "y", "on":
		return true, nil
	case "0", "false", "no", "n", "off":
		return false, nil
	default:
		return false, fmt.Errorf("expected boolean value, got %q", value)
	}
}

func parseCommaSeparatedList(value string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed == "" {
			continue
		}
		out = append(out, trimmed)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
