package signal

// Runtime bundles write/read queue components for signal-driven UI flows.
type Runtime struct {
	Dispatcher Dispatcher
	ReadModel  ReadModel
}

// NewRuntime creates a shared signal queue and read model for trader plus TUI.
func NewRuntime() Runtime {
	dispatcher := NewMemoryDispatcher()

	return Runtime{
		Dispatcher: dispatcher,
		ReadModel:  NewReadModel(dispatcher),
	}
}
