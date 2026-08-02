package tui

import (
	"context"
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
	GetCostBasisMethod(ctx context.Context) (configuration.CostBasisMethod, error)
	SetCostBasisMethod(ctx context.Context, method configuration.CostBasisMethod) error
	GetDataProvider(ctx context.Context) (configuration.DataProviderSetting, error)
	SetDataProvider(ctx context.Context, value configuration.DataProviderSetting) error
	GetTAA(ctx context.Context) (configuration.TAASetting, error)
	SetTAA(ctx context.Context, value configuration.TAASetting) error
	GetCoreSatelliteTargets(ctx context.Context) (configuration.CoreSatelliteTargetSetting, error)
	SetCoreSatelliteTargets(ctx context.Context, value configuration.CoreSatelliteTargetSetting) error
}

const (
	settingFieldCoreRatio = iota
	settingFieldSatRatio
	settingFieldRebalance
	settingFieldCostBasis
	settingFieldProvider
	settingFieldCurrency
	settingFieldCount = 6
)

type settingsActivateMsg struct{}

type settingsLoadedMsg struct {
	coreRatio float64
	satRatio  float64
	rebalance float64
	costBasis configuration.CostBasisMethod
	provider  string
	currency  string
	err       error
}

type settingsSavedMsg struct{ err error }

// SettingsTabModel renders editable application configuration in Tab 3.
type SettingsTabModel struct {
	svc     SettingsService
	focused int
	quit    bool
	status  string

	coreInput     textinput.Model
	satInput      textinput.Model
	rebalInput    textinput.Model
	providerInput textinput.Model
	currencyInput textinput.Model
	costBasis     configuration.CostBasisMethod

	styles settingsStyles
}

type settingsStyles struct {
	title        lipgloss.Style
	sectionTitle lipgloss.Style
	label        lipgloss.Style
	labelFocused lipgloss.Style
	toggleOn     lipgloss.Style
	toggleOff    lipgloss.Style
	status       lipgloss.Style
	errStyle     lipgloss.Style
	hint         lipgloss.Style
}

func newSettingsStyles() settingsStyles {
	return settingsStyles{
		title:        lipgloss.NewStyle().Bold(true),
		sectionTitle: lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39")),
		label:        lipgloss.NewStyle().Width(22).Foreground(lipgloss.Color("252")),
		labelFocused: lipgloss.NewStyle().Width(22).Foreground(lipgloss.Color("33")),
		toggleOn:     lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("230")).Background(lipgloss.Color("28")).Padding(0, 1),
		toggleOff:    lipgloss.NewStyle().Foreground(lipgloss.Color("242")).Padding(0, 1),
		status:       lipgloss.NewStyle().Foreground(lipgloss.Color("242")),
		errStyle:     lipgloss.NewStyle().Foreground(lipgloss.Color("196")),
		hint:         lipgloss.NewStyle().Foreground(lipgloss.Color("242")),
	}
}

// NewSettingsTabModel creates the Tab 3 settings model wired to the given service.
// Pass nil to render the tab in read-only / unconfigured mode.
func NewSettingsTabModel(svc SettingsService) SettingsTabModel {
	coreInput := textinput.New()
	coreInput.Placeholder = "0.80"
	coreInput.CharLimit = 8
	coreInput.Width = 10

	satInput := textinput.New()
	satInput.Placeholder = "0.20"
	satInput.CharLimit = 8
	satInput.Width = 10

	rebalInput := textinput.New()
	rebalInput.Placeholder = "0.05"
	rebalInput.CharLimit = 8
	rebalInput.Width = 10

	providerInput := textinput.New()
	providerInput.Placeholder = "YAHOO"
	providerInput.CharLimit = 20
	providerInput.Width = 14

	currencyInput := textinput.New()
	currencyInput.Placeholder = "EUR"
	currencyInput.CharLimit = 8
	currencyInput.Width = 6

	return SettingsTabModel{
		svc:           svc,
		focused:       settingFieldCoreRatio,
		costBasis:     configuration.CostBasisPMC,
		coreInput:     coreInput,
		satInput:      satInput,
		rebalInput:    rebalInput,
		providerInput: providerInput,
		currencyInput: currencyInput,
		styles:        newSettingsStyles(),
	}
}

// Init focuses the first field and loads current settings from the service.
func (m SettingsTabModel) Init() tea.Cmd {
	cmds := []tea.Cmd{func() tea.Msg { return settingsActivateMsg{} }}
	if m.svc != nil {
		cmds = append(cmds, m.loadCmd())
	}
	return tea.Batch(cmds...)
}

func (m SettingsTabModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if m.quit {
		return m, nil
	}

	switch msg := msg.(type) {
	case settingsActivateMsg:
		m.focused = settingFieldCoreRatio
		cmd := m.coreInput.Focus()
		return m, cmd

	case settingsLoadedMsg:
		if msg.err != nil {
			m.status = fmt.Sprintf("Load error: %v", msg.err)
			return m, nil
		}
		m.coreInput.SetValue(fmt.Sprintf("%.4f", msg.coreRatio))
		m.satInput.SetValue(fmt.Sprintf("%.4f", msg.satRatio))
		m.rebalInput.SetValue(fmt.Sprintf("%.4f", msg.rebalance))
		m.providerInput.SetValue(msg.provider)
		m.currencyInput.SetValue(msg.currency)
		m.costBasis = msg.costBasis
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
		case "j", "down":
			return m.moveSettingsFocus(1)
		case "k", "up":
			return m.moveSettingsFocus(-1)
		case " ":
			if m.focused == settingFieldCostBasis {
				switch m.costBasis {
				case configuration.CostBasisPMC:
					m.costBasis = configuration.CostBasisFIFO
				case configuration.CostBasisFIFO:
					m.costBasis = configuration.CostBasisLIFO
				default:
					m.costBasis = configuration.CostBasisPMC
				}
				return m, nil
			}
		case "s":
			return m, m.saveCmd()
		case "enter":
			if m.focused != settingFieldCostBasis {
				return m, m.saveCmd()
			}
		}
	}

	if m.focused != settingFieldCostBasis {
		return m.updateSettingsFocused(msg)
	}
	return m, nil
}

func (m SettingsTabModel) moveSettingsFocus(delta int) (SettingsTabModel, tea.Cmd) {
	m.blurSettingsCurrent()
	m.focused = ((m.focused+delta)%settingFieldCount + settingFieldCount) % settingFieldCount
	return m, m.focusSettingsCurrent()
}

func (m *SettingsTabModel) blurSettingsCurrent() {
	switch m.focused {
	case settingFieldCoreRatio:
		m.coreInput.Blur()
	case settingFieldSatRatio:
		m.satInput.Blur()
	case settingFieldRebalance:
		m.rebalInput.Blur()
	case settingFieldProvider:
		m.providerInput.Blur()
	case settingFieldCurrency:
		m.currencyInput.Blur()
	}
}

func (m *SettingsTabModel) focusSettingsCurrent() tea.Cmd {
	switch m.focused {
	case settingFieldCoreRatio:
		return m.coreInput.Focus()
	case settingFieldSatRatio:
		return m.satInput.Focus()
	case settingFieldRebalance:
		return m.rebalInput.Focus()
	case settingFieldProvider:
		return m.providerInput.Focus()
	case settingFieldCurrency:
		return m.currencyInput.Focus()
	}
	return nil
}

func (m SettingsTabModel) updateSettingsFocused(msg tea.Msg) (SettingsTabModel, tea.Cmd) {
	var cmd tea.Cmd
	switch m.focused {
	case settingFieldCoreRatio:
		m.coreInput, cmd = m.coreInput.Update(msg)
	case settingFieldSatRatio:
		m.satInput, cmd = m.satInput.Update(msg)
	case settingFieldRebalance:
		m.rebalInput, cmd = m.rebalInput.Update(msg)
	case settingFieldProvider:
		m.providerInput, cmd = m.providerInput.Update(msg)
	case settingFieldCurrency:
		m.currencyInput, cmd = m.currencyInput.Update(msg)
	}
	return m, cmd
}

func (m SettingsTabModel) loadCmd() tea.Cmd {
	svc := m.svc
	return func() tea.Msg {
		ctx := context.Background()
		targets, err := svc.GetCoreSatelliteTargets(ctx)
		if err != nil {
			return settingsLoadedMsg{err: err}
		}
		taa, err := svc.GetTAA(ctx)
		if err != nil {
			return settingsLoadedMsg{err: err}
		}
		costBasis, err := svc.GetCostBasisMethod(ctx)
		if err != nil {
			return settingsLoadedMsg{err: err}
		}
		provider, err := svc.GetDataProvider(ctx)
		if err != nil {
			return settingsLoadedMsg{err: err}
		}
		return settingsLoadedMsg{
			coreRatio: targets.CoreRatio,
			satRatio:  targets.SatelliteRatio,
			rebalance: taa.RebalanceThreshold,
			costBasis: costBasis,
			provider:  provider.Provider,
			currency:  provider.Currency,
		}
	}
}

func (m SettingsTabModel) saveCmd() tea.Cmd {
	if m.svc == nil {
		return func() tea.Msg { return settingsSavedMsg{err: fmt.Errorf("settings service not configured")} }
	}

	coreStr := strings.TrimSpace(m.coreInput.Value())
	satStr := strings.TrimSpace(m.satInput.Value())
	rebalStr := strings.TrimSpace(m.rebalInput.Value())
	providerStr := strings.ToUpper(strings.TrimSpace(m.providerInput.Value()))
	currencyStr := strings.ToUpper(strings.TrimSpace(m.currencyInput.Value()))
	costBasis := m.costBasis

	coreRatio, err := strconv.ParseFloat(coreStr, 64)
	if err != nil {
		return func() tea.Msg { return settingsSavedMsg{err: fmt.Errorf("core ratio: must be a number")} }
	}
	satRatio, err := strconv.ParseFloat(satStr, 64)
	if err != nil {
		return func() tea.Msg { return settingsSavedMsg{err: fmt.Errorf("satellite ratio: must be a number")} }
	}
	rebalThreshold, err := strconv.ParseFloat(rebalStr, 64)
	if err != nil {
		return func() tea.Msg { return settingsSavedMsg{err: fmt.Errorf("rebalance threshold: must be a number")} }
	}

	svc := m.svc
	return func() tea.Msg {
		ctx := context.Background()
		if err := svc.SetCoreSatelliteTargets(ctx, configuration.CoreSatelliteTargetSetting{
			CoreRatio:      coreRatio,
			SatelliteRatio: satRatio,
		}); err != nil {
			return settingsSavedMsg{err: err}
		}
		if err := svc.SetTAA(ctx, configuration.TAASetting{
			RebalanceThreshold: rebalThreshold,
		}); err != nil {
			return settingsSavedMsg{err: err}
		}
		if err := svc.SetCostBasisMethod(ctx, costBasis); err != nil {
			return settingsSavedMsg{err: err}
		}
		if err := svc.SetDataProvider(ctx, configuration.DataProviderSetting{
			Provider: providerStr,
			Currency: currencyStr,
		}); err != nil {
			return settingsSavedMsg{err: err}
		}
		return settingsSavedMsg{}
	}
}

func (m SettingsTabModel) View() string {
	if m.quit {
		return ""
	}

	st := m.styles
	lines := []string{
		st.title.Render("Tab 3 - Settings"),
		"",
		st.sectionTitle.Render("Portfolio Targets"),
		m.renderSettingsInput("Core Ratio:          ", m.coreInput, m.focused == settingFieldCoreRatio),
		m.renderSettingsInput("Satellite Ratio:     ", m.satInput, m.focused == settingFieldSatRatio),
		"",
		st.sectionTitle.Render("TAA Parameters"),
		m.renderSettingsInput("Rebalance Threshold: ", m.rebalInput, m.focused == settingFieldRebalance),
		"",
		st.sectionTitle.Render("Cost Basis Method"),
		m.renderCostBasisToggle(),
		"",
		st.sectionTitle.Render("Data Provider"),
		m.renderSettingsInput("Provider:            ", m.providerInput, m.focused == settingFieldProvider),
		m.renderSettingsInput("Currency:            ", m.currencyInput, m.focused == settingFieldCurrency),
		"",
	}

	if m.status != "" {
		style := st.status
		if strings.HasPrefix(m.status, "Error:") || strings.HasPrefix(m.status, "Load error:") {
			style = st.errStyle
		}
		lines = append(lines, style.Render(m.status))
	}
	lines = append(lines, st.hint.Render("j/k: navigate · space: cycle cost basis · s/enter: save"))

	return strings.Join(lines, "\n")
}

func (m SettingsTabModel) renderSettingsInput(label string, inp textinput.Model, isFocused bool) string {
	st := m.styles
	lStyle := st.label
	if isFocused {
		lStyle = st.labelFocused
	}
	return lStyle.Render(label) + inp.View()
}

func (m SettingsTabModel) renderCostBasisToggle() string {
	st := m.styles
	isFocused := m.focused == settingFieldCostBasis
	lStyle := st.label
	if isFocused {
		lStyle = st.labelFocused
	}
	methods := []configuration.CostBasisMethod{
		configuration.CostBasisPMC,
		configuration.CostBasisFIFO,
		configuration.CostBasisLIFO,
	}
	parts := make([]string, len(methods))
	for i, method := range methods {
		if method == m.costBasis {
			parts[i] = st.toggleOn.Render(string(method))
		} else {
			parts[i] = st.toggleOff.Render(string(method))
		}
	}
	hint := ""
	if isFocused {
		hint = st.hint.Render(" (space: cycle)")
	}
	return lStyle.Render("Cost Basis:          ") + strings.Join(parts, " ") + hint
}
