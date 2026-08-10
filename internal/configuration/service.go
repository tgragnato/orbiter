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
	if _, err := s.GetCostBasisMethod(ctx); err != nil {
		return fmt.Errorf("validate %s: %w", KeyCostBasisMethod, err)
	}
	if _, err := s.GetDataProvider(ctx); err != nil {
		return fmt.Errorf("validate %s: %w", KeyDataProvider, err)
	}
	if _, err := s.GetTAA(ctx); err != nil {
		return fmt.Errorf("validate %s: %w", KeyTAAParameters, err)
	}
	if _, err := s.GetCoreSatelliteTargets(ctx); err != nil {
		return fmt.Errorf("validate %s: %w", KeyCoreSatelliteTargets, err)
	}
	if _, err := s.GetTUIPreferences(ctx); err != nil {
		return fmt.Errorf("validate %s: %w", KeyTUIPreferences, err)
	}
	if _, err := s.GetYahooCredentials(ctx); err != nil {
		return fmt.Errorf("validate %s: %w", KeyYahooCredentials, err)
	}
	return nil
}

// GetCostBasisMethod returns the configured lot accounting method.
func (s *Service) GetCostBasisMethod(ctx context.Context) (CostBasisMethod, error) {
	setting, err := getTyped[CostBasisSetting](ctx, s.repo, KeyCostBasisMethod)
	if err != nil {
		return "", err
	}
	switch setting.Method {
	case CostBasisPMC, CostBasisFIFO, CostBasisLIFO:
		return setting.Method, nil
	default:
		return "", fmt.Errorf("%w: unsupported cost basis method %q", ErrInvalidSetting, setting.Method)
	}
}

// SetCostBasisMethod updates the configured lot accounting method.
func (s *Service) SetCostBasisMethod(ctx context.Context, method CostBasisMethod) error {
	switch method {
	case CostBasisPMC, CostBasisFIFO, CostBasisLIFO:
		return setTyped(ctx, s.repo, KeyCostBasisMethod, defaultSettings[KeyCostBasisMethod].scope, defaultSettings[KeyCostBasisMethod].description, CostBasisSetting{Method: method})
	default:
		return fmt.Errorf("%w: unsupported cost basis method %q", ErrInvalidSetting, method)
	}
}

// GetDataProvider returns provider and currency preferences.
func (s *Service) GetDataProvider(ctx context.Context) (DataProviderSetting, error) {
	return getTyped[DataProviderSetting](ctx, s.repo, KeyDataProvider)
}

// SetDataProvider updates provider and currency preferences.
func (s *Service) SetDataProvider(ctx context.Context, value DataProviderSetting) error {
	if value.Provider == "" {
		return fmt.Errorf("%w: provider is required", ErrInvalidSetting)
	}
	if value.Currency == "" {
		return fmt.Errorf("%w: currency is required", ErrInvalidSetting)
	}
	return setTyped(ctx, s.repo, KeyDataProvider, defaultSettings[KeyDataProvider].scope, defaultSettings[KeyDataProvider].description, value)
}

// GetTAA returns tactical allocation parameters.
func (s *Service) GetTAA(ctx context.Context) (TAASetting, error) {
	value, err := getTyped[TAASetting](ctx, s.repo, KeyTAAParameters)
	if err != nil {
		return TAASetting{}, err
	}
	if value.RebalanceThreshold <= 0 || value.RebalanceThreshold >= 1 {
		return TAASetting{}, fmt.Errorf("%w: rebalance_threshold must be in (0,1)", ErrInvalidSetting)
	}
	return value, nil
}

// SetTAA updates tactical allocation parameters.
func (s *Service) SetTAA(ctx context.Context, value TAASetting) error {
	if value.RebalanceThreshold <= 0 || value.RebalanceThreshold >= 1 {
		return fmt.Errorf("%w: rebalance_threshold must be in (0,1)", ErrInvalidSetting)
	}
	return setTyped(ctx, s.repo, KeyTAAParameters, defaultSettings[KeyTAAParameters].scope, defaultSettings[KeyTAAParameters].description, value)
}

// GetCoreSatelliteTargets returns target allocation ratios.
func (s *Service) GetCoreSatelliteTargets(ctx context.Context) (CoreSatelliteTargetSetting, error) {
	value, err := getTyped[CoreSatelliteTargetSetting](ctx, s.repo, KeyCoreSatelliteTargets)
	if err != nil {
		return CoreSatelliteTargetSetting{}, err
	}
	if err := validateTargets(value); err != nil {
		return CoreSatelliteTargetSetting{}, err
	}
	return value, nil
}

// SetCoreSatelliteTargets updates target allocation ratios.
func (s *Service) SetCoreSatelliteTargets(ctx context.Context, value CoreSatelliteTargetSetting) error {
	if err := validateTargets(value); err != nil {
		return err
	}
	return setTyped(ctx, s.repo, KeyCoreSatelliteTargets, defaultSettings[KeyCoreSatelliteTargets].scope, defaultSettings[KeyCoreSatelliteTargets].description, value)
}

// GetTUIPreferences returns persisted terminal UI settings.
func (s *Service) GetTUIPreferences(ctx context.Context) (TUIPreferenceSetting, error) {
	return getTyped[TUIPreferenceSetting](ctx, s.repo, KeyTUIPreferences)
}

// SetTUIPreferences updates terminal UI settings.
func (s *Service) SetTUIPreferences(ctx context.Context, value TUIPreferenceSetting) error {
	if value.NumberFormat == "" {
		return fmt.Errorf("%w: number_format is required", ErrInvalidSetting)
	}
	return setTyped(ctx, s.repo, KeyTUIPreferences, defaultSettings[KeyTUIPreferences].scope, defaultSettings[KeyTUIPreferences].description, value)
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

func validateTargets(value CoreSatelliteTargetSetting) error {
	if value.CoreRatio < 0 || value.SatelliteRatio < 0 {
		return fmt.Errorf("%w: target ratios must be >= 0", ErrInvalidSetting)
	}
	sum := value.CoreRatio + value.SatelliteRatio
	if sum < 0.999999 || sum > 1.000001 {
		return fmt.Errorf("%w: target ratios must sum to 1.0", ErrInvalidSetting)
	}
	return nil
}
