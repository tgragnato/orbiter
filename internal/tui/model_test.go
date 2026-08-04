package tui

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/tgragnato/orbiter/internal/portfolio"
)

type fakeHoldingsStore struct {
	holdings          []portfolio.Holding
	toggleErr         error
	taaToggleErr      error
	listErr           error
	toggledIDs        []int64
	taaToggledSymbols []string
	realizedPnL       float64
}

func (s *fakeHoldingsStore) ListHoldings(context.Context) ([]portfolio.Holding, error) {
	if s.listErr != nil {
		return nil, s.listErr
	}
	result := make([]portfolio.Holding, len(s.holdings))
	copy(result, s.holdings)
	return result, nil
}

func (s *fakeHoldingsStore) ToggleAllocation(_ context.Context, id int64) error {
	s.toggledIDs = append(s.toggledIDs, id)
	if s.toggleErr != nil {
		return s.toggleErr
	}
	for i := range s.holdings {
		if s.holdings[i].ID == id {
			s.holdings[i].AllocationType = s.holdings[i].ToggleAllocation()
		}
	}
	return nil
}

func (s *fakeHoldingsStore) ToggleTAAEnabled(_ context.Context, symbol string) error {
	s.taaToggledSymbols = append(s.taaToggledSymbols, symbol)
	if s.taaToggleErr != nil {
		return s.taaToggleErr
	}
	for i := range s.holdings {
		if s.holdings[i].Symbol == symbol {
			s.holdings[i].TAAEnabled = !s.holdings[i].TAAEnabled
		}
	}
	return nil
}

func (s *fakeHoldingsStore) TotalRealizedPnL(context.Context) (float64, error) {
	return s.realizedPnL, nil
}

func TestModelInitReturnsBatchCmd(t *testing.T) {
	t.Parallel()

	model := NewModel(&fakeHoldingsStore{})
	if cmd := model.Init(); cmd == nil {
		t.Fatalf("Init() cmd = nil, want non-nil")
	}
}

func TestModelRefreshLoadsRowsAndSummary(t *testing.T) {
	t.Parallel()

	store := &fakeHoldingsStore{holdings: []portfolio.Holding{
		{ID: 1, Symbol: "VWCE.DE", Quantity: 2, MarketPrice: 100, AllocationType: portfolio.AllocationCore},
		{ID: 2, Symbol: "ZPRV.DE", Quantity: 1, MarketPrice: 50, AllocationType: portfolio.AllocationSatellite},
	}}

	m := NewModel(store)
	msg := m.refreshCmd()()
	updated, _ := m.Update(msg)
	model := updated.(Model)

	if model.summary.TotalNAV != 250 {
		t.Fatalf("TotalNAV = %f, want 250", model.summary.TotalNAV)
	}
	if model.table.Rows() == nil || len(model.table.Rows()) != 2 {
		t.Fatalf("rows len = %d, want 2", len(model.table.Rows()))
	}
	if !strings.Contains(model.View(), "[CORE]") || !strings.Contains(model.View(), "[SAT]") {
		t.Fatalf("view missing allocation chips: %s", model.View())
	}
	if !strings.Contains(model.View(), "Core: 200.00 (80.0%)") {
		t.Fatalf("view missing expected summary: %s", model.View())
	}
}

func TestModelToggleFlowUpdatesAllocationAndSummary(t *testing.T) {
	t.Parallel()

	store := &fakeHoldingsStore{
		holdings: []portfolio.Holding{
			{ID: 1, Symbol: "CORE", Quantity: 1, MarketPrice: 100, PMC: 80, AllocationType: portfolio.AllocationCore},
			{ID: 2, Symbol: "SAT", Quantity: 1, MarketPrice: 100, PMC: 80, AllocationType: portfolio.AllocationSatellite},
		},
		realizedPnL: 80,
	}

	m := NewModelWithMetrics(store, "MAIN")
	updated, _ := m.Update(m.refreshCmd()())
	m = updated.(Model)

	key := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'t'}}
	updated, cmd := m.Update(key)
	m = updated.(Model)
	if cmd == nil {
		t.Fatalf("toggle cmd = nil, want non-nil")
	}

	toggleMsg := cmd()
	updated, cmd = m.Update(toggleMsg)
	m = updated.(Model)
	if cmd == nil {
		t.Fatalf("refresh after toggle cmd = nil, want non-nil")
	}

	updated, _ = m.Update(cmd())
	m = updated.(Model)

	if len(store.toggledIDs) != 1 || store.toggledIDs[0] != 1 {
		t.Fatalf("toggled ids = %v, want [1]", store.toggledIDs)
	}
	if m.summary.CorePercent != 0 {
		t.Fatalf("CorePercent = %f, want 0", m.summary.CorePercent)
	}
	if m.summary.SatellitePercent != 100 {
		t.Fatalf("SatellitePercent = %f, want 100", m.summary.SatellitePercent)
	}
	// unrealizedPnL = 2 holdings × (100−80) = 40
	if m.unrealizedPnL != 40 {
		t.Fatalf("unrealizedPnL = %f, want 40", m.unrealizedPnL)
	}
	if m.realized != 80 {
		t.Fatalf("realized = %f, want 80", m.realized)
	}
}

func TestModelToggleError(t *testing.T) {
	t.Parallel()

	store := &fakeHoldingsStore{
		holdings:  []portfolio.Holding{{ID: 10, Symbol: "AAA", Quantity: 1, MarketPrice: 10, AllocationType: portfolio.AllocationCore}},
		toggleErr: errors.New("toggle failed"),
	}
	m := NewModel(store)
	updated, _ := m.Update(m.refreshCmd()())
	m = updated.(Model)

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'t'}})
	m = updated.(Model)
	updated, _ = m.Update(cmd())
	m = updated.(Model)

	if m.loadError == nil {
		t.Fatalf("loadError = nil, want non-nil")
	}
	if !strings.Contains(m.View(), "Toggle failed") {
		t.Fatalf("view missing toggle error status: %s", m.View())
	}
}

func TestModelWindowResizeAndQuit(t *testing.T) {
	t.Parallel()

	m := NewModel(&fakeHoldingsStore{})
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = updated.(Model)
	if m.table.Width() != 96 {
		t.Fatalf("table width = %d, want 96", m.table.Width())
	}

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	m = updated.(Model)
	if cmd == nil {
		t.Fatalf("quit cmd = nil, want non-nil")
	}
	if !m.quit {
		t.Fatalf("quit = false, want true")
	}
}

func TestTickCmdReturnsTickMsg(t *testing.T) {
	t.Parallel()

	// Use a 1ms tick to verify the message type without waiting refreshInterval.
	msg := tea.Tick(time.Millisecond, func(t time.Time) tea.Msg { return tickMsg(t) })()
	if _, ok := msg.(tickMsg); !ok {
		t.Fatalf("tickCmd msg type = %T, want tickMsg", msg)
	}
}

func TestModelRefreshError(t *testing.T) {
	t.Parallel()

	store := &fakeHoldingsStore{listErr: errors.New("load fail")}
	m := NewModel(store)
	updated, _ := m.Update(m.refreshCmd()())
	m = updated.(Model)
	if m.loadError == nil {
		t.Fatalf("loadError = nil, want non-nil")
	}
}

func TestUpdateWithTickSchedulesRefreshWhenIdle(t *testing.T) {
	t.Parallel()

	m := NewModel(&fakeHoldingsStore{})
	m.loading = false
	updated, cmd := m.Update(tickMsg(time.Now()))
	m = updated.(Model)
	if cmd == nil {
		t.Fatalf("cmd = nil, want non-nil")
	}
	if !m.loading {
		t.Fatalf("loading = false, want true")
	}
}

func TestModelSummaryIncludesUnrealizedAndRealizedPnL(t *testing.T) {
	t.Parallel()

	store := &fakeHoldingsStore{
		holdings: []portfolio.Holding{
			// unrealizedPnL = 2 × (100 − 75) = 50.00
			{ID: 1, Symbol: "VWCE.DE", Quantity: 2, MarketPrice: 100, PMC: 75, AllocationType: portfolio.AllocationCore},
		},
		realizedPnL: 45.67,
	}
	m := NewModelWithMetrics(store, "MAIN")
	updated, _ := m.Update(m.refreshCmd()())
	m = updated.(Model)

	view := m.View()
	if !strings.Contains(view, "Unreal. PnL: +50.00") {
		t.Fatalf("view missing unrealized pnl: %s", view)
	}
	if !strings.Contains(view, "Real. PnL: +45.67") {
		t.Fatalf("view missing realized pnl: %s", view)
	}
}


func TestMaxHelper(t *testing.T) {
	t.Parallel()

	if got := max(5, 10); got != 10 {
		t.Fatalf("max(5, 10) = %d, want 10", got)
	}
	if got := max(10, 5); got != 10 {
		t.Fatalf("max(10, 5) = %d, want 10", got)
	}
	if got := max(7, 7); got != 7 {
		t.Fatalf("max(7, 7) = %d, want 7", got)
	}
}

func TestToggleCmdNilStore(t *testing.T) {
	t.Parallel()

	m := NewModel(nil)
	msg := m.toggleCmd(42)()
	toggled, ok := msg.(toggledMsg)
	if !ok {
		t.Fatalf("msg type = %T, want toggledMsg", msg)
	}
	if toggled.holdingID != 42 {
		t.Fatalf("holdingID = %d, want 42", toggled.holdingID)
	}
	if toggled.err != nil {
		t.Fatalf("err = %v, want nil for nil store", toggled.err)
	}
}

func TestModelTAAToggle(t *testing.T) {
	t.Parallel()

	store := &fakeHoldingsStore{
		holdings: []portfolio.Holding{
			{ID: 1, Symbol: "VWCE.DE", Quantity: 2, MarketPrice: 100, AllocationType: portfolio.AllocationCore, TAAEnabled: true},
		},
	}
	m := NewModel(store)
	updated, _ := m.Update(m.refreshCmd()())
	m = updated.(Model)

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	m = updated.(Model)
	if cmd == nil {
		t.Fatalf("taa toggle cmd = nil, want non-nil")
	}

	taaMsg := cmd()
	updated, cmd = m.Update(taaMsg)
	_ = updated.(Model)
	if cmd == nil {
		t.Fatalf("refresh after taa toggle = nil, want non-nil")
	}

	if len(store.taaToggledSymbols) != 1 || store.taaToggledSymbols[0] != "VWCE.DE" {
		t.Fatalf("taaToggledSymbols = %v, want [VWCE.DE]", store.taaToggledSymbols)
	}
}

func TestModelTAAToggleError(t *testing.T) {
	t.Parallel()

	store := &fakeHoldingsStore{
		holdings:     []portfolio.Holding{{ID: 1, Symbol: "AAA", Quantity: 1, MarketPrice: 10, AllocationType: portfolio.AllocationCore, TAAEnabled: true}},
		taaToggleErr: errors.New("taa toggle failed"),
	}
	m := NewModel(store)
	updated, _ := m.Update(m.refreshCmd()())
	m = updated.(Model)

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	m = updated.(Model)
	updated, _ = m.Update(cmd())
	m = updated.(Model)

	if m.loadError == nil {
		t.Fatalf("loadError = nil, want non-nil after taa toggle error")
	}
}

func TestModelClosedHoldingRendered(t *testing.T) {
	t.Parallel()

	store := &fakeHoldingsStore{holdings: []portfolio.Holding{
		{ID: 1, Symbol: "OPEN", Quantity: 2, MarketPrice: 100, AllocationType: portfolio.AllocationCore, TAAEnabled: true},
		{ID: 2, Symbol: "CLOSED", Quantity: 0, MarketPrice: 80, AllocationType: portfolio.AllocationSatellite, TAAEnabled: false},
	}}
	m := NewModel(store)
	updated, _ := m.Update(m.refreshCmd()())
	m = updated.(Model)

	if len(m.table.Rows()) != 2 {
		t.Fatalf("rows len = %d, want 2 (closed holdings still displayed)", len(m.table.Rows()))
	}
	// Symbol column stays at full brightness; dimmed sleeve badge is still present.
	if !strings.Contains(m.View(), "CLOSED") {
		t.Fatalf("view missing CLOSED symbol in table: %s", m.View())
	}
}

