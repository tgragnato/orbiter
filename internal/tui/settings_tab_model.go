package tui

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/tgragnato/orbiter/internal/configuration"
)

// SettingsService is the configuration persistence contract required by the settings tab.
// *configuration.Service satisfies this interface.
type SettingsService interface {
	// GetYahooCredentials returns persisted Yahoo provider credentials.
	GetYahooCredentials(ctx context.Context) (configuration.YahooCredentialsSetting, error)
	// SetYahooCredentials updates Yahoo provider credentials.
	SetYahooCredentials(ctx context.Context, value configuration.YahooCredentialsSetting) error
	// GetBaseCurrency returns the ISO 4217 portfolio base currency (e.g. "EUR").
	GetBaseCurrency(ctx context.Context) (string, error)
	// SetBaseCurrency validates and persists the ISO 4217 portfolio base currency.
	SetBaseCurrency(ctx context.Context, currency string) error
	// GetBrokerConfig returns the TAA engine fiscal friction parameters.
	GetBrokerConfig(ctx context.Context) (configuration.TAABrokerConfig, error)
	// SetBrokerConfig validates and persists the TAA engine fiscal parameters.
	SetBrokerConfig(ctx context.Context, value configuration.TAABrokerConfig) error
}

const (
	settingFieldYahooAPIKey = iota
	settingFieldCurrency    // 1
	settingFieldTaxRate     // 2  — stored as decimal, displayed as %
	settingFieldBrokerFeePercent
	settingFieldMaxBrokerFee // 4  — absolute base-currency amount
	settingFieldBuffer       // 5  — stored as decimal, displayed as %
)

const settingFieldCount = 6

const (
	settingLabelWidth      = 22
	settingPctDivisor      = 100.0
	settingCurrencyCodeLen = 3
	settingCurrencyDefault = "EUR"
)

// ErrSettingsServiceNotConfigured is returned when the settings service is nil.
var ErrSettingsServiceNotConfigured = errors.New("settings service not configured")

// ErrBaseCurrencyInvalid is returned when the base currency is not a 3-letter ISO code.
var ErrBaseCurrencyInvalid = errors.New("base currency: must be a 3-letter ISO code")

type settingsActivateMsg struct{}

type settingsLoadedMsg struct {
	yahooAPIKey  string
	currency     string
	brokerConfig configuration.TAABrokerConfig
	err          error
}

type settingsSavedMsg struct {
	err          error
	apiKey       string                        // populated on success; empty string is valid (key removed)
	currency     string                        // populated on success; always 3-letter ISO code
	brokerConfig configuration.TAABrokerConfig // populated on success
}

// SettingsTabModel renders editable application configuration in Tab 3.
//
//nolint:recvcheck // tea.Model interface requires value receivers; blurCurrent/focusCurrent use pointer
type SettingsTabModel struct {
	svc     SettingsService
	focused int
	quit    bool
	status  string

	yahooKeyInput     textinput.Model
	currencyInput     textinput.Model
	taxRateInput      textinput.Model // displayed as % (e.g. "26"), stored as decimal 0.26
	brokerFeeInput    textinput.Model // displayed as % (e.g. "0.19"), stored as decimal 0.0019
	maxBrokerFeeInput textinput.Model // absolute base-currency amount (e.g. "18.9")
	bufferInput       textinput.Model // displayed as % (e.g. "1"), stored as decimal 0.01

	styles settingsStyles
}

type settingsStyles struct {
	title        lipgloss.Style
	sectionTitle lipgloss.Style
	label        lipgloss.Style
	labelFocused lipgloss.Style
	status       lipgloss.Style
	errStyle     lipgloss.Style
	hint         lipgloss.Style
}

func newSettingsStyles() settingsStyles {
	return settingsStyles{
		title:        lipgloss.NewStyle().Bold(true),
		sectionTitle: lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39")),
		label:        lipgloss.NewStyle().Width(settingLabelWidth).Foreground(lipgloss.Color("252")),
		labelFocused: lipgloss.NewStyle().Width(settingLabelWidth).Foreground(lipgloss.Color("33")),
		status:       lipgloss.NewStyle().Foreground(lipgloss.Color("242")),
		errStyle:     lipgloss.NewStyle().Foreground(lipgloss.Color("196")),
		hint:         lipgloss.NewStyle().Foreground(lipgloss.Color("242")),
	}
}

// NewSettingsTabModel creates the Tab 3 settings model wired to the given service.
// Pass nil to render the tab in read-only / unconfigured mode.
func NewSettingsTabModel(svc SettingsService) SettingsTabModel {
	yahooKeyInput := textinput.New()
	yahooKeyInput.Placeholder = "Optional API Key"
	yahooKeyInput.CharLimit = 64
	yahooKeyInput.Width = 32

	currencyInput := textinput.New()
	currencyInput.Placeholder = settingCurrencyDefault
	currencyInput.CharLimit = 8
	currencyInput.Width = 6

	taxRateInput := textinput.New()
	taxRateInput.Placeholder = "26"
	taxRateInput.CharLimit = 8
	taxRateInput.Width = 10

	brokerFeeInput := textinput.New()
	brokerFeeInput.Placeholder = "0.19"
	brokerFeeInput.CharLimit = 8
	brokerFeeInput.Width = 10

	maxBrokerFeeInput := textinput.New()
	maxBrokerFeeInput.Placeholder = "18.9"
	maxBrokerFeeInput.CharLimit = 10
	maxBrokerFeeInput.Width = 10

	bufferInput := textinput.New()
	bufferInput.Placeholder = "1"
	bufferInput.CharLimit = 8
	bufferInput.Width = 10

	return SettingsTabModel{
		svc:               svc,
		focused:           settingFieldYahooAPIKey,
		quit:              false,
		status:            "",
		yahooKeyInput:     yahooKeyInput,
		currencyInput:     currencyInput,
		taxRateInput:      taxRateInput,
		brokerFeeInput:    brokerFeeInput,
		maxBrokerFeeInput: maxBrokerFeeInput,
		bufferInput:       bufferInput,
		styles:            newSettingsStyles(),
	}
}

// fmtPct converts a decimal rate to a compact percentage string (e.g. 0.26 → "26", 0.0019 → "0.19").
func fmtPct(val float64) string {
	return strconv.FormatFloat(val*settingPctDivisor, 'f', -1, 64)
}

// parsePct parses a percentage string and returns the equivalent decimal (e.g. "26" → 0.26).
func parsePct(raw string) (float64, error) {
	val, err := strconv.ParseFloat(strings.TrimSpace(raw), 64)
	if err != nil {
		return 0, fmt.Errorf("parsePct: %w", err)
	}

	return val / settingPctDivisor, nil
}

// Init focuses the first field and loads current settings from the service.
func (m SettingsTabModel) Init() tea.Cmd {
	cmds := []tea.Cmd{func() tea.Msg { return settingsActivateMsg{} }}
	if m.svc != nil {
		cmds = append(cmds, m.loadCmd())
	}

	return tea.Batch(cmds...)
}

// Update handles incoming messages for the settings tab.
//
//nolint:cyclop // settings tab dispatch; complexity is inherent in multi-field form
func (m SettingsTabModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if m.quit {
		return m, nil
	}

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		// Settings renders plain text; terminal dimensions are not needed.
		return m, nil
	case settingsActivateMsg:
		m.focused = settingFieldYahooAPIKey
		cmd := m.yahooKeyInput.Focus()

		return m, cmd

	case settingsLoadedMsg:
		if msg.err != nil {
			m.status = fmt.Sprintf("Load error: %v", msg.err)

			return m, nil
		}

		m.yahooKeyInput.SetValue(msg.yahooAPIKey)
		m.currencyInput.SetValue(msg.currency)
		m.taxRateInput.SetValue(fmtPct(msg.brokerConfig.TaxRate))
		m.brokerFeeInput.SetValue(fmtPct(msg.brokerConfig.BrokerFeePercent))
		m.maxBrokerFeeInput.SetValue(strconv.FormatFloat(msg.brokerConfig.MaxBrokerFee, 'f', -1, 64))
		m.bufferInput.SetValue(fmtPct(msg.brokerConfig.Buffer))
		m.status = "Settings loaded"

		return m, nil

	case settingsSavedMsg:
		if msg.err != nil {
			m.status = fmt.Sprintf("Error: %v", msg.err)
		} else {
			m.status = "Settings saved"
		}

		return m, nil
	}

	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case "j", keyDown:
			return m.moveSettingsFocus(1)
		case "k", "up":
			return m.moveSettingsFocus(-1)
		case "s", keyEnter:
			return m, m.saveCmd()
		}
	}

	return m.updateSettingsFocused(msg)
}

// NavHint returns the context-sensitive hint string that the root model merges
// into its global help line when the Settings tab is active.
func (m SettingsTabModel) NavHint() string {
	nav := "j/k: navigate · s/enter: save"
	if m.status != "" {
		return m.status + "  |  " + nav
	}

	return nav
}

// View renders the settings tab.
func (m SettingsTabModel) View() string {
	if m.quit {
		return ""
	}

	styles := m.styles
	lines := []string{
		styles.sectionTitle.Render("Data Provider Credentials"),
		m.renderSettingsInput("Yahoo API Key:       ", m.yahooKeyInput, m.focused == settingFieldYahooAPIKey),
		"",
		styles.sectionTitle.Render("Portfolio Currency"),
		m.renderSettingsInput("Base Currency (ISO): ", m.currencyInput, m.focused == settingFieldCurrency),
		"",
		styles.sectionTitle.Render("TAA Broker Configuration"),
		m.renderSettingsInput("Tax Rate (%):        ", m.taxRateInput, m.focused == settingFieldTaxRate),
		m.renderSettingsInput("Broker Fee (%):      ", m.brokerFeeInput, m.focused == settingFieldBrokerFeePercent),
		m.renderSettingsInput("Max Broker Fee:      ", m.maxBrokerFeeInput, m.focused == settingFieldMaxBrokerFee),
		m.renderSettingsInput("Buffer (%):          ", m.bufferInput, m.focused == settingFieldBuffer),
	}

	return strings.Join(lines, "\n")
}

func (m SettingsTabModel) moveSettingsFocus(delta int) (SettingsTabModel, tea.Cmd) {
	m.blurSettingsCurrent()
	m.focused = ((m.focused+delta)%settingFieldCount + settingFieldCount) % settingFieldCount
	cmd := m.focusSettingsCurrent()

	return m, cmd
}

func (m *SettingsTabModel) blurSettingsCurrent() {
	switch m.focused {
	case settingFieldYahooAPIKey:
		m.yahooKeyInput.Blur()
	case settingFieldCurrency:
		m.currencyInput.Blur()
	case settingFieldTaxRate:
		m.taxRateInput.Blur()
	case settingFieldBrokerFeePercent:
		m.brokerFeeInput.Blur()
	case settingFieldMaxBrokerFee:
		m.maxBrokerFeeInput.Blur()
	case settingFieldBuffer:
		m.bufferInput.Blur()
	}
}

func (m *SettingsTabModel) focusSettingsCurrent() tea.Cmd {
	switch m.focused {
	case settingFieldYahooAPIKey:
		return m.yahooKeyInput.Focus()
	case settingFieldCurrency:
		return m.currencyInput.Focus()
	case settingFieldTaxRate:
		return m.taxRateInput.Focus()
	case settingFieldBrokerFeePercent:
		return m.brokerFeeInput.Focus()
	case settingFieldMaxBrokerFee:
		return m.maxBrokerFeeInput.Focus()
	case settingFieldBuffer:
		return m.bufferInput.Focus()
	}

	return nil
}

func (m SettingsTabModel) updateSettingsFocused(msg tea.Msg) (SettingsTabModel, tea.Cmd) {
	var cmd tea.Cmd

	switch m.focused {
	case settingFieldYahooAPIKey:
		m.yahooKeyInput, cmd = m.yahooKeyInput.Update(msg)
	case settingFieldCurrency:
		m.currencyInput, cmd = m.currencyInput.Update(msg)
	case settingFieldTaxRate:
		m.taxRateInput, cmd = m.taxRateInput.Update(msg)
	case settingFieldBrokerFeePercent:
		m.brokerFeeInput, cmd = m.brokerFeeInput.Update(msg)
	case settingFieldMaxBrokerFee:
		m.maxBrokerFeeInput, cmd = m.maxBrokerFeeInput.Update(msg)
	case settingFieldBuffer:
		m.bufferInput, cmd = m.bufferInput.Update(msg)
	}

	return m, cmd
}

func (m SettingsTabModel) loadCmd() tea.Cmd {
	svc := m.svc

	return func() tea.Msg {
		ctx := context.Background()

		creds, err := svc.GetYahooCredentials(ctx)
		if err != nil {
			return settingsLoadedMsg{
				err:          err,
				yahooAPIKey:  "",
				currency:     "",
				brokerConfig: configuration.TAABrokerConfig{TaxRate: 0, BrokerFeePercent: 0, MaxBrokerFee: 0, Buffer: 0},
			}
		}

		baseCurrency, err := svc.GetBaseCurrency(ctx)
		if err != nil {
			return settingsLoadedMsg{
				err:          err,
				yahooAPIKey:  "",
				currency:     "",
				brokerConfig: configuration.TAABrokerConfig{TaxRate: 0, BrokerFeePercent: 0, MaxBrokerFee: 0, Buffer: 0},
			}
		}

		brokerConfig, err := svc.GetBrokerConfig(ctx)
		if err != nil {
			return settingsLoadedMsg{
				err:          err,
				yahooAPIKey:  "",
				currency:     "",
				brokerConfig: configuration.TAABrokerConfig{TaxRate: 0, BrokerFeePercent: 0, MaxBrokerFee: 0, Buffer: 0},
			}
		}

		return settingsLoadedMsg{
			yahooAPIKey:  creds.APIKey,
			currency:     baseCurrency,
			brokerConfig: brokerConfig,
			err:          nil,
		}
	}
}

//nolint:funlen // saveCmd validates and persists many fields; each branch is a required validation step
func (m SettingsTabModel) saveCmd() tea.Cmd {
	if m.svc == nil {
		return func() tea.Msg {
			return settingsSavedMsg{
				err:          ErrSettingsServiceNotConfigured,
				apiKey:       "",
				currency:     "",
				brokerConfig: configuration.TAABrokerConfig{TaxRate: 0, BrokerFeePercent: 0, MaxBrokerFee: 0, Buffer: 0},
			}
		}
	}

	apiKey := strings.TrimSpace(m.yahooKeyInput.Value())
	currencyStr := strings.ToUpper(strings.TrimSpace(m.currencyInput.Value()))

	if len(currencyStr) != settingCurrencyCodeLen {
		return func() tea.Msg {
			return settingsSavedMsg{
				err:          ErrBaseCurrencyInvalid,
				apiKey:       "",
				currency:     "",
				brokerConfig: configuration.TAABrokerConfig{TaxRate: 0, BrokerFeePercent: 0, MaxBrokerFee: 0, Buffer: 0},
			}
		}
	}

	// Parse broker config fields (displayed as %, stored as decimals).
	taxRate, err := parsePct(m.taxRateInput.Value())
	if err != nil {
		return func() tea.Msg {
			return settingsSavedMsg{
				err:          fmt.Errorf("tax rate: %w", err),
				apiKey:       "",
				currency:     "",
				brokerConfig: configuration.TAABrokerConfig{TaxRate: 0, BrokerFeePercent: 0, MaxBrokerFee: 0, Buffer: 0},
			}
		}
	}

	brokerFee, err := parsePct(m.brokerFeeInput.Value())
	if err != nil {
		return func() tea.Msg {
			return settingsSavedMsg{
				err:          fmt.Errorf("broker fee %%: %w", err),
				apiKey:       "",
				currency:     "",
				brokerConfig: configuration.TAABrokerConfig{TaxRate: 0, BrokerFeePercent: 0, MaxBrokerFee: 0, Buffer: 0},
			}
		}
	}

	maxFee, err := strconv.ParseFloat(strings.TrimSpace(m.maxBrokerFeeInput.Value()), 64)
	if err != nil {
		return func() tea.Msg {
			return settingsSavedMsg{
				err:          fmt.Errorf("max broker fee: %w", err),
				apiKey:       "",
				currency:     "",
				brokerConfig: configuration.TAABrokerConfig{TaxRate: 0, BrokerFeePercent: 0, MaxBrokerFee: 0, Buffer: 0},
			}
		}
	}

	buffer, err := parsePct(m.bufferInput.Value())
	if err != nil {
		return func() tea.Msg {
			return settingsSavedMsg{
				err:          fmt.Errorf("buffer: %w", err),
				apiKey:       "",
				currency:     "",
				brokerConfig: configuration.TAABrokerConfig{TaxRate: 0, BrokerFeePercent: 0, MaxBrokerFee: 0, Buffer: 0},
			}
		}
	}

	brokerConfig := configuration.TAABrokerConfig{
		TaxRate:          taxRate,
		BrokerFeePercent: brokerFee,
		MaxBrokerFee:     maxFee,
		Buffer:           buffer,
	}

	svc := m.svc

	return func() tea.Msg {
		ctx := context.Background()

		err := svc.SetYahooCredentials(ctx, configuration.YahooCredentialsSetting{
			APIKey: apiKey,
		})
		if err != nil {
			return settingsSavedMsg{
				err:          err,
				apiKey:       "",
				currency:     "",
				brokerConfig: configuration.TAABrokerConfig{TaxRate: 0, BrokerFeePercent: 0, MaxBrokerFee: 0, Buffer: 0},
			}
		}

		err = svc.SetBaseCurrency(ctx, currencyStr)
		if err != nil {
			return settingsSavedMsg{
				err:          err,
				apiKey:       "",
				currency:     "",
				brokerConfig: configuration.TAABrokerConfig{TaxRate: 0, BrokerFeePercent: 0, MaxBrokerFee: 0, Buffer: 0},
			}
		}

		err = svc.SetBrokerConfig(ctx, brokerConfig)
		if err != nil {
			return settingsSavedMsg{
				err:          err,
				apiKey:       "",
				currency:     "",
				brokerConfig: configuration.TAABrokerConfig{TaxRate: 0, BrokerFeePercent: 0, MaxBrokerFee: 0, Buffer: 0},
			}
		}

		return settingsSavedMsg{
			apiKey:       apiKey,
			currency:     currencyStr,
			brokerConfig: brokerConfig,
			err:          nil,
		}
	}
}

func (m SettingsTabModel) renderSettingsInput(label string, inp textinput.Model, isFocused bool) string {
	styles := m.styles

	lStyle := styles.label
	if isFocused {
		lStyle = styles.labelFocused
	}

	return lStyle.Render(label) + inp.View()
}
