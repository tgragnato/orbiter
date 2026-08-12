package tui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	innersignal "github.com/tgragnato/orbiter/internal/signal"
)

const signalsRefreshInterval = time.Hour

type signalsTickMsg time.Time

type signalsMsg struct {
	messages []innersignal.Message
}

// MLEngine is the subset of the background ML engine surface needed by the TUI.
type MLEngine interface {
	// Status returns the current engine status code.
	Status() int32
	// Pause suspends background training.
	Pause()
	// Resume resumes background training.
	Resume()
	// Trigger requests an immediate training run (bypasses the 24-hour interval).
	// No-op when training is already in progress.
	Trigger()
}

// SignalsTabModel renders the signal queue and ML training status in Tab 2.
type SignalsTabModel struct {
	readModel innersignal.ReadModel
	mlEngine  MLEngine
	messages  []innersignal.Message
	quit      bool

	width  int
	height int
	offset int // lines scrolled up from the bottom; 0 = newest entries visible

	styles signalStyles
}

type signalStyles struct {
	ts      lipgloss.Style // timestamp — dimmed
	sigType lipgloss.Style // signal type (BUY/SELL/…) — rosa bold, matches table Selected
	summary lipgloss.Style // signal summary — terminal default (white)
	empty   lipgloss.Style // placeholder when no signals
	status  lipgloss.Style // nav hints
}

// NewSignalsTabModelWithML creates the Tab 2 model with optional ML status.
func NewSignalsTabModelWithML(readModel innersignal.ReadModel, ml MLEngine) SignalsTabModel {
	return SignalsTabModel{
		readModel: readModel,
		mlEngine:  ml,
		styles: signalStyles{
			ts:      lipgloss.NewStyle().Foreground(lipgloss.Color("242")),
			sigType: lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("212")),
			summary: lipgloss.NewStyle(),
			empty:   lipgloss.NewStyle().Foreground(lipgloss.Color("244")),
			status:  lipgloss.NewStyle().Foreground(lipgloss.Color("242")),
		},
	}
}

func (m SignalsTabModel) Init() tea.Cmd {
	return tea.Batch(signalsTickCmd(), m.refreshCmd())
}

func (m SignalsTabModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if m.quit {
		return m, nil
	}

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		// ML engine controls.
		case "p":
			if m.mlEngine != nil {
				if m.mlEngine.Status() == 1 { // StatusRunning
					m.mlEngine.Pause()
				} else if m.mlEngine.Status() == 2 { // StatusPaused
					m.mlEngine.Resume()
				}
			}
		case "r":
			if m.mlEngine != nil {
				m.mlEngine.Trigger()
			}
		// Scroll controls.
		case "up", "k":
			maxOff := max(0, len(m.messages)-m.visibleLines())
			if m.offset < maxOff {
				m.offset++
			}
		case "down", "j":
			if m.offset > 0 {
				m.offset--
			}
		case "g":
			m.offset = max(0, len(m.messages)-m.visibleLines())
		case "G":
			m.offset = 0
		}

	case signalsTickMsg:
		return m, tea.Batch(signalsTickCmd(), m.refreshCmd())

	case signalsMsg:
		m.messages = msg.messages
		// Clamp scroll offset after a refresh so it never exceeds the new range.
		maxOff := max(0, len(m.messages)-m.visibleLines())
		if m.offset > maxOff {
			m.offset = maxOff
		}
		return m, nil
	}

	return m, nil
}

// visibleLines returns how many signal rows fit on screen.
// All chrome (ML status, nav hints, scroll indicator) is surfaced via NavHint()
// in the root help line, so the full content height is available here.
func (m SignalsTabModel) visibleLines() int {
	if m.height < 1 {
		return 20
	}
	return m.height
}

// NavHint returns the context-sensitive hint string that the root model merges
// into its global help line when the Signals tab is active.
func (m SignalsTabModel) NavHint() string {
	parts := []string{}

	if m.mlEngine != nil {
		label := mlStatusLabel(m.mlEngine.Status())
		parts = append(parts, fmt.Sprintf("ML: %s · p: pause/resume · r: run now", label))
	}

	nav := "↑/↓ scroll · g top · G bottom"
	if len(m.messages) > 0 {
		total := len(m.messages)
		end := total - m.offset
		scrollHint := ""
		if m.offset > 0 {
			scrollHint = fmt.Sprintf(" ↑ %d more below", m.offset)
		}
		nav = fmt.Sprintf("%d/%d signals · %s%s", end, total, nav, scrollHint)
	}
	parts = append(parts, nav)

	return strings.Join(parts, "  |  ")
}

func (m SignalsTabModel) View() string {
	if m.quit {
		return ""
	}

	if len(m.messages) == 0 {
		return m.styles.empty.Render("No queued signals yet.")
	}

	visible := m.visibleLines()
	total := len(m.messages)
	end := total - m.offset
	start := max(end-visible, 0)
	slice := m.messages[start:end]

	lines := make([]string, 0, len(slice))
	for i := range slice {
		row := m.styles.ts.Render(slice[i].CreatedAt.Format(time.RFC3339)) +
			" │ " +
			m.styles.sigType.Render(fmt.Sprintf("%-22s", slice[i].Type)) +
			" │ " +
			m.styles.summary.Render(slice[i].Summary)
		lines = append(lines, row)
	}

	return strings.Join(lines, "\n")
}

func mlStatusLabel(status int32) string {
	switch status {
	case 1:
		return "running"
	case 2:
		return "paused"
	case 3:
		return "done"
	default:
		return "idle"
	}
}

func (m SignalsTabModel) refreshCmd() tea.Cmd {
	if m.readModel == nil {
		return func() tea.Msg { return signalsMsg{messages: nil} }
	}
	return func() tea.Msg {
		return signalsMsg{messages: m.readModel.Pending()}
	}
}

func signalsTickCmd() tea.Cmd {
	return tea.Tick(signalsRefreshInterval, func(t time.Time) tea.Msg { return signalsTickMsg(t) })
}
