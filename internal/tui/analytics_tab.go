package tui

import (
	"context"
	"fmt"
	"math"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/guptarohit/asciigraph"
	"github.com/tgragnato/orbiter/internal/ml"
	"github.com/tgragnato/orbiter/internal/portfolio/analytics"
)

type analyticsLoadedMsg struct {
	twrData      []float64
	sortinoData  []float64
	drawdownData []float64
	// Summary scalars
	totalReturn    float64
	annualizedRet  float64
	maxDrawdown    float64
	currentSortino float64
	// Actual data window
	dataFrom time.Time
	dataTo   time.Time
	err      error
}

type AnalyticsTabModel struct {
	engine      *analytics.TWREngine
	portfolioID string

	twrData      []float64
	sortinoData  []float64
	drawdownData []float64

	totalReturn    float64
	annualizedRet  float64
	maxDrawdown    float64
	currentSortino float64
	dataFrom       time.Time
	dataTo         time.Time

	loading bool
	err     error
	width   int
	height  int
	quit    bool

	styles lipgloss.Style
}

func NewAnalyticsTabModel(engine *analytics.TWREngine, portfolioID string) AnalyticsTabModel {
	return AnalyticsTabModel{
		engine:      engine,
		portfolioID: portfolioID,
		loading:     true,
		styles:      lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39")),
	}
}

func (m AnalyticsTabModel) Init() tea.Cmd {
	if m.engine == nil {
		return nil
	}
	return m.loadDataCmd()
}

func (m AnalyticsTabModel) loadDataCmd() tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()

		// Use a far-past From so every snapshot ever recorded is included.
		// The actual visible range is derived from the returned periods.
		tr := analytics.TimeRange{
			From: time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC),
			To:   time.Now().UTC(),
		}

		result, err := m.engine.CalculateTWR(ctx, m.portfolioID, tr)
		if err != nil {
			return analyticsLoadedMsg{err: err}
		}
		if len(result.Periods) == 0 {
			return analyticsLoadedMsg{}
		}

		var (
			twrSeries      []float64
			sortinoSeries  []float64
			drawdownSeries []float64
			returns        []float64
		)

		cumulativeTWR := 1.0
		peakTWR := 1.0

		for _, p := range result.Periods {
			cumulativeTWR *= (1 + p.Return)
			twrPct := (cumulativeTWR - 1) * 100
			twrSeries = append(twrSeries, twrPct)

			if cumulativeTWR > peakTWR {
				peakTWR = cumulativeTWR
			}
			dd := 0.0
			if peakTWR > 0 {
				dd = (cumulativeTWR - peakTWR) / peakTWR * 100
			}
			drawdownSeries = append(drawdownSeries, dd)

			returns = append(returns, p.Return)
			sortinoSeries = append(sortinoSeries, ml.Sortino(returns))
		}

		totalReturn := cumulativeTWR - 1
		maxDD := 0.0
		for _, d := range drawdownSeries {
			if d < maxDD {
				maxDD = d
			}
		}

		dataFrom := result.Periods[0].StartAt
		dataTo := result.Periods[len(result.Periods)-1].EndAt
		years := dataTo.Sub(dataFrom).Hours() / (365.25 * 24)
		annualized := 0.0
		if years > 0 && cumulativeTWR > 0 {
			annualized = math.Pow(cumulativeTWR, 1.0/years) - 1
		}

		currentSortino := 0.0
		if len(returns) > 0 {
			currentSortino = ml.Sortino(returns)
		}

		return analyticsLoadedMsg{
			twrData:        twrSeries,
			sortinoData:    sortinoSeries,
			drawdownData:   drawdownSeries,
			totalReturn:    totalReturn,
			annualizedRet:  annualized,
			maxDrawdown:    maxDD,
			currentSortino: currentSortino,
			dataFrom:       dataFrom,
			dataTo:         dataTo,
		}
	}
}

func (m AnalyticsTabModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if m.quit {
		return m, nil
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "r":
			m.loading = true
			return m, m.loadDataCmd()
		}
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	case analyticsLoadedMsg:
		m.loading = false
		m.err = msg.err
		m.twrData = msg.twrData
		m.sortinoData = msg.sortinoData
		m.drawdownData = msg.drawdownData
		m.totalReturn = msg.totalReturn
		m.annualizedRet = msg.annualizedRet
		m.maxDrawdown = msg.maxDrawdown
		m.currentSortino = msg.currentSortino
		m.dataFrom = msg.dataFrom
		m.dataTo = msg.dataTo
	}
	return m, nil
}

func (m AnalyticsTabModel) View() string {
	if m.quit {
		return ""
	}

	dateRange := "All Time"
	if !m.dataFrom.IsZero() && !m.dataTo.IsZero() {
		dateRange = fmt.Sprintf("%s → %s",
			m.dataFrom.Format("2006-01-02"),
			m.dataTo.Format("2006-01-02"),
		)
	}
	title := m.styles.Render(fmt.Sprintf("Tab 6 - Portfolio Analytics (%s)", dateRange))

	if m.loading {
		return lipgloss.JoinVertical(lipgloss.Left, title, "\n  Loading analytics from database...")
	}
	if m.err != nil {
		return lipgloss.JoinVertical(lipgloss.Left, title, fmt.Sprintf("\n  Error: %v", m.err))
	}
	if len(m.twrData) < 2 {
		return lipgloss.JoinVertical(lipgloss.Left, title, "\n  Not enough NAV snapshots to compute analytics.\n  NAV history is being built in the background — check back soon.")
	}

	graphWidth := max(m.width-15, 20)
	// Reserve space for title + stats bar + 3 chart areas + hint
	chartRows := max((m.height-14)/3, 4)

	// Summary stats bar
	statStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("252")).Background(lipgloss.Color("24")).Padding(0, 1)
	stats := statStyle.Render(fmt.Sprintf(
		"TWR: %+.2f%%  |  CAGR: %+.2f%%  |  Max DD: %.2f%%  |  Sortino: %.2f",
		m.totalReturn*100,
		m.annualizedRet*100,
		m.maxDrawdown,
		m.currentSortino,
	))

	twrChart := asciigraph.Plot(m.twrData,
		asciigraph.Height(chartRows),
		asciigraph.Width(graphWidth),
		asciigraph.Caption("Cumulative TWR (%)"),
	)

	ddChart := asciigraph.Plot(m.drawdownData,
		asciigraph.Height(chartRows),
		asciigraph.Width(graphWidth),
		asciigraph.Caption("Drawdown from Peak (%)"),
	)

	sortinoChart := asciigraph.Plot(m.sortinoData,
		asciigraph.Height(chartRows),
		asciigraph.Width(graphWidth),
		asciigraph.Caption("Rolling Sortino Ratio"),
	)

	hint := lipgloss.NewStyle().Foreground(lipgloss.Color("242")).Render("  r: reload data")

	return lipgloss.JoinVertical(lipgloss.Left,
		title,
		stats,
		"\n", twrChart,
		"\n", ddChart,
		"\n", sortinoChart,
		hint,
	)
}
