package openai

import (
	"github.com/YaHeii/agentGo/internal/provider"
	"strings"
	"testing"
)

func TestClientImplementsStreamingLLM(t *testing.T) {
	t.Parallel()

	var _ provider.StreamingLLM = (*Client)(nil)
}

func TestNewRequiresAPIKey(t *testing.T) {
	t.Parallel()

	_, err := New(Config{Model: "gpt-test"})
	if err == nil {
		t.Fatal("expected missing api key error")
	}
	if !strings.Contains(err.Error(), "api key") {
		t.Fatalf("expected api key error, got %v", err)
	}
}

func TestNewRequiresModel(t *testing.T) {
	t.Parallel()

	_, err := New(Config{APIKey: "test-key"})
	if err == nil {
		t.Fatal("expected missing model error")
	}
	if !strings.Contains(err.Error(), "model") {
		t.Fatalf("expected model error, got %v", err)
	}
}
