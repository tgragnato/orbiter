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
	apiKey       string
	currency     string
	brokerConfig configuration.TAABrokerConfig
	loadErr      error
	saveErr      error
}

func (f *fakeSettingsService) GetYahooCredentials(_ context.Context) (configuration.YahooCredentialsSetting, error) {
	return configuration.YahooCredentialsSetting{APIKey: f.apiKey}, f.loadErr
}

func (f *fakeSettingsService) SetYahooCredentials(_ context.Context, value configuration.YahooCredentialsSetting) error {
	f.apiKey = value.APIKey
	return f.saveErr
}

func (f *fakeSettingsService) GetBaseCurrency(_ context.Context) (string, error) {
	return f.currency, f.loadErr
}

func (f *fakeSettingsService) SetBaseCurrency(_ context.Context, currency string) error {
	f.currency = currency
	return f.saveErr
}

func (f *fakeSettingsService) GetBrokerConfig(_ context.Context) (configuration.TAABrokerConfig, error) {
	return f.brokerConfig, f.loadErr
}

func (f *fakeSettingsService) SetBrokerConfig(_ context.Context, value configuration.TAABrokerConfig) error {
	f.brokerConfig = value
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
		apiKey:   "test-key",
		currency: "USD",
		brokerConfig: configuration.TAABrokerConfig{
			TaxRate:          0.26,
			BrokerFeePercent: 0.0019,
			MaxBrokerFee:     18.9,
			Buffer:           0.01,
		},
	}
	m := NewSettingsTabModel(svc)
	loadMsg := m.loadCmd()().(settingsLoadedMsg)

	updated, _ := m.Update(loadMsg)
	m = updated.(SettingsTabModel)

	if m.yahooKeyInput.Value() != "test-key" {
		t.Errorf("apiKey = %q, want test-key", m.yahooKeyInput.Value())
	}
	if m.currencyInput.Value() != "USD" {
		t.Errorf("currency = %q, want USD", m.currencyInput.Value())
	}
	if m.taxRateInput.Value() != "26" {
		t.Errorf("taxRate = %q, want 26", m.taxRateInput.Value())
	}
	if m.brokerFeeInput.Value() != "0.19" {
		t.Errorf("brokerFee = %q, want 0.19", m.brokerFeeInput.Value())
	}
	if m.maxBrokerFeeInput.Value() != "18.9" {
		t.Errorf("maxBrokerFee = %q, want 18.9", m.maxBrokerFeeInput.Value())
	}
	if m.bufferInput.Value() != "1" {
		t.Errorf("buffer = %q, want 1", m.bufferInput.Value())
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
	if m.focused != settingFieldYahooAPIKey {
		t.Fatalf("focused = %d, want settingFieldYahooAPIKey", m.focused)
	}

	// j moves down.
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	m = updated.(SettingsTabModel)
	if m.focused != settingFieldCurrency {
		t.Fatalf("focused after j = %d, want settingFieldCurrency", m.focused)
	}

	// k moves back up.
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	m = updated.(SettingsTabModel)
	if m.focused != settingFieldYahooAPIKey {
		t.Fatalf("focused after k = %d, want settingFieldYahooAPIKey", m.focused)
	}
}

func TestSettingsTabSaveSuccess(t *testing.T) {
	t.Parallel()

	svc := &fakeSettingsService{
		apiKey:   "old-key",
		currency: "EUR",
		brokerConfig: configuration.TAABrokerConfig{
			TaxRate:          0.26,
			BrokerFeePercent: 0.0019,
			MaxBrokerFee:     18.9,
			Buffer:           0.01,
		},
	}
	m := NewSettingsTabModel(svc)
	// Populate all fields via a load.
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

func TestSettingsTabBrokerConfigRoundTrip(t *testing.T) {
	t.Parallel()

	svc := &fakeSettingsService{
		currency: "EUR",
		brokerConfig: configuration.TAABrokerConfig{
			TaxRate:          0.26,
			BrokerFeePercent: 0.0019,
			MaxBrokerFee:     18.9,
			Buffer:           0.01,
		},
	}
	m := NewSettingsTabModel(svc)

	loaded := m.loadCmd()()
	updated, _ := m.Update(loaded)
	m = updated.(SettingsTabModel)

	saveMsg := m.saveCmd()().(settingsSavedMsg)
	if saveMsg.err != nil {
		t.Fatalf("save error: %v", saveMsg.err)
	}

	// Verify the saved broker config matches original values.
	const eps = 1e-9
	if diff := saveMsg.brokerConfig.TaxRate - 0.26; diff > eps || diff < -eps {
		t.Errorf("TaxRate = %g, want 0.26", saveMsg.brokerConfig.TaxRate)
	}
	if diff := saveMsg.brokerConfig.BrokerFeePercent - 0.0019; diff > eps || diff < -eps {
		t.Errorf("BrokerFeePercent = %g, want 0.0019", saveMsg.brokerConfig.BrokerFeePercent)
	}
	if diff := saveMsg.brokerConfig.MaxBrokerFee - 18.9; diff > eps || diff < -eps {
		t.Errorf("MaxBrokerFee = %g, want 18.9", saveMsg.brokerConfig.MaxBrokerFee)
	}
	if diff := saveMsg.brokerConfig.Buffer - 0.01; diff > eps || diff < -eps {
		t.Errorf("Buffer = %g, want 0.01", saveMsg.brokerConfig.Buffer)
	}
}

func TestSettingsTabBrokerConfigParseError(t *testing.T) {
	t.Parallel()

	svc := &fakeSettingsService{currency: "EUR"}
	m := NewSettingsTabModel(svc)
	m.taxRateInput.SetValue("not-a-number")
	msg := m.saveCmd()().(settingsSavedMsg)
	if msg.err == nil {
		t.Fatal("saveCmd with invalid tax rate: err = nil, want parse error")
	}
}

func TestSettingsTabSaveError(t *testing.T) {
	t.Parallel()

	svc := &fakeSettingsService{
		apiKey:   "key",
		currency: "EUR",
		saveErr:  errors.New("write failed"),
		// brokerConfig zero-value: TaxRate=0, BrokerFee=0, MaxBrokerFee=0, Buffer=0 — all valid
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
	m.currencyInput.SetValue("EURO") // 4 letters instead of 3
	msg := m.saveCmd()().(settingsSavedMsg)
	if msg.err == nil {
		t.Fatalf("saveCmd with invalid currency: err = nil, want validation error")
	}
}

func TestSettingsTabViewContainsExpectedSections(t *testing.T) {
	t.Parallel()

	m := NewSettingsTabModel(nil)
	view := m.View()

	for _, want := range []string{"Data Provider Credentials", "Portfolio Currency", "Yahoo API Key", "Base Currency", "TAA Broker Configuration", "Tax Rate", "Broker Fee", "Max Broker Fee", "Buffer"} {
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
