package tui

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	lipglosstable "github.com/charmbracelet/lipgloss/table"
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
	offset int // rows scrolled up from the bottom; 0 = highest-conviction entries visible

	styles signalStyles
}

type signalStyles struct {
	borderStyle lipgloss.Style // table column separators and header border
	headerStyle lipgloss.Style // column headers
	empty       lipgloss.Style // placeholder when no signals
	status      lipgloss.Style // nav hints
}

// NewSignalsTabModelWithML creates the Tab 2 model with optional ML status.
func NewSignalsTabModelWithML(readModel innersignal.ReadModel, ml MLEngine) SignalsTabModel {
	return SignalsTabModel{
		readModel: readModel,
		mlEngine:  ml,
		styles: signalStyles{
			borderStyle: lipgloss.NewStyle().Foreground(lipgloss.Color("240")),
			headerStyle: lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("252")),
			empty:       lipgloss.NewStyle().Foreground(lipgloss.Color("244")),
			status:      lipgloss.NewStyle().Foreground(lipgloss.Color("242")),
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
		// Sort by absolute conviction descending so highest-conviction signals appear first.
		sort.SliceStable(m.messages, func(i, j int) bool {
			return math.Abs(m.messages[i].Conviction) > math.Abs(m.messages[j].Conviction)
		})
		// Clamp scroll offset after a refresh so it never exceeds the new range.
		maxOff := max(0, len(m.messages)-m.visibleLines())
		if m.offset > maxOff {
			m.offset = maxOff
		}
		return m, nil
	}

	return m, nil
}

// visibleLines returns how many signal data rows fit on screen.
// Two lines are reserved for the lipgloss table header + header-border separator.
func (m SignalsTabModel) visibleLines() int {
	if m.height < 3 {
		return 1
	}
	return m.height - 2
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

	rows := make([][]string, len(slice))
	for i, msg := range slice {
		rows[i] = signalTableRow(msg)
	}

	headerStyle := m.styles.headerStyle
	borderStyle := m.styles.borderStyle

	t := lipglosstable.New().
		BorderTop(false).
		BorderBottom(false).
		BorderLeft(false).
		BorderRight(false).
		BorderHeader(true).
		BorderColumn(true).
		BorderStyle(borderStyle).
		Headers("Time", "Type", "Instrument", "cv", "Allocation", "Δ EUR").
		Rows(rows...).
		StyleFunc(func(row, col int) lipgloss.Style {
			if row == lipglosstable.HeaderRow {
				return headerStyle
			}
			if row >= len(slice) {
				return lipgloss.NewStyle()
			}
			return signalCellStyle(slice[row], col)
		})

	if m.width > 0 {
		t = t.Width(m.width)
	}

	return t.String()
}

// signalTableRow converts a Message into a table row (plain strings, no ANSI).
// Styling is applied separately via signalCellStyle so lipgloss/table can
// measure column widths correctly.
func signalTableRow(msg innersignal.Message) []string {
	ts := msg.CreatedAt.Format("2006-01-02 15:04:05")
	typ := signalTypeLabel(msg)
	instr := msg.Instrument

	switch msg.Type {
	case innersignal.TypeCorePMCFloorAlert:
		return []string{
			ts, typ, instr,
			"—",
			fmt.Sprintf("mkt %.4f", msg.MarketPrice),
			fmt.Sprintf("pmc %.4f", msg.PMC),
		}
	default:
		return []string{
			ts, typ, instr,
			fmt.Sprintf("%+.2f", msg.Conviction),
			signalWeightsLabel(msg),
			fmt.Sprintf("%+.0f%s", msg.Delta, currencySymbol(msg.Currency)),
		}
	}
}

// signalCellStyle returns the lipgloss style for a given cell position.
func signalCellStyle(msg innersignal.Message, col int) lipgloss.Style {
	switch col {
	case 0: // Time
		return lipgloss.NewStyle().Foreground(lipgloss.Color("242"))
	case 1: // Type
		return lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("212"))
	case 2: // Instrument
		return lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("255"))
	case 3: // cv / "—"
		if msg.Type == innersignal.TypeCorePMCFloorAlert {
			return lipgloss.NewStyle().Foreground(lipgloss.Color("242"))
		}
		return signalConvictionStyle(msg.Conviction)
	case 4: // Allocation / mkt price
		if msg.Type == innersignal.TypeCorePMCFloorAlert {
			return lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("214"))
		}
		return lipgloss.NewStyle().Foreground(lipgloss.Color("250"))
	case 5: // Δ EUR / pmc
		if msg.Type == innersignal.TypeCorePMCFloorAlert {
			return lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("214"))
		}
		return signalDeltaStyle(msg.Delta)
	}
	return lipgloss.NewStyle()
}

// signalTypeLabel returns a display-friendly short label for each signal type.
// BUY is rendered as LONG or SHORT depending on conviction sign: positive conviction
// means a long entry, negative means a short entry.
func signalTypeLabel(msg innersignal.Message) string {
	switch msg.Type {
	case innersignal.TypeBuy:
		if msg.Conviction < 0 {
			return "SHORT"
		}
		return "LONG"
	case innersignal.TypeSell:
		return "SELL"
	case innersignal.TypeRebalance:
		return "REBALANCE"
	case innersignal.TypeCorePMCFloorAlert:
		return "PMC ALERT"
	default:
		return string(msg.Type)
	}
}

// signalConvictionStyle maps conviction magnitude to a color:
//   - |cv| ≥ 0.7 → bold green (long) / bold red (short)
//   - |cv| ≥ 0.4 → green / red
//   - |cv| < 0.4 → yellow (weak conviction)
func signalConvictionStyle(cv float64) lipgloss.Style {
	abs := math.Abs(cv)
	switch {
	case abs >= 0.7 && cv >= 0:
		return lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("82"))
	case abs >= 0.7:
		return lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("203"))
	case abs >= 0.4 && cv >= 0:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("78"))
	case abs >= 0.4:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("167"))
	default:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("220"))
	}
}

// signalDeltaStyle returns green for inflows (buy) and red for outflows (sell).
func signalDeltaStyle(delta float64) lipgloss.Style {
	if delta >= 0 {
		return lipgloss.NewStyle().Foreground(lipgloss.Color("82"))
	}
	return lipgloss.NewStyle().Foreground(lipgloss.Color("203"))
}

// signalWeightsLabel formats the allocation change for the Allocation column.
// BUY shows only the target (entering from zero); SELL shows only current (exiting to zero);
// REBALANCE shows the full before→after transition.
func signalWeightsLabel(msg innersignal.Message) string {
	switch msg.Type {
	case innersignal.TypeBuy:
		return fmt.Sprintf("%5.1f%% → %5.1f%%", 0.0, msg.TargetWeight*100)
	case innersignal.TypeSell:
		return fmt.Sprintf("%5.1f%% →", msg.CurrentWeight*100)
	case innersignal.TypeRebalance:
		return fmt.Sprintf("%5.1f%% → %5.1f%%", msg.CurrentWeight*100, msg.TargetWeight*100)
	default:
		return ""
	}
}

// currencySymbol maps an ISO 4217 code to its common display symbol.
// Falls back to the code itself for unknown currencies.
func currencySymbol(code string) string {
	switch code {
	case "EUR":
		return "€"
	case "USD":
		return "$"
	case "GBP":
		return "£"
	case "JPY":
		return "¥"
	case "CHF":
		return "Fr"
	default:
		if code == "" {
			return "€" // legacy signals without currency stamped
		}
		return code
	}
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
