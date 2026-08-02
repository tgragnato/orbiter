package signal

// ReadModel exposes queued signal messages for TUI consumers.
type ReadModel interface {
	Pending() []Message
	Drain() []Message
}

// NewReadModel creates a queue read model over a dispatcher.
func NewReadModel(dispatcher Dispatcher) ReadModel {
	return readModel{dispatcher: dispatcher}
}

type readModel struct {
	dispatcher Dispatcher
}

func (r readModel) Pending() []Message {
	if q, ok := r.dispatcher.(interface{ Messages() []Message }); ok {
		return q.Messages()
	}
	return nil
}

func (r readModel) Drain() []Message {
	if q, ok := r.dispatcher.(interface{ Drain() []Message }); ok {
		return q.Drain()
	}
	return nil
}
