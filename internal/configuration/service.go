package configuration

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
)

var (
	// ErrSettingNotFound indicates a requested configuration key does not exist.
	ErrSettingNotFound = errors.New("configuration setting not found")
	// ErrInvalidSetting indicates malformed or invalid configuration data.
	ErrInvalidSetting = errors.New("invalid configuration setting")
)

// Service exposes typed accessors on top of configuration repository storage.
type Service struct {
	repo Repository
}

// NewService creates a typed configuration service.
func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

// ValidateRequired ensures all mandatory settings are present and valid.
func (s *Service) ValidateRequired(ctx context.Context) error {
	if _, err := s.GetYahooCredentials(ctx); err != nil {
		return fmt.Errorf("validate %s: %w", KeyYahooCredentials, err)
	}
	if _, err := s.GetBaseCurrency(ctx); err != nil {
		return fmt.Errorf("validate %s: %w", KeyPortfolioBaseCurrency, err)
	}
	return nil
}

// GetYahooCredentials returns persisted Yahoo provider credentials.
func (s *Service) GetYahooCredentials(ctx context.Context) (YahooCredentialsSetting, error) {
	return getTyped[YahooCredentialsSetting](ctx, s.repo, KeyYahooCredentials)
}

// SetYahooCredentials updates Yahoo provider credentials.
func (s *Service) SetYahooCredentials(ctx context.Context, value YahooCredentialsSetting) error {
	return setTyped(ctx, s.repo, KeyYahooCredentials, defaultSettings[KeyYahooCredentials].scope, defaultSettings[KeyYahooCredentials].description, value)
}

// GetBaseCurrency returns the ISO 4217 code used as the portfolio base currency.
// Falls back to "EUR" if the setting is absent.
func (s *Service) GetBaseCurrency(ctx context.Context) (string, error) {
	setting, err := getTyped[PortfolioBaseCurrencySetting](ctx, s.repo, KeyPortfolioBaseCurrency)
	if err != nil {
		return "EUR", nil //nolint:nilerr // missing setting is non-fatal; default is EUR
	}
	if setting.Currency == "" {
		return "EUR", nil
	}
	return setting.Currency, nil
}

// SetBaseCurrency updates the portfolio base currency.
func (s *Service) SetBaseCurrency(ctx context.Context, currency string) error {
	if len(currency) != 3 {
		return fmt.Errorf("%w: base currency must be a 3-letter ISO 4217 code, got %q", ErrInvalidSetting, currency)
	}
	return setTyped(ctx, s.repo, KeyPortfolioBaseCurrency,
		defaultSettings[KeyPortfolioBaseCurrency].scope,
		defaultSettings[KeyPortfolioBaseCurrency].description,
		PortfolioBaseCurrencySetting{Currency: currency},
	)
}

// GetBrokerConfig returns the persisted TAA broker friction parameters.
// Falls back to the built-in defaults when the key is absent.
func (s *Service) GetBrokerConfig(ctx context.Context) (TAABrokerConfig, error) {
	cfg, err := getTyped[TAABrokerConfig](ctx, s.repo, KeyTAABrokerConfig)
	if err != nil {
		d := defaultSettings[KeyTAABrokerConfig].value.(TAABrokerConfig)
		return d, nil //nolint:nilerr // absent key → use defaults
	}
	return cfg, nil
}

// SetBrokerConfig validates and persists the TAA broker friction parameters.
// All rate fields must be in [0, 1]; MaxBrokerFee must be >= 0.
func (s *Service) SetBrokerConfig(ctx context.Context, value TAABrokerConfig) error {
	if value.TaxRate < 0 || value.TaxRate > 1 {
		return fmt.Errorf("%w: tax_rate must be in [0, 1], got %g", ErrInvalidSetting, value.TaxRate)
	}
	if value.BrokerFeePercent < 0 || value.BrokerFeePercent > 1 {
		return fmt.Errorf("%w: broker_fee_percent must be in [0, 1], got %g", ErrInvalidSetting, value.BrokerFeePercent)
	}
	if value.MaxBrokerFee < 0 {
		return fmt.Errorf("%w: max_broker_fee must be >= 0, got %g", ErrInvalidSetting, value.MaxBrokerFee)
	}
	if value.Buffer < 0 || value.Buffer > 1 {
		return fmt.Errorf("%w: buffer must be in [0, 1], got %g", ErrInvalidSetting, value.Buffer)
	}
	return setTyped(ctx, s.repo, KeyTAABrokerConfig,
		defaultSettings[KeyTAABrokerConfig].scope,
		defaultSettings[KeyTAABrokerConfig].description,
		value,
	)
}

func getTyped[T any](ctx context.Context, repo Repository, key string) (T, error) {
	setting, err := repo.Get(ctx, key)
	if err != nil {
		var zero T
		return zero, err
	}

	var value T
	if err := json.Unmarshal(setting.ValueJSON, &value); err != nil {
		var zero T
		return zero, fmt.Errorf("%w: key %s cannot decode JSON: %w", ErrInvalidSetting, key, err)
	}

	return value, nil
}

func setTyped(ctx context.Context, repo Repository, key, scope, description string, value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("marshal setting %s: %w", key, err)
	}

	return repo.Set(ctx, Setting{
		Key:         key,
		Scope:       scope,
		Description: description,
		ValueJSON:   data,
	})
}
