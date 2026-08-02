package tui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	innersignal "github.com/tgragnato/orbiter/internal/signal"
)

const (
	signalsRefreshInterval = time.Hour
	mlLogMaxLines          = 20 // visible ML log lines kept in the ring buffer
)

type signalsTickMsg time.Time

type signalsMsg struct {
	messages []innersignal.Message
}

type mlLogMsg struct {
	line string
}

type mlStatusMsg struct {
	status int32 // mirrors ml.Status* constants
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
	// LogsChan returns the channel of log lines streamed from the training goroutine.
	LogsChan() chan string
}

// SignalsTabModel renders the signal queue and ML training status in Tab 2.
type SignalsTabModel struct {
	readModel innersignal.ReadModel
	mlEngine  MLEngine
	messages  []innersignal.Message
	mlLogs    []string // ring buffer of recent ML log lines
	quit      bool

	styles signalStyles
}

type signalStyles struct {
	title   lipgloss.Style
	line    lipgloss.Style
	empty   lipgloss.Style
	status  lipgloss.Style
	mlLog   lipgloss.Style
	mlTitle lipgloss.Style
}

// NewSignalsTabModel creates the Tab 2 signal queue model.
func NewSignalsTabModel(readModel innersignal.ReadModel) SignalsTabModel {
	return NewSignalsTabModelWithML(readModel, nil)
}

// NewSignalsTabModelWithML creates the Tab 2 model with optional ML status.
func NewSignalsTabModelWithML(readModel innersignal.ReadModel, ml MLEngine) SignalsTabModel {
	return SignalsTabModel{
		readModel: readModel,
		mlEngine:  ml,
		styles: signalStyles{
			title:   lipgloss.NewStyle().Bold(true),
			line:    lipgloss.NewStyle().Foreground(lipgloss.Color("252")),
			empty:   lipgloss.NewStyle().Foreground(lipgloss.Color("244")),
			status:  lipgloss.NewStyle().Foreground(lipgloss.Color("242")),
			mlLog:   lipgloss.NewStyle().Foreground(lipgloss.Color("33")),
			mlTitle: lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39")),
		},
	}
}

func (m SignalsTabModel) Init() tea.Cmd {
	return tea.Batch(signalsTickCmd(), m.refreshCmd(), m.drainMLLogsCmd())
}

func (m SignalsTabModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if m.quit {
		return m, nil
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
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
		}
	case signalsTickMsg:
		return m, tea.Batch(signalsTickCmd(), m.refreshCmd(), m.drainMLLogsCmd())
	case signalsMsg:
		m.messages = msg.messages
		return m, nil
	case mlLogMsg:
		m.mlLogs = appendRing(m.mlLogs, msg.line, mlLogMaxLines)
		return m, nil
	}

	return m, nil
}

func (m SignalsTabModel) View() string {
	if m.quit {
		return ""
	}

	sections := []string{m.styles.title.Render("Tab 2 - Signals")}

	// Signal queue.
	if len(m.messages) == 0 {
		sections = append(sections, m.styles.empty.Render("No queued signals yet."))
	} else {
		for _, msg := range m.messages {
			line := fmt.Sprintf("%s | %-22s | %s", msg.CreatedAt.Format(time.RFC3339), msg.Type, msg.Summary)
			sections = append(sections, m.styles.line.Render(line))
		}
	}
	sections = append(sections, m.styles.status.Render(fmt.Sprintf("Queued messages: %d", len(m.messages))))

	// ML training panel (only shown when an engine is wired).
	if m.mlEngine != nil {
		sections = append(sections, m.renderMLPanel())
	}

	return strings.Join(sections, "\n")
}

func (m SignalsTabModel) renderMLPanel() string {
	statusLabel := mlStatusLabel(m.mlEngine.Status())
	header := m.styles.mlTitle.Render(fmt.Sprintf("ML Engine — %s  [p: pause/resume | r: run now]", statusLabel))
	lines := []string{header}

	if len(m.mlLogs) == 0 {
		lines = append(lines, m.styles.empty.Render("No training logs yet."))
	} else {
		for _, l := range m.mlLogs {
			lines = append(lines, m.styles.mlLog.Render(l))
		}
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

// drainMLLogsCmd reads one log line from the ML engine's channel (non-blocking)
// and returns it as a mlLogMsg. This keeps UI latency low while the training
// goroutine emits at its own pace.
func (m SignalsTabModel) drainMLLogsCmd() tea.Cmd {
	if m.mlEngine == nil {
		return nil
	}
	ch := m.mlEngine.LogsChan()
	return func() tea.Msg {
		select {
		case line := <-ch:
			return mlLogMsg{line: line}
		default:
			return nil
		}
	}
}

func signalsTickCmd() tea.Cmd {
	return tea.Tick(signalsRefreshInterval, func(t time.Time) tea.Msg { return signalsTickMsg(t) })
}

// appendRing appends line to buf and trims it to maxLen from the end.
func appendRing(buf []string, line string, maxLen int) []string {
	buf = append(buf, line)
	if len(buf) > maxLen {
		buf = buf[len(buf)-maxLen:]
	}
	return buf
}
