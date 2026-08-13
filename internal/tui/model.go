package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/bubbles/textinput"
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
	modeAddWatchlist
)

// rowKind identifies whether a table row represents a held position or a
// watchlist entry. This lets key-handlers behave differently per row type
// without duplicating the table.
type rowKind int

const (
	rowKindHolding   rowKind = iota
	rowKindWatchlist         // no position yet; tracked for entry signals
)

type rowEntry struct {
	kind         rowKind
	holdingIdx   int // valid when kind == rowKindHolding
	watchlistIdx int // valid when kind == rowKindWatchlist
}

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

type watchlistLoadedMsg struct {
	items []portfolio.WatchlistItem
	err   error
}

type watchlistMutatedMsg struct {
	err error
}

// TWRReader is kept as an exported interface for backward compatibility.
type TWRReader interface {
	CalculateTWR(ctx context.Context, portfolioID string, tr analytics.TimeRange) (analytics.TWRResult, error)
}

// RealizedPnLReader is kept as an exported interface for backward compatibility.
type RealizedPnLReader interface {
	TotalRealizedPnL(ctx context.Context) (float64, error)
}

// Model renders a unified holdings table with core/satellite toggling,
// an optional transaction-entry form (requires TransactionStore), and
// watchlist rows integrated directly into the same table.
type Model struct {
	store   portfolio.HoldingsStore
	txStore portfolio.TransactionStore

	portfolioID  string
	baseCurrency string

	table             table.Model
	holdings          []portfolio.Holding
	rows              []rowEntry // parallel to table rows; tracks holding vs watchlist
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

	// watchlist
	watchlistStore portfolio.WatchlistStore
	watchlist      []portfolio.WatchlistItem
	watchlistInput watchlistInputModel

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

// NewModelWithAll builds a holdings model with all optional dependencies.
func NewModelWithAll(store portfolio.HoldingsStore, txStore portfolio.TransactionStore, portfolioID string) Model {
	return newModelCore(store, txStore, portfolioID)
}

// WithBaseCurrency sets the ISO 4217 base currency label shown in the summary bar.
func (m Model) WithBaseCurrency(currency string) Model {
	m.baseCurrency = currency
	return m
}

// WithWatchlist enables watchlist rows inside the unified holdings table.
// Watchlist items appear at the bottom of the table with sleeve badge [WL].
func (m Model) WithWatchlist(ws portfolio.WatchlistStore) Model {
	m.watchlistStore = ws
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
	return tea.Batch(tickCmd(), m.refreshCmd(), m.watchlistRefreshCmd())
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if m.quit {
		return m, nil
	}

	// Watchlist add form.
	if m.mode == modeAddWatchlist {
		switch msg := msg.(type) {
		case watchlistInputResultMsg:
			m.mode = modeNormal
			if msg.cancelled || msg.symbol == "" {
				m.status = "Cancelled"
				return m, nil
			}
			m.status = fmt.Sprintf("Adding %s to watchlist...", msg.symbol)
			return m, m.addToWatchlistCmd(msg.symbol)

		case tea.WindowSizeMsg:
			m.width = msg.Width
			m.height = msg.Height
			m.recomputeHeights()
			return m, nil

		default:
			var cmd tea.Cmd
			m.watchlistInput, cmd = m.watchlistInput.Update(msg)
			return m, cmd
		}
	}

	// Transaction form mode.
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
			m.recomputeHeights()
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

		case "w":
			if m.watchlistStore == nil || m.loading {
				return m, nil
			}
			m.mode = modeAddWatchlist
			var cmd tea.Cmd
			m.watchlistInput, cmd = newWatchlistInput(m.knownSymbols())
			return m, cmd

		case "d":
			// Remove the selected watchlist row (no-op on holding rows).
			if m.watchlistStore == nil {
				return m, nil
			}
			sel := m.table.Cursor()
			if sel < 0 || sel >= len(m.rows) {
				return m, nil
			}
			re := m.rows[sel]
			if re.kind != rowKindWatchlist {
				return m, nil
			}
			sym := m.watchlist[re.watchlistIdx].Symbol
			m.status = fmt.Sprintf("Removing %s from watchlist...", sym)
			return m, m.removeFromWatchlistCmd(sym)

		case "t":
			if m.loading || m.store == nil {
				return m, nil
			}
			sel := m.table.Cursor()
			if sel < 0 || sel >= len(m.rows) || m.rows[sel].kind != rowKindHolding {
				return m, nil
			}
			holdingID := m.holdings[m.rows[sel].holdingIdx].ID
			m.status = "Toggling allocation..."
			m.loading = true
			return m, m.toggleCmd(holdingID)

		case "x":
			if m.loading || m.store == nil {
				return m, nil
			}
			sel := m.table.Cursor()
			if sel < 0 || sel >= len(m.rows) || m.rows[sel].kind != rowKindHolding {
				return m, nil
			}
			symbol := m.holdings[m.rows[sel].holdingIdx].Symbol
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
		m.recomputeHeights()
		return m, nil

	case tickMsg:
		if m.loading {
			return m, tickCmd()
		}
		m.loading = true
		return m, tea.Batch(tickCmd(), m.refreshCmd(), m.watchlistRefreshCmd())

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

	case watchlistLoadedMsg:
		if msg.err != nil {
			return m, nil
		}
		m.watchlist = msg.items
		m.syncRows() // re-render unified table with updated watchlist rows
		return m, nil

	case watchlistMutatedMsg:
		if msg.err != nil {
			m.status = fmt.Sprintf("Watchlist error: %v", msg.err)
			return m, nil
		}
		m.status = "Watchlist updated"
		return m, m.watchlistRefreshCmd()

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

// NavHint returns the context-sensitive hint shown in the root model's help line.
func (m Model) NavHint() string {
	if m.mode == modeAddTx {
		return "esc: cancel"
	}
	if m.mode == modeAddWatchlist {
		return "enter: confirm · esc: cancel"
	}

	// Context-aware hint based on the currently selected row.
	sel := m.table.Cursor()
	onWatchlist := sel >= 0 && sel < len(m.rows) && m.rows[sel].kind == rowKindWatchlist

	var nav string
	if onWatchlist {
		nav = "d: remove from watchlist"
	} else {
		nav = "t: core↔sat · x: toggle TAA"
	}
	if m.watchlistStore != nil {
		nav += " · w: add to watchlist"
	}
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

	if m.mode == modeAddWatchlist {
		return lipgloss.JoinVertical(lipgloss.Left, summary, m.watchlistInput.View())
	}

	if m.loadError != nil {
		errLine := m.styles.error.Render(fmt.Sprintf("Error: %v", m.loadError))
		return lipgloss.JoinVertical(lipgloss.Left, summary, errLine)
	}

	return lipgloss.JoinVertical(lipgloss.Left, summary, m.table.View())
}

// ── Commands ────────────────────────────────────────────────────────────────

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

		unrealizedPnL := 0.0
		for _, h := range holdings {
			if h.Quantity > 0 && h.PMC > 0 {
				unrealizedPnL += h.Quantity * (h.MarketPrice - h.PMC)
			}
		}

		realizedPnL := 0.0
		if total, err := m.store.TotalRealizedPnL(context.Background()); err == nil {
			realizedPnL = total
		}

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

func (m Model) watchlistRefreshCmd() tea.Cmd {
	if m.watchlistStore == nil {
		return nil
	}
	return func() tea.Msg {
		items, err := m.watchlistStore.ListWatchlist(context.Background())
		return watchlistLoadedMsg{items: items, err: err}
	}
}

func (m Model) addToWatchlistCmd(symbol string) tea.Cmd {
	return func() tea.Msg {
		err := m.watchlistStore.AddToWatchlist(context.Background(), symbol)
		return watchlistMutatedMsg{err: err}
	}
}

func (m Model) removeFromWatchlistCmd(symbol string) tea.Cmd {
	return func() tea.Msg {
		err := m.watchlistStore.RemoveFromWatchlist(context.Background(), symbol)
		return watchlistMutatedMsg{err: err}
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

// ── Table sync ───────────────────────────────────────────────────────────────

// syncRows rebuilds the table rows from the current holdings and watchlist
// slices. Holdings come first; watchlist entries follow with sleeve "[WL]".
func (m *Model) syncRows() {
	capacity := len(m.holdings) + len(m.watchlist)
	rows := make([]table.Row, 0, capacity)
	m.rows = make([]rowEntry, 0, capacity)

	for i, h := range m.holdings {
		m.rows = append(m.rows, rowEntry{kind: rowKindHolding, holdingIdx: i})
		rows = append(rows, m.holdingRow(h))
	}

	for i, item := range m.watchlist {
		m.rows = append(m.rows, rowEntry{kind: rowKindWatchlist, watchlistIdx: i})

		priceStr := "--"
		if item.MarketPrice > 0 {
			priceStr = fmt.Sprintf("%.2f", item.MarketPrice)
		}

		ccyStr := item.Currency
		if ccyStr == "" {
			ccyStr = "---"
		}

		rows = append(rows, table.Row{
			"[WL]",   // Sleeve — no active position
			"[TAA]",  // always TAA-eligible (that's the point of the watchlist)
			item.Symbol,
			ccyStr,   // ISO 4217 currency populated by the price feed on first refresh
			"--",     // Qty
			"--",     // PMC
			priceStr, // Price — updated by the price feed on every refresh cycle
			"--",     // P&L%
			"--",     // NAV
			"--",     // Dividends
		})
	}

	m.table.SetRows(rows)
}

func (m Model) holdingRow(h portfolio.Holding) table.Row {
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
		sleeveBadge := "[SAT]"
		if h.AllocationType == portfolio.AllocationCore {
			sleeveBadge = "[CORE]"
		}
		return table.Row{
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
		}
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

	return table.Row{
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
	}
}

// recomputeHeights applies the current terminal dimensions to the holdings table.
// Called on every WindowSizeMsg and after watchlist mutations.
func (m *Model) recomputeHeights() {
	if m.height == 0 {
		return
	}
	m.table.SetWidth(max(40, m.width-4))
	// Fixed chrome in the holdings body: summary(1) + table-header(1) = 2 rows.
	m.table.SetHeight(max(3, m.height-2))
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

// ── Watchlist input sub-model ────────────────────────────────────────────────

type watchlistInputResultMsg struct {
	symbol    string
	cancelled bool
}

type watchlistInputModel struct {
	input            textinput.Model
	autocompleteHint string
	knownSymbols     []string
	errMsg           string
	styles           formStyles
}

func newWatchlistInput(knownSymbols []string) (watchlistInputModel, tea.Cmd) {
	inp := textinput.New()
	inp.Placeholder = "e.g. MSFT"
	inp.CharLimit = 24
	inp.Width = 22

	m := watchlistInputModel{
		input:        inp,
		knownSymbols: knownSymbols,
		styles:       newFormStyles(),
	}
	cmd := m.input.Focus()
	return m, cmd
}

func (m watchlistInputModel) withUpdatedAutocomplete() watchlistInputModel {
	prefix := strings.ToUpper(strings.TrimSpace(m.input.Value()))
	m.autocompleteHint = ""
	if prefix == "" || len(m.knownSymbols) == 0 {
		return m
	}
	for _, sym := range m.knownSymbols {
		if strings.HasPrefix(sym, prefix) && sym != prefix {
			m.autocompleteHint = sym
			return m
		}
	}
	return m
}

func (m watchlistInputModel) Update(msg tea.Msg) (watchlistInputModel, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case "esc":
			if m.autocompleteHint != "" {
				m.autocompleteHint = ""
				return m, nil
			}
			return m, func() tea.Msg { return watchlistInputResultMsg{cancelled: true} }

		case "enter":
			if m.autocompleteHint != "" {
				m.input.SetValue(m.autocompleteHint)
				m.autocompleteHint = ""
				return m, nil
			}
			symbol := strings.ToUpper(strings.TrimSpace(m.input.Value()))
			if symbol == "" {
				m.errMsg = "symbol is required"
				return m, nil
			}
			return m, func() tea.Msg { return watchlistInputResultMsg{symbol: symbol} }

		case "tab", "down":
			if m.autocompleteHint != "" {
				m.input.SetValue(m.autocompleteHint)
				m.autocompleteHint = ""
				return m, nil
			}
		}
	}

	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	m = m.withUpdatedAutocomplete()
	return m, cmd
}

func (m watchlistInputModel) View() string {
	st := m.styles
	divider := st.divider.Render(strings.Repeat("─", 44))

	lines := []string{
		st.title.Render("Add to Watchlist"),
		divider,
		st.label.Render("Symbol:       ") + m.input.View(),
	}
	if m.autocompleteHint != "" {
		lines = append(lines, st.hint.Render("  → "+m.autocompleteHint+" (↓/tab: accept)"))
	}
	if m.errMsg != "" {
		lines = append(lines, st.errorStyle.Render("  Error: "+m.errMsg))
	}
	lines = append(lines, "", st.hint.Render("  enter: confirm · esc: cancel"))

	return lipgloss.JoinVertical(lipgloss.Left, lines...)
}
