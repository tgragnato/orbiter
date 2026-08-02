package tui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/tgragnato/orbiter/internal/portfolio"
	innersignal "github.com/tgragnato/orbiter/internal/signal"
)

const (
	tabHoldings = iota
	tabSignals
	tabSettings
	tabLogs
	tabTransactions
	tabCount = 5
)

// RootModel hosts Tab 1 (holdings), Tab 2 (signals), Tab 3 (settings), Tab 4 (logs), and Tab 5 (transactions).
type RootModel struct {
	holdingsTab     Model
	signalsTab      SignalsTabModel
	settingsTab     SettingsTabModel
	logsTab         LogsTabModel
	transactionsTab TransactionsTabModel
	activeTab       int
	quitting        bool

	tabStyle stylesRoot
}

type stylesRoot struct {
	activeTab   lipgloss.Style
	inactiveTab lipgloss.Style
	help        lipgloss.Style
}

// NewRootModel builds the root TUI model without analytics or settings.
func NewRootModel(store portfolio.HoldingsStore, readModel innersignal.ReadModel) RootModel {
	return NewRootModelWithMetrics(store, readModel, defaultPortfolioID, nil, nil, nil, nil)
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
) RootModel {
	// Upgrade txStore to a TransactionEditor if the underlying implementation supports it.
	var txEditor TransactionEditor
	if te, ok := txStore.(TransactionEditor); ok {
		txEditor = te
	}
	return RootModel{
		holdingsTab:     NewModelWithAll(store, txStore, portfolioID),
		signalsTab:      NewSignalsTabModelWithML(readModel, ml),
		settingsTab:     NewSettingsTabModel(configSvc),
		logsTab:         NewLogsTabModel(logCh),
		transactionsTab: NewTransactionsTabModel(txEditor),
		activeTab:       tabHoldings,
		tabStyle: stylesRoot{
			activeTab:   lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("33")),
			inactiveTab: lipgloss.NewStyle().Foreground(lipgloss.Color("245")),
			help:        lipgloss.NewStyle().Foreground(lipgloss.Color("245")),
		},
	}
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
			if m.activeTab == tabHoldings && m.holdingsTab.mode == modeAddTx {
				break
			}
			if m.activeTab == tabTransactions &&
				(m.transactionsTab.mode == txModeAdding || m.transactionsTab.mode == txModeEditing) {
				break
			}
			m.activeTab = (m.activeTab + 1) % tabCount
			return m, m.initActiveTab()
		case "shift+tab", "h":
			if m.activeTab == tabHoldings && m.holdingsTab.mode == modeAddTx {
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

	// Transactions modified in Tab 5 → refresh the holdings tab so PMC/qty stay current.
	if _, ok := msg.(txChangedMsg); ok {
		m.holdingsTab.loading = true
		return m, m.holdingsTab.refreshCmd()
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
	header := tabParts[0] + "  " + tabParts[1] + "  " + tabParts[2] + "  " + tabParts[3] + "  " + tabParts[4]
	help := m.tabStyle.help.Render("tab/l: next · shift+tab/h: prev · q: quit")

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
	default:
		body = m.holdingsTab.View()
	}

	return lipgloss.JoinVertical(lipgloss.Left, header, help, body)
}
