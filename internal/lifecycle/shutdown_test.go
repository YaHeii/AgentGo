package lifecycle

import (
	"context"
	"errors"
	"testing"
)

func TestRuntimeCloseReturnsCloserError(t *testing.T) {
	want := errors.New("close failed")
	runtime := Runtime{
		closeFn: func(context.Context) error {
			return want
		},
	}

	err := runtime.GracefulShutdown(context.Background())
	if !errors.Is(err, want) {
		t.Fatalf("expected %v, got %v", want, err)
	}
}

func TestSupervisorCloseClosesMCPClients(t *testing.T) {
	first := &stubMCPClient{}
	second := &stubMCPClient{}
	supervisor := &Supervisor{
		mcpClients: []mcpClient{first, second},
	}

	if err := supervisor.Close(context.Background()); err != nil {
		t.Fatalf("close supervisor: %v", err)
	}

	if !first.closed || !second.closed {
		t.Fatalf("expected all MCP clients to be closed")
	}
}
