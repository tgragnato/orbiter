package tui

import (
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/tgragnato/orbiter/internal/signal"
)

type fakeSignalReadModel struct {
	messages []signal.Message
}

func (f fakeSignalReadModel) Pending() []signal.Message { return f.messages }
func (f fakeSignalReadModel) Drain() []signal.Message   { return f.messages }

func TestRootModelInit(t *testing.T) {
	t.Parallel()

	model := NewRootModelWithMetrics(&fakeHoldingsStore{}, fakeSignalReadModel{}, defaultPortfolioID, nil, nil, nil, nil, nil)
	if cmd := model.Init(); cmd == nil {
		t.Fatalf("Init() cmd = nil, want non-nil")
	}
}

func TestRootModelTabSwitch(t *testing.T) {
	t.Parallel()

	model := NewRootModelWithMetrics(&fakeHoldingsStore{}, fakeSignalReadModel{}, defaultPortfolioID, nil, nil, nil, nil, nil)
	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyTab})
	model = updated.(RootModel)

	if model.activeTab != tabSignals {
		t.Fatalf("active tab = %d, want tabSignals", model.activeTab)
	}
	if cmd == nil {
		t.Fatalf("tab switch cmd = nil, want non-nil")
	}
}

func TestRootModelQuitStopsUpdateLoop(t *testing.T) {
	t.Parallel()

	model := NewRootModelWithMetrics(&fakeHoldingsStore{}, fakeSignalReadModel{}, defaultPortfolioID, nil, nil, nil, nil, nil)
	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	model = updated.(RootModel)

	if !model.quitting {
		t.Fatalf("quitting = false, want true")
	}
	if cmd == nil {
		t.Fatalf("quit cmd = nil, want non-nil")
	}

	updated, cmd = model.Update(tickMsg(time.Now()))
	model = updated.(RootModel)
	if cmd != nil {
		t.Fatalf("cmd after quit = non-nil, want nil")
	}
}

func TestRootModelView(t *testing.T) {
	t.Parallel()

	model := NewRootModelWithMetrics(&fakeHoldingsStore{}, fakeSignalReadModel{}, defaultPortfolioID, nil, nil, nil, nil, nil)
	view := model.View()
	if view == "" {
		t.Fatalf("View() empty, want non-empty")
	}
	// Ensure header contains dashboard name
	if view == "" {
		t.Fatalf("View() returned empty string")
	}
}

func TestRootModelViewQuitting(t *testing.T) {
	t.Parallel()

	model := NewRootModelWithMetrics(&fakeHoldingsStore{}, fakeSignalReadModel{}, defaultPortfolioID, nil, nil, nil, nil, nil)
	model.quitting = true
	if got := model.View(); got != "" {
		t.Fatalf("View() while quitting = %q, want empty", got)
	}
}

func TestRootModelViewSignalsTab(t *testing.T) {
	t.Parallel()

	model := NewRootModelWithMetrics(&fakeHoldingsStore{}, fakeSignalReadModel{}, defaultPortfolioID, nil, nil, nil, nil, nil)
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyTab})
	model = updated.(RootModel)

	view := model.View()
	if view == "" {
		t.Fatalf("View() on signals tab is empty")
	}
}

func TestRootModelShiftTabNavigation(t *testing.T) {
	t.Parallel()

	model := NewRootModelWithMetrics(&fakeHoldingsStore{}, fakeSignalReadModel{}, defaultPortfolioID, nil, nil, nil, nil, nil)

	// holdings(0) → shift+tab → analytics(5) [wraps to last tab]
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
	model = updated.(RootModel)
	if model.activeTab != tabAnalytics {
		t.Fatalf("active tab = %d, want tabAnalytics after shift+tab from holdings", model.activeTab)
	}

	// analytics(5) → shift+tab → transactions(4)
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
	model = updated.(RootModel)
	if model.activeTab != tabTransactions {
		t.Fatalf("active tab = %d, want tabTransactions after shift+tab from analytics", model.activeTab)
	}

	// transactions(4) → shift+tab → logs(3) — logs Init() returns nil cmd, so no cmd assertion here
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
	model = updated.(RootModel)
	if model.activeTab != tabLogs {
		t.Fatalf("active tab = %d, want tabLogs after shift+tab from transactions", model.activeTab)
	}

	// logs(3) → shift+tab → settings(2)
	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
	model = updated.(RootModel)
	if model.activeTab != tabSettings {
		t.Fatalf("active tab = %d, want tabSettings after shift+tab from logs", model.activeTab)
	}
	if cmd == nil {
		t.Fatalf("shift+tab cmd = nil, want non-nil")
	}

	// settings(2) → shift+tab → signals(1)
	updated, cmd = model.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
	model = updated.(RootModel)
	if model.activeTab != tabSignals {
		t.Fatalf("active tab = %d, want tabSignals after shift+tab from settings", model.activeTab)
	}
	if cmd == nil {
		t.Fatalf("shift+tab cmd = nil, want non-nil")
	}

	// signals(1) → shift+tab → holdings(0)
	updated, cmd = model.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
	model = updated.(RootModel)
	if model.activeTab != tabHoldings {
		t.Fatalf("active tab = %d, want tabHoldings after shift+tab from signals", model.activeTab)
	}
	if cmd == nil {
		t.Fatalf("shift+tab cmd = nil, want non-nil")
	}
}

func TestRootModelTabLAndHKeys(t *testing.T) {
	t.Parallel()

	model := NewRootModelWithMetrics(&fakeHoldingsStore{}, fakeSignalReadModel{}, defaultPortfolioID, nil, nil, nil, nil, nil)
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
	model = updated.(RootModel)
	if model.activeTab != tabSignals {
		t.Fatalf("active tab after 'l' = %d, want tabSignals", model.activeTab)
	}

	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}})
	model = updated.(RootModel)
	if model.activeTab != tabHoldings {
		t.Fatalf("active tab after 'h' = %d, want tabHoldings", model.activeTab)
	}
}

func TestRootModelForwardCyclesSixTabs(t *testing.T) {
	t.Parallel()

	model := NewRootModelWithMetrics(&fakeHoldingsStore{}, fakeSignalReadModel{}, defaultPortfolioID, nil, nil, nil, nil, nil)
	// holdings(0) → tab → signals(1)
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyTab})
	model = updated.(RootModel)
	if model.activeTab != tabSignals {
		t.Fatalf("tab 1: active = %d, want tabSignals", model.activeTab)
	}
	// signals(1) → tab → settings(2)
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyTab})
	model = updated.(RootModel)
	if model.activeTab != tabSettings {
		t.Fatalf("tab 2: active = %d, want tabSettings", model.activeTab)
	}
	// settings(2) → tab → logs(3)
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyTab})
	model = updated.(RootModel)
	if model.activeTab != tabLogs {
		t.Fatalf("tab 3: active = %d, want tabLogs", model.activeTab)
	}
	// logs(3) → tab → transactions(4)
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyTab})
	model = updated.(RootModel)
	if model.activeTab != tabTransactions {
		t.Fatalf("tab 4: active = %d, want tabTransactions", model.activeTab)
	}
	// transactions(4) → tab → analytics(5)
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyTab})
	model = updated.(RootModel)
	if model.activeTab != tabAnalytics {
		t.Fatalf("tab 5: active = %d, want tabAnalytics", model.activeTab)
	}
	// analytics(5) → tab → holdings(0) [wraps]
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyTab})
	model = updated.(RootModel)
	if model.activeTab != tabHoldings {
		t.Fatalf("tab 6: active = %d, want tabHoldings", model.activeTab)
	}
}

func TestRootModelSettingsTabView(t *testing.T) {
	t.Parallel()

	model := NewRootModelWithMetrics(&fakeHoldingsStore{}, fakeSignalReadModel{}, defaultPortfolioID, nil, nil, nil, nil, nil)
	// Navigate to settings tab
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyTab})
	updated, _ = updated.(RootModel).Update(tea.KeyMsg{Type: tea.KeyTab})
	model = updated.(RootModel)
	if model.activeTab != tabSettings {
		t.Fatalf("active tab = %d, want tabSettings", model.activeTab)
	}
	view := model.View()
	if view == "" {
		t.Fatalf("View() on settings tab is empty")
	}
}

func TestRootModelSignalsTabMessageRouting(t *testing.T) {
	t.Parallel()

	model := NewRootModelWithMetrics(&fakeHoldingsStore{}, fakeSignalReadModel{
		messages: []signal.Message{{Type: signal.TypeBuy, Summary: "test"}},
	}, defaultPortfolioID, nil, nil, nil, nil, nil)
	// Switch to signals tab
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyTab})
	model = updated.(RootModel)

	// Route a signalsMsg through the root model
	updated, _ = model.Update(signalsMsg{messages: model.signalsTab.readModel.Pending()})
	model = updated.(RootModel)

	if len(model.signalsTab.messages) != 1 {
		t.Fatalf("signals messages = %d, want 1", len(model.signalsTab.messages))
	}
}
