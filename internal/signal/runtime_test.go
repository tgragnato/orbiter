package signal_test

import (
	"testing"

	"github.com/tgragnato/orbiter/internal/signal"
)

func TestNewRuntime(t *testing.T) {
	t.Parallel()

	runtime := signal.NewRuntime()

	if runtime.Dispatcher == nil {
		t.Fatalf("Dispatcher = nil")
	}

	if runtime.ReadModel == nil {
		t.Fatalf("ReadModel = nil")
	}
}
