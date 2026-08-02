package tui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/tgragnato/orbiter/internal/portfolio"
)

type programRunner interface {
	Run() (tea.Model, error)
}

var newProgram = func(model tea.Model, options ...tea.ProgramOption) programRunner {
	return tea.NewProgram(model, options...)
}

// Run starts the interactive signals TUI.
func Run(store portfolio.HoldingsStore) error {
	p := newProgram(NewModel(store), tea.WithAltScreen())
	_, err := p.Run()
	return err
}
