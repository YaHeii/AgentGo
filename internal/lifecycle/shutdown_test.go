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
