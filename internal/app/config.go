package app

type backtestingConfig struct {
	debug                  bool
	gatherPerformanceData  bool
	importHistDataCSVFiles []string
	priceSource            string
	dbDSN                  string
	priceDBDSN             string
	instrument             string
	broker                 string
	strategyName           string
	candleDuration         string
	yearFrom               int
	monthFrom              int
	yearTo                 int
	monthTo                int
}

type igConfig struct {
	debug                  bool
	gatherPerformanceData  bool
	importHistDataCSVFiles []string
	instrument             string
	currencyCode           string
	broker                 string
	strategyName           string
	candleDuration         string
	igAPIURL               string
	igIdentifier           string
	igAPIKey               string
	igPassword             string
	igAccountID            string
	dbDSN                  string
	yearFrom               int
	yearTo                 int
}

type histdataConfig struct {
	debug bool
	dbDSN string
}

type coinbaseConfig struct {
	debug          bool
	instrument     string
	candleDuration string
	dbDSN          string
}
