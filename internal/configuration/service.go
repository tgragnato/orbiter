package configuration

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

const isoCurrencyCodeLen = 3

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
	_, err := s.GetYahooCredentials(ctx)
	if err != nil {
		return fmt.Errorf("validate %s: %w", KeyYahooCredentials, err)
	}

	_, err = s.GetBaseCurrency(ctx)
	if err != nil {
		return fmt.Errorf("validate %s: %w", KeyPortfolioBaseCurrency, err)
	}

	return nil
}

// GetBaseCurrency returns the ISO 4217 code used as the portfolio base currency.
// Falls back to "EUR" if the setting is absent.
func (s *Service) GetBaseCurrency(ctx context.Context) (string, error) {
	setting, err := getTyped[PortfolioBaseCurrencySetting](ctx, s.repo, KeyPortfolioBaseCurrency)
	if err != nil {
		return defaultBaseCurrency, nil //nolint:nilerr // missing setting is non-fatal; default is EUR
	}

	if setting.Currency == "" {
		return defaultBaseCurrency, nil
	}

	return setting.Currency, nil
}

// GetBrokerConfig returns the persisted TAA broker friction parameters.
// Falls back to the built-in defaults when the key is absent.
func (s *Service) GetBrokerConfig(ctx context.Context) (TAABrokerConfig, error) {
	cfg, err := getTyped[TAABrokerConfig](ctx, s.repo, KeyTAABrokerConfig)
	if err != nil {
		defaultVal, ok := defaultSettings[KeyTAABrokerConfig].value.(TAABrokerConfig)
		if !ok {
			return TAABrokerConfig{}, fmt.Errorf("%w: broker config default has wrong type", ErrInvalidSetting)
		}

		return defaultVal, nil
	}

	return cfg, nil
}

// GetYahooCredentials returns persisted Yahoo provider credentials.
func (s *Service) GetYahooCredentials(ctx context.Context) (YahooCredentialsSetting, error) {
	return getTyped[YahooCredentialsSetting](ctx, s.repo, KeyYahooCredentials)
}

// SetBaseCurrency updates the portfolio base currency.
func (s *Service) SetBaseCurrency(ctx context.Context, currency string) error {
	if len(currency) != isoCurrencyCodeLen {
		return fmt.Errorf("%w: base currency must be a 3-letter ISO 4217 code, got %q", ErrInvalidSetting, currency)
	}

	return setTyped(
		ctx,
		s.repo,
		KeyPortfolioBaseCurrency,
		defaultSettings[KeyPortfolioBaseCurrency].scope,
		defaultSettings[KeyPortfolioBaseCurrency].description,
		PortfolioBaseCurrencySetting{Currency: currency},
	)
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

	return setTyped(
		ctx,
		s.repo,
		KeyTAABrokerConfig,
		defaultSettings[KeyTAABrokerConfig].scope,
		defaultSettings[KeyTAABrokerConfig].description,
		value,
	)
}

// SetYahooCredentials updates Yahoo provider credentials.
func (s *Service) SetYahooCredentials(ctx context.Context, value YahooCredentialsSetting) error {
	yahooDefaults := defaultSettings[KeyYahooCredentials]

	return setTyped(ctx, s.repo, KeyYahooCredentials, yahooDefaults.scope, yahooDefaults.description, value)
}

//nolint:ireturn // generic return type; T is a concrete type parameter constrained to any
func getTyped[T any](ctx context.Context, repo Repository, key string) (T, error) {
	setting, err := repo.Get(ctx, key)
	if err != nil {
		var zero T

		return zero, fmt.Errorf("get setting %s: %w", key, err)
	}

	var value T

	err = json.Unmarshal(setting.ValueJSON, &value)
	if err != nil {
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

	err = repo.Set(ctx, Setting{
		Key:         key,
		Scope:       scope,
		Description: description,
		ValueJSON:   data,
		CreatedAt:   time.Time{},
		UpdatedAt:   time.Time{},
	})
	if err != nil {
		return fmt.Errorf("set setting %s: %w", key, err)
	}

	return nil
}
