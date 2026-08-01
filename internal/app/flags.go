package app

import (
	"flag"
	"strings"
)

const defaultPostgresDSN = "postgres://postgres:postgres@localhost:5432/at?sslmode=disable"

func parseBacktestingFlags(args []string) (backtestingConfig, error) {
	var conf backtestingConfig
	var csvFiles string

	fs := flag.NewFlagSet("backtesting", flag.ContinueOnError)
	fs.BoolVar(&conf.debug, "debug", false, "Enable debug logging")
	fs.BoolVar(&conf.gatherPerformanceData, "performance-data", false, "Gather performance data and print as CSV")
	fs.StringVar(&csvFiles, "import-histdata-csv-files", "", "Comma-separated histdata.com CSV files to import instead of reading from the DB")
	fs.StringVar(&conf.priceSource, "price-source", "LOCAL_DB", "Price source for backtesting. E.g. COINBASE")
	fs.StringVar(&conf.dbDSN, "db-dsn", defaultPostgresDSN, "PostgreSQL DSN for the backtesting result database")
	fs.StringVar(&conf.priceDBDSN, "price-db-dsn", defaultPostgresDSN, "PostgreSQL DSN for historical OHLC price data")
	fs.StringVar(&conf.instrument, "instrument", "CS.D.EURUSD.MINI.IP", "Instrument to trade")
	fs.StringVar(&conf.broker, "broker", "backtest", "Broker backend")
	fs.StringVar(&conf.strategyName, "strategy", "rsi", "Strategy to be executed")
	fs.StringVar(&conf.candleDuration, "candle-duration", "60m", "Duration for OHLC candle")
	fs.IntVar(&conf.yearFrom, "year-from", 1970, "Backtesting beginning year")
	fs.IntVar(&conf.yearTo, "year-to", 2022, "Backtesting end year")
	fs.IntVar(&conf.monthFrom, "month-from", 1, "Backtesting beginning month")
	fs.IntVar(&conf.monthTo, "month-to", 12, "Backtesting end month")

	if err := fs.Parse(args); err != nil {
		return conf, err
	}
	conf.importHistDataCSVFiles = splitCSV(csvFiles)
	return conf, nil
}

func parseIGFlags(args []string) (igConfig, error) {
	var conf igConfig
	var csvFiles string

	fs := flag.NewFlagSet("at-ig", flag.ContinueOnError)
	fs.BoolVar(&conf.debug, "debug", false, "Enable debug logging")
	fs.BoolVar(&conf.gatherPerformanceData, "performance-data", false, "Gather performance data and print as CSV")
	fs.StringVar(&csvFiles, "import-histdata-csv-files", "", "Comma-separated histdata.com CSV files to import instead of reading from the DB")
	fs.StringVar(&conf.instrument, "instrument", "CS.D.EURUSD.MINI.IP", "Instrument to trade")
	fs.StringVar(&conf.currencyCode, "currency-code", "EUR", "Currency code")
	fs.StringVar(&conf.broker, "broker", "none", "Broker backend")
	fs.StringVar(&conf.strategyName, "strategy", "rsi", "Strategy to be executed")
	fs.StringVar(&conf.candleDuration, "candle-duration", "60m", "Duration for OHLC candle")
	fs.StringVar(&conf.igAPIURL, "ig-api-url", "https://demo-api.ig.com/gateway/deal", "IG API URL")
	fs.StringVar(&conf.igIdentifier, "ig-identifier", "", "IG Identifier")
	fs.StringVar(&conf.igAPIKey, "ig-api-key", "", "IG API key")
	fs.StringVar(&conf.igPassword, "ig-password", "", "IG password")
	fs.StringVar(&conf.igAccountID, "ig-account", "", "IG account ID")
	fs.StringVar(&conf.dbDSN, "db-dsn", defaultPostgresDSN, "PostgreSQL DSN for persistence")
	fs.IntVar(&conf.yearFrom, "year-from", 1970, "Backtesting beginning")
	fs.IntVar(&conf.yearTo, "year-to", 2022, "Backtesting end")

	if err := fs.Parse(args); err != nil {
		return conf, err
	}
	conf.importHistDataCSVFiles = splitCSV(csvFiles)
	return conf, nil
}

func parseHistdataFlags(args []string) (histdataConfig, string, string, error) {
	var conf histdataConfig
	var csvFiles string
	var instrument string

	fs := flag.NewFlagSet("import-histdata", flag.ContinueOnError)
	fs.StringVar(&csvFiles, "import-histdata-csv-files", "", "Comma-separated CSV files to import from histdata.com")
	fs.StringVar(&instrument, "instrument", "EURUSD", "Instrument name e.g. EURUSD")
	fs.StringVar(&conf.dbDSN, "db-dsn", defaultPostgresDSN, "PostgreSQL DSN for the price database")
	fs.BoolVar(&conf.debug, "debug", false, "Enable debug logging")

	if err := fs.Parse(args); err != nil {
		return conf, "", "", err
	}
	return conf, csvFiles, instrument, nil
}

func parseCoinbaseFlags(args []string) (coinbaseConfig, error) {
	var conf coinbaseConfig

	fs := flag.NewFlagSet("at-coinbase", flag.ContinueOnError)
	fs.BoolVar(&conf.debug, "debug", false, "Enable debug logging")
	fs.StringVar(&conf.instrument, "instrument", "BTC-USD", "Instrument to trade")
	fs.StringVar(&conf.candleDuration, "candle-duration", "1m", "Duration for OHLC candle")
	fs.StringVar(&conf.dbDSN, "db-dsn", defaultPostgresDSN, "PostgreSQL DSN for persistence")

	if err := fs.Parse(args); err != nil {
		return conf, err
	}
	return conf, nil
}

func splitCSV(value string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}
