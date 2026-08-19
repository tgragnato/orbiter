package tui

import (
	"fmt"
	"log/slog"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const (
	maxLogEntries       = 500
	defaultVisibleLines = 20
)

// LogsTabModel displays captured slog entries in a scrollable list.
type LogsTabModel struct {
	ch      LogChannel
	entries []LogEntry
	quit    bool
	width   int
	height  int
	offset  int // lines scrolled up from the bottom; 0 = newest entries visible
	styles  logTabStyles
}

type logTabStyles struct {
	debug  lipgloss.Style
	info   lipgloss.Style
	warn   lipgloss.Style
	errSt  lipgloss.Style
	ts     lipgloss.Style
	key    lipgloss.Style
	status lipgloss.Style
}

func newLogTabStyles() logTabStyles {
	return logTabStyles{
		debug:  lipgloss.NewStyle().Foreground(lipgloss.Color("242")),
		info:   lipgloss.NewStyle().Foreground(lipgloss.Color("39")),
		warn:   lipgloss.NewStyle().Foreground(lipgloss.Color("214")),
		errSt:  lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("196")),
		ts:     lipgloss.NewStyle().Foreground(lipgloss.Color("240")),
		key:    lipgloss.NewStyle().Foreground(lipgloss.Color("245")),
		status: lipgloss.NewStyle().Foreground(lipgloss.Color("245")),
	}
}

// NewLogsTabModel creates the logs tab. ch may be nil (tab shows a placeholder).
func NewLogsTabModel(ch LogChannel) LogsTabModel {
	return LogsTabModel{
		ch:      ch,
		entries: nil,
		quit:    false,
		width:   0,
		height:  0,
		offset:  0,
		styles:  newLogTabStyles(),
	}
}

func waitForLogEntry(ch LogChannel) tea.Cmd {
	return func() tea.Msg { return <-ch }
}

// Init starts listening for log entries on the channel.
func (m LogsTabModel) Init() tea.Cmd {
	if m.ch == nil {
		return nil
	}

	return waitForLogEntry(m.ch)
}

// Update handles incoming messages for the logs tab.
//
//nolint:cyclop // logs tab dispatch; complexity is inherent in scroll+stream handling
func (m LogsTabModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if m.quit {
		return m, nil
	}

	switch msg := msg.(type) {
	case LogEntry:
		m.entries = append(m.entries, msg)
		if len(m.entries) > maxLogEntries {
			m.entries = m.entries[len(m.entries)-maxLogEntries:]
		}
		// clamp offset so it doesn't exceed available scroll range
		if m.offset > 0 {
			maxOff := max(0, len(m.entries)-m.visibleLines())
			if m.offset > maxOff {
				m.offset = maxOff
			}
		}

		return m, waitForLogEntry(m.ch)

	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k":
			maxOff := max(0, len(m.entries)-m.visibleLines())
			if m.offset < maxOff {
				m.offset++
			}
		case keyDown, "j":
			if m.offset > 0 {
				m.offset--
			}
		case "g":
			m.offset = max(0, len(m.entries)-m.visibleLines())
		case "G":
			m.offset = 0
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	}

	return m, nil
}

// NavHint returns the scroll/count string that the root model merges into its
// global help line when the Logs tab is active. This keeps the log view
// itself free of any fixed-height chrome so log entries fill every available row.
func (m LogsTabModel) NavHint() string {
	base := "↑/↓ scroll · g top · G bottom"
	if len(m.entries) == 0 {
		return base
	}

	total := len(m.entries)
	end := total - m.offset

	scrollHint := ""
	if m.offset > 0 {
		scrollHint = fmt.Sprintf(" ↑ %d more below", m.offset)
	}

	return fmt.Sprintf("%d/%d entries · %s%s", end, total, base, scrollHint)
}

// View renders the log entries in a scrollable list.
func (m LogsTabModel) View() string {
	logStyles := m.styles

	if m.ch == nil {
		return logStyles.debug.Render("  log capture not configured")
	}

	if len(m.entries) == 0 {
		return logStyles.debug.Render("  no log entries yet")
	}

	visible := m.visibleLines()
	total := len(m.entries)
	end := total - m.offset
	start := max(end-visible, 0)
	slice := m.entries[start:end]

	lines := make([]string, 0, len(slice))
	for _, entry := range slice {
		lines = append(lines, m.renderEntry(entry))
	}

	return strings.Join(lines, "\n")
}

func (m LogsTabModel) visibleLines() int {
	if m.height < 1 {
		return defaultVisibleLines
	}

	return m.height
}

func (m LogsTabModel) renderEntry(entry LogEntry) string {
	styles := m.styles
	timestamp := styles.ts.Render(entry.Time.Format("15:04:05"))

	var lvlStyle lipgloss.Style

	var lvlStr string

	switch entry.Level {
	case slog.LevelDebug:
		lvlStyle, lvlStr = styles.debug, "DBG"
	case slog.LevelInfo:
		lvlStyle, lvlStr = styles.info, "INF"
	case slog.LevelWarn:
		lvlStyle, lvlStr = styles.warn, "WRN"
	case slog.LevelError:
		lvlStyle, lvlStr = styles.errSt, "ERR"
	default:
		lvlStyle, lvlStr = styles.info, fmt.Sprintf("%3d", int(entry.Level))
	}

	attrParts := make([]string, 0, len(entry.Attrs))
	for _, attr := range entry.Attrs {
		attrParts = append(attrParts, styles.key.Render(attr.Key)+"="+attr.Value.String())
	}

	attrs := ""
	if len(attrParts) > 0 {
		attrs = " " + strings.Join(attrParts, " ")
	}

	return timestamp + " " + lvlStyle.Render(lvlStr) + " " + lvlStyle.Render(entry.Message) + styles.debug.Render(attrs)
}
