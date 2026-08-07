package tui

import (
	"context"
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/tgragnato/orbiter/internal/configuration"
)

// fakeSettingsService implements SettingsService for testing.
type fakeSettingsService struct {
	coreRatio float64
	satRatio  float64
	rebalance float64
	costBasis configuration.CostBasisMethod
	provider  string
	currency  string
	loadErr   error
	saveErr   error
}

func (f *fakeSettingsService) GetCoreSatelliteTargets(_ context.Context) (configuration.CoreSatelliteTargetSetting, error) {
	return configuration.CoreSatelliteTargetSetting{CoreRatio: f.coreRatio, SatelliteRatio: f.satRatio}, f.loadErr
}
func (f *fakeSettingsService) SetCoreSatelliteTargets(_ context.Context, _ configuration.CoreSatelliteTargetSetting) error {
	return f.saveErr
}
func (f *fakeSettingsService) GetTAA(_ context.Context) (configuration.TAASetting, error) {
	return configuration.TAASetting{RebalanceThreshold: f.rebalance}, f.loadErr
}
func (f *fakeSettingsService) SetTAA(_ context.Context, _ configuration.TAASetting) error {
	return f.saveErr
}
func (f *fakeSettingsService) GetCostBasisMethod(_ context.Context) (configuration.CostBasisMethod, error) {
	return f.costBasis, f.loadErr
}
func (f *fakeSettingsService) SetCostBasisMethod(_ context.Context, _ configuration.CostBasisMethod) error {
	return f.saveErr
}
func (f *fakeSettingsService) GetDataProvider(_ context.Context) (configuration.DataProviderSetting, error) {
	return configuration.DataProviderSetting{Provider: f.provider, Currency: f.currency}, f.loadErr
}
func (f *fakeSettingsService) SetDataProvider(_ context.Context, _ configuration.DataProviderSetting) error {
	return f.saveErr
}

func TestSettingsTabInitNilService(t *testing.T) {
	t.Parallel()

	m := NewSettingsTabModel(nil)
	cmd := m.Init()
	// Only the activate message cmd is returned when svc is nil.
	if cmd == nil {
		t.Fatalf("Init() cmd = nil, want non-nil (activate message)")
	}
}

func TestSettingsTabInitLoadsSettings(t *testing.T) {
	t.Parallel()

	svc := &fakeSettingsService{
		coreRatio: 0.8, satRatio: 0.2, rebalance: 0.05,
		costBasis: configuration.CostBasisFIFO,
		provider:  "YAHOO", currency: "EUR",
	}
	m := NewSettingsTabModel(svc)
	loadMsg := m.loadCmd()().(settingsLoadedMsg)

	updated, _ := m.Update(loadMsg)
	m = updated.(SettingsTabModel)

	if m.costBasis != configuration.CostBasisFIFO {
		t.Errorf("costBasis = %q, want FIFO", m.costBasis)
	}
	if !strings.Contains(m.status, "loaded") {
		t.Errorf("status = %q, want 'loaded'", m.status)
	}
}

func TestSettingsTabLoadError(t *testing.T) {
	t.Parallel()

	svc := &fakeSettingsService{loadErr: errors.New("db down")}
	m := NewSettingsTabModel(svc)
	msg := m.loadCmd()()
	updated, _ := m.Update(msg)
	m = updated.(SettingsTabModel)

	if !strings.Contains(m.status, "error") && !strings.Contains(m.status, "Error") {
		t.Errorf("status = %q, want error mention", m.status)
	}
}

func TestSettingsTabFocusNavigation(t *testing.T) {
	t.Parallel()

	m := NewSettingsTabModel(nil)
	// Simulate activate to focus first field.
	updated, _ := m.Update(settingsActivateMsg{})
	m = updated.(SettingsTabModel)
	if m.focused != settingFieldCoreRatio {
		t.Fatalf("focused = %d, want settingFieldCoreRatio", m.focused)
	}

	// j moves down.
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	m = updated.(SettingsTabModel)
	if m.focused != settingFieldSatRatio {
		t.Fatalf("focused after j = %d, want settingFieldSatRatio", m.focused)
	}

	// k moves back up.
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	m = updated.(SettingsTabModel)
	if m.focused != settingFieldCoreRatio {
		t.Fatalf("focused after k = %d, want settingFieldCoreRatio", m.focused)
	}
}

func TestSettingsTabCostBasisCycle(t *testing.T) {
	t.Parallel()

	m := NewSettingsTabModel(nil)
	// Navigate to cost basis field.
	for range settingFieldCostBasis {
		updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
		m = updated.(SettingsTabModel)
	}
	if m.focused != settingFieldCostBasis {
		t.Fatalf("focused = %d, want settingFieldCostBasis", m.focused)
	}

	// Cycle through PMC → FIFO → LIFO → PMC.
	space := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}}
	updated, _ := m.Update(space)
	m = updated.(SettingsTabModel)
	if m.costBasis != configuration.CostBasisFIFO {
		t.Errorf("after 1 space: costBasis = %q, want FIFO", m.costBasis)
	}
	updated, _ = m.Update(space)
	m = updated.(SettingsTabModel)
	if m.costBasis != configuration.CostBasisLIFO {
		t.Errorf("after 2 spaces: costBasis = %q, want LIFO", m.costBasis)
	}
	updated, _ = m.Update(space)
	m = updated.(SettingsTabModel)
	if m.costBasis != configuration.CostBasisPMC {
		t.Errorf("after 3 spaces: costBasis = %q, want PMC", m.costBasis)
	}
}

func TestSettingsTabSaveSuccess(t *testing.T) {
	t.Parallel()

	svc := &fakeSettingsService{
		coreRatio: 0.8, satRatio: 0.2, rebalance: 0.05,
		costBasis: configuration.CostBasisPMC, provider: "YAHOO", currency: "EUR",
	}
	m := NewSettingsTabModel(svc)
	// Populate fields via a load.
	loaded := m.loadCmd()()
	updated, _ := m.Update(loaded)
	m = updated.(SettingsTabModel)

	// Trigger save.
	saveMsg := m.saveCmd()().(settingsSavedMsg)
	updated, _ = m.Update(saveMsg)
	m = updated.(SettingsTabModel)

	if !strings.Contains(m.status, "saved") {
		t.Errorf("status = %q, want 'saved'", m.status)
	}
}

func TestSettingsTabSaveError(t *testing.T) {
	t.Parallel()

	svc := &fakeSettingsService{
		coreRatio: 0.8, satRatio: 0.2, rebalance: 0.05,
		costBasis: configuration.CostBasisPMC, provider: "YAHOO", currency: "EUR",
		saveErr: errors.New("write failed"),
	}
	m := NewSettingsTabModel(svc)
	loaded := m.loadCmd()()
	updated, _ := m.Update(loaded)
	m = updated.(SettingsTabModel)

	saveMsg := m.saveCmd()().(settingsSavedMsg)
	updated, _ = m.Update(saveMsg)
	m = updated.(SettingsTabModel)

	if !strings.Contains(m.status, "Error") {
		t.Errorf("status = %q, want error mention", m.status)
	}
}

func TestSettingsTabSaveNilService(t *testing.T) {
	t.Parallel()

	m := NewSettingsTabModel(nil)
	msg := m.saveCmd()().(settingsSavedMsg)
	if msg.err == nil {
		t.Fatalf("saveCmd with nil svc: err = nil, want non-nil")
	}
}

func TestSettingsTabSaveValidationError(t *testing.T) {
	t.Parallel()

	svc := &fakeSettingsService{}
	m := NewSettingsTabModel(svc)
	// Leave coreInput empty — should fail to parse.
	msg := m.saveCmd()().(settingsSavedMsg)
	if msg.err == nil {
		t.Fatalf("saveCmd with empty inputs: err = nil, want parse error")
	}
}

func TestSettingsTabViewContainsExpectedSections(t *testing.T) {
	t.Parallel()

	m := NewSettingsTabModel(nil)
	view := m.View()

	for _, want := range []string{"Tab 3", "Portfolio Targets", "TAA Parameters", "Cost Basis", "Data Provider"} {
		if !strings.Contains(view, want) {
			t.Errorf("View() missing %q", want)
		}
	}
}

func TestSettingsTabQuitSuppressesView(t *testing.T) {
	t.Parallel()

	m := NewSettingsTabModel(nil)
	m.quit = true
	if got := m.View(); got != "" {
		t.Fatalf("View() while quit = %q, want empty", got)
	}
}

func TestSettingsTabQuitSuppressesUpdate(t *testing.T) {
	t.Parallel()

	m := NewSettingsTabModel(nil)
	m.quit = true
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
	_ = updated.(SettingsTabModel)
	if cmd != nil {
		t.Fatalf("cmd after quit = non-nil, want nil")
	}
}
