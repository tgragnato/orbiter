package tui

import (
	"context"
	"fmt"
	"log/slog"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/tgragnato/orbiter/internal/portfolio"
	"github.com/tgragnato/orbiter/internal/portfolio/analytics"
	"github.com/tgragnato/orbiter/internal/portfolio/data"
	"github.com/tgragnato/orbiter/internal/portfolio/feed"
	innersignal "github.com/tgragnato/orbiter/internal/signal"
	"github.com/tgragnato/orbiter/internal/signal/taa"
)

const (
	tabHoldings = iota
	tabSignals
	tabSettings
	tabLogs
	tabTransactions
	tabAnalytics
	tabCount = 6
)

// RootModel hosts Tab 1 (holdings), Tab 2 (signals), Tab 3 (settings), Tab 4 (logs), and Tab 5 (transactions).
type RootModel struct {
	holdingsTab     Model
	signalsTab      SignalsTabModel
	settingsTab     SettingsTabModel
	logsTab         LogsTabModel
	transactionsTab TransactionsTabModel
	analyticsTab    AnalyticsTabModel
	activeTab       int
	quitting        bool
	width           int
	height          int

	tabStyle stylesRoot

	// Hot-reload fields — populated via WithYahooProvider / WithUpdater / WithTAAEngine after construction.
	// All are optional; nil means the feature is disabled for that axis.
	yahooProvider *data.YahooProvider  // receives SetAPIKey on settings save
	updater       *feed.Updater        // receives SetBaseCurrency + TriggerBackfill on settings save
	twrEngine     *analytics.TWREngine // used to clear snapshots before the backfill
	taaEngine     *taa.Engine          // receives SetConfig on settings save
	portfolioID   string               // identifies which portfolio's snapshots to clear
}

type stylesRoot struct {
	activeTab   lipgloss.Style
	inactiveTab lipgloss.Style
	help        lipgloss.Style
}

// NewRootModelWithMetrics builds the root model with optional configuration and log channel.
func NewRootModelWithMetrics(
	store portfolio.HoldingsStore,
	readModel innersignal.ReadModel,
	portfolioID string,
	ml MLEngine,
	txStore portfolio.TransactionStore,
	configSvc SettingsService,
	logCh LogChannel,
	twrEngine *analytics.TWREngine,
	baseCurrency string,
) RootModel {
	// Upgrade txStore to a TransactionEditor if the underlying implementation supports it.
	var txEditor TransactionEditor
	if te, ok := txStore.(TransactionEditor); ok {
		txEditor = te
	}

	holdingsTab := NewModelWithAll(store, txStore, portfolioID).WithBaseCurrency(baseCurrency)
	// Enable the watchlist section if the store implements WatchlistStore.
	if ws, ok := store.(portfolio.WatchlistStore); ok {
		holdingsTab = holdingsTab.WithWatchlist(ws)
	}

	return RootModel{
		holdingsTab:     holdingsTab,
		signalsTab:      NewSignalsTabModelWithML(readModel, ml),
		settingsTab:     NewSettingsTabModel(configSvc),
		logsTab:         NewLogsTabModel(logCh),
		transactionsTab: NewTransactionsTabModel(txEditor),
		analyticsTab:    NewAnalyticsTabModel(twrEngine, portfolioID),
		activeTab:       tabHoldings,
		tabStyle: stylesRoot{
			activeTab:   lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("33")),
			inactiveTab: lipgloss.NewStyle().Foreground(lipgloss.Color("245")),
			help:        lipgloss.NewStyle().Foreground(lipgloss.Color("245")),
		},
		twrEngine:   twrEngine,
		portfolioID: portfolioID,
	}
}

// WithYahooProvider enables hot-reloading of the Yahoo API key from the Settings tab.
// Call once after NewRootModelWithMetrics before handing the model to tea.NewProgram.
func (m RootModel) WithYahooProvider(p *data.YahooProvider) RootModel {
	m.yahooProvider = p
	return m
}

// WithUpdater enables hot-reloading of the base currency from the Settings tab.
// Call once after NewRootModelWithMetrics before handing the model to tea.NewProgram.
func (m RootModel) WithUpdater(u *feed.Updater) RootModel {
	m.updater = u
	return m
}

// WithTAAEngine enables hot-reloading of the TAA broker friction parameters
// from the Settings tab. Call once after NewRootModelWithMetrics.
func (m RootModel) WithTAAEngine(e *taa.Engine) RootModel {
	m.taaEngine = e
	return m
}

func (m RootModel) Init() tea.Cmd {
	return tea.Batch(m.holdingsTab.Init(), m.logsTab.Init())
}

func (m RootModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if m.quitting {
		return m, nil
	}

	// Log entries are always routed to the logs tab regardless of which tab is active.
	if entry, ok := msg.(LogEntry); ok {
		updated, cmd := m.logsTab.Update(entry)
		if next, ok := updated.(LogsTabModel); ok {
			m.logsTab = next
		}
		return m, cmd
	}

	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case "ctrl+c", "q":
			m.quitting = true
			m.holdingsTab.quit = true
			m.signalsTab.quit = true
			m.settingsTab.quit = true
			m.logsTab.quit = true
			return m, tea.Quit
		case "tab", "l":
			if m.activeTab == tabHoldings &&
				(m.holdingsTab.mode == modeAddTx || m.holdingsTab.mode == modeAddWatchlist) {
				break
			}
			if m.activeTab == tabTransactions &&
				(m.transactionsTab.mode == txModeAdding || m.transactionsTab.mode == txModeEditing) {
				break
			}
			m.activeTab = (m.activeTab + 1) % tabCount
			return m, m.initActiveTab()
		case "shift+tab", "h":
			if m.activeTab == tabHoldings &&
				(m.holdingsTab.mode == modeAddTx || m.holdingsTab.mode == modeAddWatchlist) {
				break
			}
			if m.activeTab == tabTransactions &&
				(m.transactionsTab.mode == txModeAdding || m.transactionsTab.mode == txModeEditing) {
				break
			}
			m.activeTab = (m.activeTab + tabCount - 1) % tabCount
			return m, m.initActiveTab()
		}
	}

	// settingsSavedMsg must reach both the settings tab (UI status) and root-level
	// hot-reload logic, so we intercept it here rather than routing to the active tab.
	if saved, ok := msg.(settingsSavedMsg); ok {
		// Always let the settings tab update its status line.
		updated, settingsCmd := m.settingsTab.Update(saved)
		if next, ok := updated.(SettingsTabModel); ok {
			m.settingsTab = next
		}

		if saved.err != nil {
			return m, settingsCmd
		}

		var cmds []tea.Cmd
		if settingsCmd != nil {
			cmds = append(cmds, settingsCmd)
		}

		// Hot-reload: propagate the new API key into the live Yahoo provider.
		if m.yahooProvider != nil {
			m.yahooProvider.SetAPIKey(saved.apiKey)
		}

		// Hot-reload: propagate the new base currency into the price-feed updater.
		if m.updater != nil && saved.currency != "" {
			m.updater.SetBaseCurrency(saved.currency)
		}

		// Reflect the new base currency in the holdings tab (currency suffix in summary bar).
		if saved.currency != "" {
			m.holdingsTab = m.holdingsTab.WithBaseCurrency(saved.currency)
		}

		// Hot-reload TAA broker friction parameters and base currency into the TAA engine.
		if m.taaEngine != nil {
			existingCfg := m.taaEngine.GetConfig()
			newCfg := taa.Config{
				TaxRate:          saved.brokerConfig.TaxRate,
				BrokerFeePercent: saved.brokerConfig.BrokerFeePercent,
				MaxBrokerFee:     saved.brokerConfig.MaxBrokerFee,
				Buffer:           saved.brokerConfig.Buffer,
				Currency:         saved.currency,
			}
			// If currency wasn't changed (empty string can't happen here since it's validated,
			// but defensive: keep existing), preserve the current Currency.
			if newCfg.Currency == "" {
				newCfg.Currency = existingCfg.Currency
			}
			m.taaEngine.SetConfig(newCfg)
		}

		// Recreate the analytics tab so it starts fresh with the new currency context.
		m.analyticsTab = NewAnalyticsTabModel(m.analyticsTab.engine, m.analyticsTab.portfolioID)
		cmds = append(cmds, m.analyticsTab.Init())

		// Asynchronously clear old NAV snapshots and rebuild them in the new currency.
		// The backfill may take a while; it logs progress via slog and is idempotent.
		if m.twrEngine != nil && m.portfolioID != "" {
			twrEngine := m.twrEngine
			portfolioID := m.portfolioID
			updater := m.updater
			cmds = append(cmds, func() tea.Msg {
				ctx := context.Background()
				if err := twrEngine.ClearSnapshots(ctx, portfolioID); err != nil {
					slog.Warn("hot-reload: clear snapshots failed", "error", err)
				}
				if updater != nil {
					updater.TriggerBackfill(ctx)
				}
				return nil
			})
		}

		return m, tea.Batch(cmds...)
	}

	// Transactions modified in Tab 5 → refresh the holdings tab so PMC/qty stay current.
	if _, ok := msg.(txChangedMsg); ok {
		m.holdingsTab.loading = true
		return m, m.holdingsTab.refreshCmd()
	}

	// WindowSizeMsg must reach every tab, not just the active one. Off-screen tabs
	// would otherwise keep stale (or zero) dimensions and appear broken when the
	// user navigates to them. The chrome (tab bar + help line) occupies 2 rows;
	// the remaining height is forwarded as the content area to each sub-model.
	if size, ok := msg.(tea.WindowSizeMsg); ok {
		m.width = size.Width
		m.height = size.Height
		const chromeLines = 2
		contentH := max(size.Height-chromeLines, 0)
		tabMsg := tea.WindowSizeMsg{Width: size.Width, Height: contentH}
		var cmds []tea.Cmd
		var cmd tea.Cmd
		var updated tea.Model

		updated, cmd = m.holdingsTab.Update(tabMsg)
		if h, ok := updated.(Model); ok {
			m.holdingsTab = h
		}
		cmds = append(cmds, cmd)

		updated, cmd = m.signalsTab.Update(tabMsg)
		if h, ok := updated.(SignalsTabModel); ok {
			m.signalsTab = h
		}
		cmds = append(cmds, cmd)

		updated, cmd = m.settingsTab.Update(tabMsg)
		if h, ok := updated.(SettingsTabModel); ok {
			m.settingsTab = h
		}
		cmds = append(cmds, cmd)

		updated, cmd = m.logsTab.Update(tabMsg)
		if h, ok := updated.(LogsTabModel); ok {
			m.logsTab = h
		}
		cmds = append(cmds, cmd)

		updated, cmd = m.transactionsTab.Update(tabMsg)
		if h, ok := updated.(TransactionsTabModel); ok {
			m.transactionsTab = h
		}
		cmds = append(cmds, cmd)

		updated, cmd = m.analyticsTab.Update(tabMsg)
		if h, ok := updated.(AnalyticsTabModel); ok {
			m.analyticsTab = h
		}
		cmds = append(cmds, cmd)

		return m, tea.Batch(cmds...)
	}

	switch m.activeTab {
	case tabHoldings:
		updated, cmd := m.holdingsTab.Update(msg)
		if next, ok := updated.(Model); ok {
			m.holdingsTab = next
		}
		return m, cmd
	case tabSignals:
		updated, cmd := m.signalsTab.Update(msg)
		if next, ok := updated.(SignalsTabModel); ok {
			m.signalsTab = next
		}
		return m, cmd
	case tabSettings:
		updated, cmd := m.settingsTab.Update(msg)
		if next, ok := updated.(SettingsTabModel); ok {
			m.settingsTab = next
		}
		return m, cmd
	case tabLogs:
		updated, cmd := m.logsTab.Update(msg)
		if next, ok := updated.(LogsTabModel); ok {
			m.logsTab = next
		}
		return m, cmd
	case tabTransactions:
		updated, cmd := m.transactionsTab.Update(msg)
		if next, ok := updated.(TransactionsTabModel); ok {
			m.transactionsTab = next
		}
		return m, cmd
	case tabAnalytics:
		updated, cmd := m.analyticsTab.Update(msg)
		if next, ok := updated.(AnalyticsTabModel); ok {
			m.analyticsTab = next
		}
		return m, cmd
	}
	return m, nil
}

func (m RootModel) initActiveTab() tea.Cmd {
	switch m.activeTab {
	case tabHoldings:
		return m.holdingsTab.Init()
	case tabSignals:
		return m.signalsTab.Init()
	case tabSettings:
		return m.settingsTab.Init()
	case tabLogs:
		return nil
	case tabTransactions:
		return m.transactionsTab.Init()
	case tabAnalytics:
		return m.analyticsTab.Init()
	}
	return nil
}

func (m RootModel) View() string {
	if m.quitting {
		return ""
	}

	tabDefs := []struct {
		label string
		idx   int
	}{
		{"1: Holdings", tabHoldings},
		{"2: Signals", tabSignals},
		{"3: Settings", tabSettings},
		{"4: Logs", tabLogs},
		{"5: Transactions", tabTransactions},
		{"6: Analytics", tabAnalytics},
	}

	tabParts := make([]string, len(tabDefs))
	for i, td := range tabDefs {
		label := fmt.Sprintf("[%s]", td.label)
		if td.idx == m.activeTab {
			tabParts[i] = m.tabStyle.activeTab.Render(label)
		} else {
			tabParts[i] = m.tabStyle.inactiveTab.Render(label)
		}
	}
	header := tabParts[0] + "  " + tabParts[1] + "  " + tabParts[2] + "  " + tabParts[3] + "  " + tabParts[4] + "  " + tabParts[5]
	const globalNav = "tab/l: next · shift+tab/h: prev · q: quit"
	var tabHint string
	switch m.activeTab {
	case tabHoldings:
		tabHint = m.holdingsTab.NavHint()
	case tabSignals:
		tabHint = m.signalsTab.NavHint()
	case tabSettings:
		tabHint = m.settingsTab.NavHint()
	case tabLogs:
		tabHint = m.logsTab.NavHint()
	case tabTransactions:
		tabHint = m.transactionsTab.NavHint()
	case tabAnalytics:
		tabHint = m.analyticsTab.NavHint()
	}
	helpStr := globalNav
	if tabHint != "" {
		helpStr = globalNav + "  |  " + tabHint
	}
	help := m.tabStyle.help.Render(helpStr)

	var body string
	switch m.activeTab {
	case tabSignals:
		body = m.signalsTab.View()
	case tabSettings:
		body = m.settingsTab.View()
	case tabLogs:
		body = m.logsTab.View()
	case tabTransactions:
		body = m.transactionsTab.View()
	case tabAnalytics:
		body = m.analyticsTab.View()
	default:
		body = m.holdingsTab.View()
	}

	return lipgloss.JoinVertical(lipgloss.Left, header, help, body)
}
