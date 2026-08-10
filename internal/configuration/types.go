package configuration

import "time"

// CostBasisMethod controls how realized PnL is computed from lots.
type CostBasisMethod string

const (
	// CostBasisPMC is pooled/weighted average cost (default).
	CostBasisPMC CostBasisMethod = "PMC"
	// CostBasisFIFO closes lots in first-in-first-out order.
	CostBasisFIFO CostBasisMethod = "FIFO"
	// CostBasisLIFO closes lots in last-in-first-out order.
	CostBasisLIFO CostBasisMethod = "LIFO"
)

const (
	// KeyCostBasisMethod stores the selected lot accounting mode.
	KeyCostBasisMethod = "cost_basis_method"
	// KeyDataProvider stores market data provider settings.
	KeyDataProvider = "data_provider"
	// KeyTAAParameters stores tactical allocation and signal parameters.
	KeyTAAParameters = "taa_parameters"
	// KeyCoreSatelliteTargets stores target core/satellite portfolio ratios.
	KeyCoreSatelliteTargets = "core_satellite_targets"
	// KeyTUIPreferences stores terminal UI preferences.
	KeyTUIPreferences = "tui_preferences"
	// KeyYahooCredentials stores Yahoo provider credentials/options.
	KeyYahooCredentials = "credentials.yahoo"
	// KeyPortfolioBaseCurrency stores the ISO 4217 currency code used as the
	// portfolio's base currency for NAV aggregation, TWR, and P&L reporting.
	KeyPortfolioBaseCurrency = "portfolio.base_currency"
)

// Setting is a key-value configuration row persisted in PostgreSQL.
type Setting struct {
	Key         string
	Scope       string
	Description string
	ValueJSON   []byte
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// CostBasisSetting persists cost basis policy.
type CostBasisSetting struct {
	Method CostBasisMethod `json:"method"`
}

// DataProviderSetting persists EOD provider preferences.
type DataProviderSetting struct {
	Provider string `json:"provider"`
	Currency string `json:"currency"`
}

// TAASetting persists tactical allocation controls.
type TAASetting struct {
	RebalanceThreshold float64 `json:"rebalance_threshold"`
}

// CoreSatelliteTargetSetting persists target allocation ratios.
type CoreSatelliteTargetSetting struct {
	CoreRatio      float64 `json:"core_ratio"`
	SatelliteRatio float64 `json:"satellite_ratio"`
}

// TUIPreferenceSetting persists terminal UI preferences.
type TUIPreferenceSetting struct {
	ShowPercentages bool   `json:"show_percentages"`
	NumberFormat    string `json:"number_format"`
}

// YahooCredentialsSetting persists data provider credentials.
type YahooCredentialsSetting struct {
	APIKey string `json:"api_key"`
}

// PortfolioBaseCurrencySetting stores the ISO 4217 base currency for the portfolio.
// All NAV snapshots, TWR, and aggregated P&L are expressed in this currency.
type PortfolioBaseCurrencySetting struct {
	Currency string `json:"currency"`
}

var defaultSettings = map[string]struct {
	scope       string
	description string
	value       any
}{
	KeyCostBasisMethod: {
		scope:       "global",
		description: "Default cost basis method",
		value:       CostBasisSetting{Method: CostBasisPMC},
	},
	KeyDataProvider: {
		scope:       "global",
		description: "Primary EOD data provider configuration",
		value: DataProviderSetting{
			Provider: "YAHOO",
			Currency: "EUR",
		},
	},
	KeyTAAParameters: {
		scope:       "strategy",
		description: "TAA strategy defaults",
		value: TAASetting{
			RebalanceThreshold: 0.05,
		},
	},
	KeyCoreSatelliteTargets: {
		scope:       "portfolio",
		description: "Core and satellite target allocation",
		value: CoreSatelliteTargetSetting{
			CoreRatio:      0.8,
			SatelliteRatio: 0.2,
		},
	},
	KeyTUIPreferences: {
		scope:       "tui",
		description: "Terminal UI defaults",
		value: TUIPreferenceSetting{
			ShowPercentages: true,
			NumberFormat:    "2dp",
		},
	},
	KeyYahooCredentials: {
		scope:       "credentials",
		description: "Yahoo Finance credentials",
		value:       YahooCredentialsSetting{APIKey: ""},
	},
	KeyPortfolioBaseCurrency: {
		scope:       "portfolio",
		description: "ISO 4217 base currency for NAV, TWR, and P&L aggregation",
		value:       PortfolioBaseCurrencySetting{Currency: "EUR"},
	},
}
