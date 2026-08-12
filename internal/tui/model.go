package tui

import (
	"context"
	"fmt"
	"time"

	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/tgragnato/orbiter/internal/portfolio"
	"github.com/tgragnato/orbiter/internal/portfolio/analytics"
)

const refreshInterval = time.Hour
const defaultPortfolioID = "MAIN"

const (
	modeNormal = iota
	modeAddTx
)

type tickMsg time.Time

type holdingsMsg struct {
	holdings          []portfolio.Holding
	summary           portfolio.Summary
	unrealizedPnL     float64
	realized          float64
	dividendsBySymbol map[string]float64
	err               error
}

type toggledMsg struct {
	holdingID int64
	err       error
}

type taaToggledMsg struct {
	symbol string
	err    error
}

type txSubmittedMsg struct {
	err error
}

// TWRReader is kept as an exported interface for backward compatibility.
// The holdings model no longer uses it — unrealized PnL is computed directly
// from holdings data so that results are non-zero without requiring nav_snapshots.
type TWRReader interface {
	CalculateTWR(ctx context.Context, portfolioID string, tr analytics.TimeRange) (analytics.TWRResult, error)
}

// RealizedPnLReader is kept as an exported interface for backward compatibility.
// The holdings model now reads realized PnL via HoldingsStore.TotalRealizedPnL.
type RealizedPnLReader interface {
	TotalRealizedPnL(ctx context.Context) (float64, error)
}

// Model renders a unified holdings table with core/satellite toggling and
// an optional transaction-entry form (requires TransactionStore).
type Model struct {
	store   portfolio.HoldingsStore
	txStore portfolio.TransactionStore

	portfolioID  string
	baseCurrency string // ISO 4217 portfolio base currency shown in summaryView

	table             table.Model
	holdings          []portfolio.Holding
	summary           portfolio.Summary
	unrealizedPnL     float64
	realized          float64
	dividendsBySymbol map[string]float64
	status            string
	loading           bool
	loadError         error
	quit              bool

	mode int
	form transactionFormModel

	width  int
	height int

	styles styles
}

type styles struct {
	title        lipgloss.Style
	summary      lipgloss.Style
	status       lipgloss.Style
	error        lipgloss.Style
	coreBadge    lipgloss.Style
	satelliteBad lipgloss.Style
}

// NewModelWithAll builds a holdings model with all optional dependencies,
// including a TransactionStore that enables the 'a: add trade' keybinding.
func NewModelWithAll(store portfolio.HoldingsStore, txStore portfolio.TransactionStore, portfolioID string) Model {
	return newModelCore(store, txStore, portfolioID)
}

// WithBaseCurrency sets the ISO 4217 base currency label shown in the summary
// bar (e.g. "EUR"). Pass "" to hide the label.
func (m Model) WithBaseCurrency(currency string) Model {
	m.baseCurrency = currency
	return m
}

func newModelCore(store portfolio.HoldingsStore, txStore portfolio.TransactionStore, portfolioID string) Model {
	if portfolioID == "" {
		portfolioID = defaultPortfolioID
	}

	cols := []table.Column{
		{Title: "Sleeve", Width: 12},
		{Title: "TAA", Width: 5},
		{Title: "Symbol", Width: 12},
		{Title: "Ccy", Width: 5},
		{Title: "Qty", Width: 10},
		{Title: "PMC", Width: 10},
		{Title: "Price", Width: 10},
		{Title: "P&L%", Width: 9},
		{Title: "NAV", Width: 12},
		{Title: "Dividends", Width: 10},
	}

	tbl := table.New(
		table.WithColumns(cols),
		table.WithRows([]table.Row{}),
		table.WithFocused(true),
		table.WithHeight(10),
	)
	tbl.KeyMap.LineUp.SetKeys("up", "k")
	tbl.KeyMap.LineDown.SetKeys("down", "j")

	return Model{
		store:       store,
		txStore:     txStore,
		portfolioID: portfolioID,
		table:       tbl,
		styles: styles{
			title: lipgloss.NewStyle().Bold(true),
			summary: lipgloss.NewStyle().
				Foreground(lipgloss.Color("252")).
				Background(lipgloss.Color("24")).
				Padding(0, 1),
			status: lipgloss.NewStyle().Foreground(lipgloss.Color("242")),
			error:  lipgloss.NewStyle().Foreground(lipgloss.Color("196")),
			coreBadge: lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("230")).
				Background(lipgloss.Color("28")).
				Padding(0, 1),
			satelliteBad: lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("232")).
				Background(lipgloss.Color("214")).
				Padding(0, 1),
		},
		status:  "Loading holdings...",
		loading: true,
	}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(tickCmd(), m.refreshCmd())
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if m.quit {
		return m, nil
	}

	// Form mode: route messages to the form sub-model.
	if m.mode == modeAddTx {
		switch msg := msg.(type) {
		case txFormResultMsg:
			if msg.cancelled {
				m.mode = modeNormal
				m.status = "Transaction cancelled"
				return m, nil
			}
			m.mode = modeNormal
			m.loading = true
			m.status = "Adding transaction..."
			return m, m.addTransactionCmd(*msg.tx)

		case txSubmittedMsg:
			m.loading = false
			if msg.err != nil {
				m.status = fmt.Sprintf("Transaction failed: %v", msg.err)
				return m, nil
			}
			m.status = "Transaction recorded"
			return m, m.refreshCmd()

		case tea.WindowSizeMsg:
			m.width = msg.Width
			m.height = msg.Height
			m.table.SetWidth(max(40, msg.Width-4))
			// SetHeight controls data rows only; bubbles/table also renders 1 header row.
		// Fixed chrome in the holdings body: summary(1) + table-header(1) = 2 rows.
		m.table.SetHeight(max(3, msg.Height-2))
			return m, nil

		default:
			var cmd tea.Cmd
			m.form, cmd = m.form.Update(msg)
			return m, cmd
		}
	}

	// Normal mode.
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			m.quit = true
			return m, tea.Quit

		case "t":
			if m.loading || len(m.holdings) == 0 || m.store == nil {
				return m, nil
			}
			selected := m.table.Cursor()
			if selected < 0 || selected >= len(m.holdings) {
				return m, nil
			}
			holdingID := m.holdings[selected].ID
			m.status = "Toggling allocation..."
			m.loading = true
			return m, m.toggleCmd(holdingID)

		case "x":
			if m.loading || len(m.holdings) == 0 || m.store == nil {
				return m, nil
			}
			selected := m.table.Cursor()
			if selected < 0 || selected >= len(m.holdings) {
				return m, nil
			}
			symbol := m.holdings[selected].Symbol
			m.status = "Toggling TAA..."
			m.loading = true
			return m, m.taaToggleCmd(symbol)

		case "a":
			if m.txStore == nil || m.loading {
				return m, nil
			}
			m.mode = modeAddTx
			var cmd tea.Cmd
			m.form, cmd = newTransactionForm(m.knownSymbols())
			return m, cmd
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.table.SetWidth(max(40, msg.Width-4))
		// SetHeight controls data rows only; bubbles/table also renders 1 header row.
		// Fixed chrome in the holdings body: summary(1) + table-header(1) = 2 rows.
		m.table.SetHeight(max(3, msg.Height-2))
		return m, nil

	case tickMsg:
		if m.loading {
			return m, tickCmd()
		}
		m.loading = true
		return m, tea.Batch(tickCmd(), m.refreshCmd())

	case holdingsMsg:
		m.loading = false
		m.loadError = msg.err
		if msg.err != nil {
			m.status = "Load failed"
			return m, nil
		}
		m.holdings = msg.holdings
		m.summary = msg.summary
		m.unrealizedPnL = msg.unrealizedPnL
		m.realized = msg.realized
		m.dividendsBySymbol = msg.dividendsBySymbol
		m.status = fmt.Sprintf("%d holdings loaded", len(msg.holdings))
		m.syncRows()
		return m, nil

	case toggledMsg:
		if msg.err != nil {
			m.loading = false
			m.loadError = msg.err
			m.status = "Toggle failed"
			return m, nil
		}
		m.status = "Allocation updated"
		return m, m.refreshCmd()

	case taaToggledMsg:
		if msg.err != nil {
			m.loading = false
			m.loadError = msg.err
			m.status = "TAA toggle failed"
			return m, nil
		}
		m.status = "TAA updated"
		return m, m.refreshCmd()

	case txSubmittedMsg:
		m.loading = false
		if msg.err != nil {
			m.status = fmt.Sprintf("Transaction failed: %v", msg.err)
			return m, nil
		}
		m.status = "Transaction recorded"
		return m, m.refreshCmd()
	}

	var cmd tea.Cmd
	m.table, cmd = m.table.Update(msg)
	return m, cmd
}

// NavHint returns the context-sensitive hint string that the root model merges
// into its global help line when the Holdings tab is active.
func (m Model) NavHint() string {
	if m.mode == modeAddTx {
		return "esc: cancel"
	}
	nav := "t: core↔sat · x: toggle TAA"
	if m.txStore != nil {
		nav = "a: add trade · " + nav
	}
	if m.status != "" {
		return m.status + "  |  " + nav
	}
	return nav
}

func (m Model) View() string {
	if m.quit {
		return ""
	}

	summary := m.summaryView()

	if m.mode == modeAddTx {
		return lipgloss.JoinVertical(lipgloss.Left, summary, m.form.View())
	}

	if m.loadError != nil {
		errLine := m.styles.error.Render(fmt.Sprintf("Error: %v", m.loadError))
		return lipgloss.JoinVertical(lipgloss.Left, summary, errLine)
	}

	return lipgloss.JoinVertical(lipgloss.Left, summary, m.table.View())
}

func (m Model) refreshCmd() tea.Cmd {
	if m.store == nil {
		return func() tea.Msg {
			return holdingsMsg{err: nil, holdings: nil, summary: portfolio.Summary{}}
		}
	}

	return func() tea.Msg {
		holdings, err := m.store.ListHoldings(context.Background())
		if err != nil {
			return holdingsMsg{err: err}
		}

		// Unrealized PnL: sum of (price − avg_cost) × qty for active holdings.
		unrealizedPnL := 0.0
		for _, h := range holdings {
			if h.Quantity > 0 && h.PMC > 0 {
				unrealizedPnL += h.Quantity * (h.MarketPrice - h.PMC)
			}
		}

		// Realized PnL: computed from the full transaction history.
		realizedPnL := 0.0
		if total, err := m.store.TotalRealizedPnL(context.Background()); err == nil {
			realizedPnL = total
		}

		// Per-symbol dividend income: available when the store implements the optional method.
		type dividendReader interface {
			ListDividendIncome(ctx context.Context) ([]portfolio.DividendRecord, error)
		}
		dividendsBySymbol := make(map[string]float64)
		if dr, ok := any(m.txStore).(dividendReader); ok && m.txStore != nil {
			if records, err := dr.ListDividendIncome(context.Background()); err == nil {
				for _, r := range records {
					dividendsBySymbol[r.Symbol] += r.IncomeAmount
				}
			}
		}

		return holdingsMsg{
			holdings:          holdings,
			summary:           portfolio.BuildSummary(holdings),
			unrealizedPnL:     unrealizedPnL,
			realized:          realizedPnL,
			dividendsBySymbol: dividendsBySymbol,
		}
	}
}

func (m Model) toggleCmd(holdingID int64) tea.Cmd {
	if m.store == nil {
		return func() tea.Msg { return toggledMsg{holdingID: holdingID} }
	}

	return func() tea.Msg {
		err := m.store.ToggleAllocation(context.Background(), holdingID)
		return toggledMsg{holdingID: holdingID, err: err}
	}
}

func (m Model) taaToggleCmd(symbol string) tea.Cmd {
	return func() tea.Msg {
		if m.store == nil {
			return taaToggledMsg{symbol: symbol}
		}
		err := m.store.ToggleTAAEnabled(context.Background(), symbol)
		return taaToggledMsg{symbol: symbol, err: err}
	}
}

func (m Model) addTransactionCmd(tx portfolio.Transaction) tea.Cmd {
	if m.txStore == nil {
		return func() tea.Msg { return txSubmittedMsg{} }
	}
	return func() tea.Msg {
		err := m.txStore.AddTransaction(context.Background(), tx)
		return txSubmittedMsg{err: err}
	}
}

func (m *Model) syncRows() {
	rows := make([]table.Row, 0, len(m.holdings))
	for _, h := range m.holdings {
		taaStr := "[TAA]"
		if !h.TAAEnabled {
			taaStr = "[ - ]"
		}

		divIncome := m.dividendsBySymbol[h.Symbol]
		divStr := "--"
		if divIncome != 0 {
			divStr = fmt.Sprintf("%+.2f", divIncome)
		}

		ccyStr := h.Currency
		if ccyStr == "" {
			ccyStr = "---"
		}

		if h.Quantity <= 0 {
			// Plain-text sleeve label — bubbles/table uses runewidth.Truncate which
			// counts printable ANSI chars as width, so styled strings get mangled.
			sleeveBadge := "[SAT]"
			if h.AllocationType == portfolio.AllocationCore {
				sleeveBadge = "[CORE]"
			}
			rows = append(rows, table.Row{
				sleeveBadge,
				taaStr,
				h.Symbol,
				ccyStr,
				"0.0000",
				"--",
				fmt.Sprintf("%.2f", h.MarketPrice),
				"--",
				"0.00",
				divStr,
			})
			continue
		}

		allocation := "[SAT]"
		if h.AllocationType == portfolio.AllocationCore {
			allocation = "[CORE]"
		}

		pmcStr := "--"
		pnlStr := "--"
		if h.PMC > 0 {
			pmcStr = fmt.Sprintf("%.2f", h.PMC)
			pnlPct := (h.MarketPrice - h.PMC) / h.PMC * 100
			pnlStr = fmt.Sprintf("%+.1f%%", pnlPct)
		}

		rows = append(rows, table.Row{
			allocation,
			taaStr,
			h.Symbol,
			ccyStr,
			fmt.Sprintf("%.4f", h.Quantity),
			pmcStr,
			fmt.Sprintf("%.2f", h.MarketPrice),
			pnlStr,
			fmt.Sprintf("%.2f", h.NAV()),
			divStr,
		})
	}
	m.table.SetRows(rows)
}

func (m Model) knownSymbols() []string {
	syms := make([]string, 0, len(m.holdings))
	for _, h := range m.holdings {
		syms = append(syms, h.Symbol)
	}
	return syms
}

func (m Model) summaryView() string {
	ccySuffix := ""
	if m.baseCurrency != "" {
		ccySuffix = " " + m.baseCurrency
	}
	return m.styles.summary.Render(fmt.Sprintf(
		"NAV: %.2f%s | Core: %.2f (%.1f%%) | Satellite: %.2f (%.1f%%) | Unreal. PnL: %+.2f%s | Real. PnL: %+.2f%s",
		m.summary.TotalNAV, ccySuffix,
		m.summary.CoreNAV,
		m.summary.CorePercent,
		m.summary.SatelliteNAV,
		m.summary.SatellitePercent,
		m.unrealizedPnL, ccySuffix,
		m.realized, ccySuffix,
	))
}


func tickCmd() tea.Cmd {
	return tea.Tick(refreshInterval, func(t time.Time) tea.Msg { return tickMsg(t) })
}
