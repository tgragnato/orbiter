package signal

import "testing"

func TestNewRuntime(t *testing.T) {
	t.Parallel()

	runtime := NewRuntime()
	if runtime.Dispatcher == nil {
		t.Fatalf("Dispatcher = nil")
	}
	if runtime.ReadModel == nil {
		t.Fatalf("ReadModel = nil")
	}
}
