package configuration

import "time"

const (
	// KeyYahooCredentials stores Yahoo provider credentials/options.
	KeyYahooCredentials = "credentials.yahoo"
	// KeyPortfolioBaseCurrency stores the ISO 4217 currency code used as the
	// portfolio's base currency for NAV aggregation, TWR, and P&L reporting.
	KeyPortfolioBaseCurrency = "portfolio.base_currency"
	// KeyTAABrokerConfig stores the TAA engine's fiscal friction parameters
	// (tax rate, broker fee, cap, and safety buffer).
	KeyTAABrokerConfig = "taa.broker_config"

	// defaultBaseCurrency is the default ISO 4217 base currency for the portfolio.
	defaultBaseCurrency = "EUR"

	// defaultTaxRate is the default effective capital-gains tax rate (26 %).
	defaultTaxRate = 0.26
	// defaultBrokerFeePercent is the default broker transaction cost as a fraction (0.19 %).
	defaultBrokerFeePercent = 0.0019
	// defaultMaxBrokerFee caps the default broker fee in base-currency units (18.90 EUR).
	defaultMaxBrokerFee = 18.90
	// defaultBuffer is the default safety buffer above taxes+fees required before a trade is signalled.
	defaultBuffer = 0.01
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

// YahooCredentialsSetting persists data provider credentials.
type YahooCredentialsSetting struct {
	APIKey string `json:"api_key"` //nolint:tagliatelle // snake_case JSON in DB
}

// PortfolioBaseCurrencySetting stores the ISO 4217 base currency for the portfolio.
// All NAV snapshots, TWR, and aggregated P&L are expressed in this currency.
type PortfolioBaseCurrencySetting struct {
	Currency string `json:"currency"`
}

// TAABrokerConfig holds the TAA engine's fiscal friction parameters.
// All rate fields are stored as decimals (e.g. 0.26 for 26 %).
type TAABrokerConfig struct {
	// TaxRate is the effective capital-gains tax rate (e.g. 0.26 for 26 %).
	TaxRate float64 `json:"tax_rate"` //nolint:tagliatelle // snake_case JSON in DB
	// BrokerFeePercent is the broker transaction cost as a fraction (e.g. 0.0019 for 0.19 %).
	BrokerFeePercent float64 `json:"broker_fee_percent"` //nolint:tagliatelle // snake_case JSON in DB
	// MaxBrokerFee caps the broker fee in absolute base-currency units (e.g. 18.90 EUR).
	// Zero means no cap.
	MaxBrokerFee float64 `json:"max_broker_fee"` //nolint:tagliatelle // snake_case JSON in DB
	// Buffer is an additional threshold above taxes+fees required before a trade is signalled.
	Buffer float64 `json:"buffer"`
}

//nolint:gochecknoglobals // package-level default settings map is intentional; effectively read-only after init
var defaultSettings = map[string]struct {
	scope       string
	description string
	value       any
}{
	KeyYahooCredentials: {
		scope:       "credentials",
		description: "Yahoo Finance credentials",
		value:       YahooCredentialsSetting{APIKey: ""},
	},
	KeyPortfolioBaseCurrency: {
		scope:       "portfolio",
		description: "ISO 4217 base currency for NAV, TWR, and P&L aggregation",
		value:       PortfolioBaseCurrencySetting{Currency: defaultBaseCurrency},
	},
	KeyTAABrokerConfig: {
		scope:       "taa",
		description: "TAA engine fiscal friction parameters",
		value: TAABrokerConfig{
			TaxRate:          defaultTaxRate,
			BrokerFeePercent: defaultBrokerFeePercent,
			MaxBrokerFee:     defaultMaxBrokerFee,
			Buffer:           defaultBuffer,
		},
	},
}
