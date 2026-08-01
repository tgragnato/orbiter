package app

import (
	"reflect"
	"testing"
)

func TestSplitCSV(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want []string
	}{
		{name: "empty", in: "", want: nil},
		{name: "spaces only", in: "   ", want: nil},
		{name: "trimmed entries", in: "a, b,  c ,, ", want: []string{"a", "b", "c"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := splitCSV(tt.in)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("splitCSV(%q) = %#v, want %#v", tt.in, got, tt.want)
			}
		})
	}
}

func TestParseBacktestingFlagsDefaults(t *testing.T) {
	conf, err := parseBacktestingFlags(nil)
	if err != nil {
		t.Fatalf("parseBacktestingFlags() error = %v", err)
	}

	if conf.debug {
		t.Fatalf("debug = true, want false")
	}
	if conf.priceSource != "LOCAL_DB" {
		t.Fatalf("priceSource = %q, want LOCAL_DB", conf.priceSource)
	}
	if conf.dbDSN != defaultPostgresDSN {
		t.Fatalf("dbDSN = %q, want %q", conf.dbDSN, defaultPostgresDSN)
	}
	if conf.priceDBDSN != defaultPostgresDSN {
		t.Fatalf("priceDBDSN = %q, want %q", conf.priceDBDSN, defaultPostgresDSN)
	}
	if conf.instrument != "CS.D.EURUSD.MINI.IP" {
		t.Fatalf("instrument = %q, want CS.D.EURUSD.MINI.IP", conf.instrument)
	}
	if conf.strategyName != "rsi" {
		t.Fatalf("strategyName = %q, want rsi", conf.strategyName)
	}
	if conf.candleDuration != "60m" {
		t.Fatalf("candleDuration = %q, want 60m", conf.candleDuration)
	}
	if conf.yearFrom != 1970 || conf.yearTo != 2022 || conf.monthFrom != 1 || conf.monthTo != 12 {
		t.Fatalf("date bounds = %#v, want defaults", conf)
	}
}

func TestParseHistdataFlags(t *testing.T) {
	conf, csvFiles, instrument, err := parseHistdataFlags([]string{"--import-histdata-csv-files", "a.csv,b.csv", "--instrument", "EURUSD", "--db-dsn", "postgres://example", "--debug"})
	if err != nil {
		t.Fatalf("parseHistdataFlags() error = %v", err)
	}

	if !conf.debug {
		t.Fatalf("debug = false, want true")
	}
	if conf.dbDSN != "postgres://example" {
		t.Fatalf("dbDSN = %q, want postgres://example", conf.dbDSN)
	}
	if csvFiles != "a.csv,b.csv" {
		t.Fatalf("csvFiles = %q, want a.csv,b.csv", csvFiles)
	}
	if instrument != "EURUSD" {
		t.Fatalf("instrument = %q, want EURUSD", instrument)
	}
}
