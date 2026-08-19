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

const (
	colSleeveWidth    = 12
	colTAAWidth       = 5
	colSymbolWidth    = 12
	colCcyWidth       = 5
	colQtyWidth       = 10
	colPMCWidth       = 10
	colPriceWidth     = 10
	colPnLWidth       = 9
	colNAVWidth       = 12
	colDividendsWidth = 10
	tableInitHeight   = 10
	tableMinWidth     = 40
	tableMinHeight    = 3
	tableWidthOffset  = 4
	tableHeightOffset = 2
	dividerLen        = 44
	pnlPctMultiplier  = 100.0
)

const (
	keyEnter = "enter"
	keyDown  = "down"
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
//
//nolint:recvcheck // tea.Model interface requires value receivers; syncRows/recomputeHeights use pointer
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

// Init starts the holdings ticker, initial refresh, and watchlist refresh.
func (m Model) Init() tea.Cmd {
	return tea.Batch(tickCmd(), m.refreshCmd(), m.watchlistRefreshCmd())
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

// Update handles incoming messages for the holdings model.
//
//nolint:gocognit,cyclop,gocyclo,funlen,maintidx // holdings model dispatch; complexity is inherent in multi-mode UI
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

// View renders the holdings model.
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

//nolint:cyclop,funlen // refreshCmd aggregates multiple data sources; splitting would scatter related logic
func (m Model) refreshCmd() tea.Cmd {
	if m.store == nil {
		return func() tea.Msg {
			return holdingsMsg{
				err:               nil,
				holdings:          nil,
				summary: portfolio.Summary{
					TotalNAV: 0, CoreNAV: 0, SatelliteNAV: 0, CorePercent: 0,
					SatellitePercent: 0, TotalHoldingsRows: 0,
				},
				unrealizedPnL:     0,
				realized:          0,
				dividendsBySymbol: nil,
			}
		}
	}

	return func() tea.Msg {
		holdings, err := m.store.ListHoldings(context.Background())
		if err != nil {
			return holdingsMsg{
				err:      err,
				holdings: nil,
				summary: portfolio.Summary{
					TotalNAV: 0, CoreNAV: 0, SatelliteNAV: 0, CorePercent: 0,
					SatellitePercent: 0, TotalHoldingsRows: 0,
				},
				unrealizedPnL:     0,
				realized:          0,
				dividendsBySymbol: nil,
			}
		}

		unrealizedPnL := 0.0

		for _, holding := range holdings {
			if holding.Quantity > 0 && holding.PMC > 0 {
				unrealizedPnL += holding.Quantity * (holding.MarketPrice - holding.PMC)
			}
		}

		realizedPnL := 0.0

		total, err := m.store.TotalRealizedPnL(context.Background())
		if err == nil {
			realizedPnL = total
		}

		type dividendReader interface {
			ListDividendIncome(ctx context.Context) ([]portfolio.DividendRecord, error)
		}

		dividendsBySymbol := make(map[string]float64)

		if dr, ok := any(m.txStore).(dividendReader); ok && m.txStore != nil {
			records, err := dr.ListDividendIncome(context.Background())
			if err == nil {
				for _, record := range records {
					dividendsBySymbol[record.Symbol] += record.IncomeAmount
				}
			}
		}

		return holdingsMsg{
			holdings:          holdings,
			summary:           portfolio.BuildSummary(holdings),
			unrealizedPnL:     unrealizedPnL,
			realized:          realizedPnL,
			dividendsBySymbol: dividendsBySymbol,
			err:               nil,
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
		return func() tea.Msg { return toggledMsg{holdingID: holdingID, err: nil} }
	}

	return func() tea.Msg {
		err := m.store.ToggleAllocation(context.Background(), holdingID)

		return toggledMsg{holdingID: holdingID, err: err}
	}
}

func (m Model) taaToggleCmd(symbol string) tea.Cmd {
	return func() tea.Msg {
		if m.store == nil {
			return taaToggledMsg{symbol: symbol, err: nil}
		}

		err := m.store.ToggleTAAEnabled(context.Background(), symbol)

		return taaToggledMsg{symbol: symbol, err: err}
	}
}

func (m Model) addTransactionCmd(transaction portfolio.Transaction) tea.Cmd {
	if m.txStore == nil {
		return func() tea.Msg { return txSubmittedMsg{err: nil} }
	}

	return func() tea.Msg {
		err := m.txStore.AddTransaction(context.Background(), transaction)

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

	for idx, holding := range m.holdings {
		m.rows = append(m.rows, rowEntry{kind: rowKindHolding, holdingIdx: idx, watchlistIdx: 0})
		rows = append(rows, m.holdingRow(holding))
	}

	for idx, item := range m.watchlist {
		m.rows = append(m.rows, rowEntry{kind: rowKindWatchlist, watchlistIdx: idx, holdingIdx: 0})

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

//nolint:funlen // holdingRow formats many fields; all branches are necessary
func (m Model) holdingRow(holding portfolio.Holding) table.Row {
	taaStr := "[TAA]"
	if !holding.TAAEnabled {
		taaStr = "[ - ]"
	}

	divIncome := m.dividendsBySymbol[holding.Symbol]

	divStr := "--"
	if divIncome != 0 {
		divStr = fmt.Sprintf("%+.2f", divIncome)
	}

	ccyStr := holding.Currency
	if ccyStr == "" {
		ccyStr = "---"
	}

	if holding.Quantity <= 0 {
		sleeveBadge := "[SAT]"
		if holding.AllocationType == portfolio.AllocationCore {
			sleeveBadge = "[CORE]"
		}

		return table.Row{
			sleeveBadge,
			taaStr,
			holding.Symbol,
			ccyStr,
			"0.0000",
			"--",
			fmt.Sprintf("%.2f", holding.MarketPrice),
			"--",
			"0.00",
			divStr,
		}
	}

	allocation := "[SAT]"
	if holding.AllocationType == portfolio.AllocationCore {
		allocation = "[CORE]"
	}

	pmcStr := "--"

	pnlStr := "--"

	if holding.PMC > 0 {
		pmcStr = fmt.Sprintf("%.2f", holding.PMC)
		pnlPct := (holding.MarketPrice - holding.PMC) / holding.PMC * pnlPctMultiplier
		pnlStr = fmt.Sprintf("%+.1f%%", pnlPct)
	}

	return table.Row{
		allocation,
		taaStr,
		holding.Symbol,
		ccyStr,
		fmt.Sprintf("%.4f", holding.Quantity),
		pmcStr,
		fmt.Sprintf("%.2f", holding.MarketPrice),
		pnlStr,
		fmt.Sprintf("%.2f", holding.NAV()),
		divStr,
	}
}

// recomputeHeights applies the current terminal dimensions to the holdings table.
// Called on every WindowSizeMsg and after watchlist mutations.
func (m *Model) recomputeHeights() {
	if m.height == 0 {
		return
	}

	m.table.SetWidth(max(tableMinWidth, m.width-tableWidthOffset))
	// Fixed chrome in the holdings body: summary(1) + table-header(1) = 2 rows.
	m.table.SetHeight(max(tableMinHeight, m.height-tableHeightOffset))
}

func (m Model) knownSymbols() []string {
	syms := make([]string, 0, len(m.holdings))
	for _, holding := range m.holdings {
		syms = append(syms, holding.Symbol)
	}

	return syms
}

func (m Model) summaryView() string {
	styles := m.styles

	ccySuffix := ""
	if m.baseCurrency != "" {
		ccySuffix = " " + m.baseCurrency
	}

	return styles.summary.Render(fmt.Sprintf(
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
	return tea.Tick(refreshInterval, func(tick time.Time) tea.Msg { return tickMsg(tick) })
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

	watchlistModel := watchlistInputModel{
		input:            inp,
		autocompleteHint: "",
		knownSymbols:     knownSymbols,
		errMsg:           "",
		styles:           newFormStyles(),
	}
	cmd := watchlistModel.input.Focus()

	return watchlistModel, cmd
}

func (m watchlistInputModel) Update(msg tea.Msg) (watchlistInputModel, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case "esc":
			if m.autocompleteHint != "" {
				m.autocompleteHint = ""

				return m, nil
			}

			return m, func() tea.Msg { return watchlistInputResultMsg{cancelled: true, symbol: ""} }

		case keyEnter:
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

			return m, func() tea.Msg { return watchlistInputResultMsg{symbol: symbol, cancelled: false} }

		case "tab", keyDown:
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
	styles := m.styles
	div := styles.divider.Render(strings.Repeat("─", dividerLen))

	lines := []string{
		styles.title.Render("Add to Watchlist"),
		div,
		styles.label.Render("Symbol:       ") + m.input.View(),
	}

	if m.autocompleteHint != "" {
		lines = append(lines, styles.hint.Render("  → "+m.autocompleteHint+" (↓/tab: accept)"))
	}

	if m.errMsg != "" {
		lines = append(lines, styles.errorStyle.Render("  Error: "+m.errMsg))
	}

	lines = append(lines, "", styles.hint.Render("  enter: confirm · esc: cancel"))

	return lipgloss.JoinVertical(lipgloss.Left, lines...)
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

//nolint:funlen // newModelCore initialises many struct fields; extraction adds no clarity
func newModelCore(store portfolio.HoldingsStore, txStore portfolio.TransactionStore, portfolioID string) Model {
	if portfolioID == "" {
		portfolioID = defaultPortfolioID
	}

	cols := []table.Column{
		{Title: "Sleeve", Width: colSleeveWidth},
		{Title: "TAA", Width: colTAAWidth},
		{Title: "Symbol", Width: colSymbolWidth},
		{Title: "Ccy", Width: colCcyWidth},
		{Title: "Qty", Width: colQtyWidth},
		{Title: "PMC", Width: colPMCWidth},
		{Title: "Price", Width: colPriceWidth},
		{Title: "P&L%", Width: colPnLWidth},
		{Title: "NAV", Width: colNAVWidth},
		{Title: "Dividends", Width: colDividendsWidth},
	}

	tbl := table.New(
		table.WithColumns(cols),
		table.WithRows([]table.Row{}),
		table.WithFocused(true),
		table.WithHeight(tableInitHeight),
	)
	tbl.KeyMap.LineUp.SetKeys("up", "k")
	tbl.KeyMap.LineDown.SetKeys("down", "j")

	return Model{
		store:             store,
		txStore:           txStore,
		portfolioID:       portfolioID,
		baseCurrency:      "",
		table:             tbl,
		holdings:          nil,
		rows:              nil,
		summary: portfolio.Summary{
			TotalNAV: 0, CoreNAV: 0, SatelliteNAV: 0, CorePercent: 0,
			SatellitePercent: 0, TotalHoldingsRows: 0,
		},
		unrealizedPnL:     0,
		realized:          0,
		dividendsBySymbol: nil,
		status:            "Loading holdings...",
		loading:           true,
		loadError:         nil,
		quit:              false,
		mode:              modeNormal,
		form: transactionFormModel{
			symbolInput:      textinput.Model{},
			qtyInput:         textinput.Model{},
			priceInput:       textinput.Model{},
			feeInput:         textinput.Model{},
			dateInput:        textinput.Model{},
			txType:           "",
			allocType:        "",
			focused:          0,
			errMsg:           "",
			knownSymbols:     nil,
			autocompleteHint: "",
			txID:             0,
			formStyles: formStyles{}, //nolint:exhaustruct // zero-value placeholder; replaced by newFormStyles() on open
		},
		watchlistStore: nil,
		watchlist:      nil,
		watchlistInput: watchlistInputModel{
			input:            textinput.Model{},
			autocompleteHint: "",
			knownSymbols:     nil,
			errMsg:           "",
			styles: formStyles{}, //nolint:exhaustruct // zero-value placeholder; replaced by newFormStyles() on open
		},
		width:             0,
		height:            0,
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
	}
}
