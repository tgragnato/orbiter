package tui

import (
	"fmt"
	"log/slog"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const maxLogEntries = 500

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
	return LogsTabModel{ch: ch, styles: newLogTabStyles()}
}

func waitForLogEntry(ch LogChannel) tea.Cmd {
	return func() tea.Msg { return <-ch }
}

func (m LogsTabModel) Init() tea.Cmd {
	if m.ch == nil {
		return nil
	}
	return waitForLogEntry(m.ch)
}

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
		case "down", "j":
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

func (m LogsTabModel) visibleLines() int {
	n := m.height - 4 // header strip + status bar
	if n < 1 {
		return 20
	}
	return n
}

func (m LogsTabModel) View() string {
	st := m.styles
	if m.ch == nil {
		return st.debug.Render("  log capture not configured")
	}
	if len(m.entries) == 0 {
		return st.debug.Render("  no log entries yet")
	}

	visible := m.visibleLines()
	total := len(m.entries)
	end := total - m.offset
	start := end - visible
	if start < 0 {
		start = 0
	}
	slice := m.entries[start:end]

	lines := make([]string, 0, len(slice)+1)
	for _, e := range slice {
		lines = append(lines, m.renderEntry(e))
	}

	scrollHint := ""
	if m.offset > 0 {
		scrollHint = fmt.Sprintf(" ↑ %d more below", m.offset)
	}
	statusLine := st.status.Render(fmt.Sprintf("  %d/%d entries · ↑/↓ scroll · g top · G bottom%s", end, total, scrollHint))
	lines = append(lines, statusLine)

	return strings.Join(lines, "\n")
}

func (m LogsTabModel) renderEntry(e LogEntry) string {
	st := m.styles
	ts := st.ts.Render(e.Time.Format("15:04:05"))

	var lvlStyle lipgloss.Style
	var lvlStr string
	switch e.Level {
	case slog.LevelDebug:
		lvlStyle, lvlStr = st.debug, "DBG"
	case slog.LevelInfo:
		lvlStyle, lvlStr = st.info, "INF"
	case slog.LevelWarn:
		lvlStyle, lvlStr = st.warn, "WRN"
	case slog.LevelError:
		lvlStyle, lvlStr = st.errSt, "ERR"
	default:
		lvlStyle, lvlStr = st.info, fmt.Sprintf("%3d", int(e.Level))
	}

	attrParts := make([]string, 0, len(e.Attrs))
	for _, a := range e.Attrs {
		attrParts = append(attrParts, st.key.Render(a.Key)+"="+a.Value.String())
	}
	attrs := ""
	if len(attrParts) > 0 {
		attrs = " " + strings.Join(attrParts, " ")
	}

	return ts + " " + lvlStyle.Render(lvlStr) + " " + lvlStyle.Render(e.Message) + st.debug.Render(attrs)
}
