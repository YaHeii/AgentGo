package provider

import (
	"context"
	"testing"
)

func TestStreamingLLMContractUsesRequest(t *testing.T) {
	t.Parallel()

	var llm StreamingLLM = stubStreamingLLM{}
	req := Request{
		Messages: []Message{
			{
				Role:    "user",
				Content: "hello",
			},
		},
	}

	stream := llm.StreamChat(context.Background(), req)
	if stream == nil {
		t.Fatal("expected request-based stream channel")
	}
}

func TestRequestCarriesMessages(t *testing.T) {
	t.Parallel()

	req := Request{
		Messages: []Message{
			{
				Role:    "user",
				Content: "hello",
			},
		},
	}

	if len(req.Messages) != 1 {
		t.Fatalf("expected one message, got %d", len(req.Messages))
	}
	if req.Messages[0].Role != "user" {
		t.Fatalf("unexpected role: %q", req.Messages[0].Role)
	}
}

func TestStreamEventTypesAreStable(t *testing.T) {
	t.Parallel()

	cases := map[StreamEventType]string{
		StreamEventTextDelta:         "text_delta",
		StreamEventReasoningDelta:    "reasoning_delta",
		StreamEventRefusalDelta:      "refusal_delta",
		StreamEventToolCallDelta:     "tool_call_delta",
		StreamEventToolCallCompleted: "tool_call_completed",
		StreamEventUsageAvailable:    "usage_available",
		StreamEventTurnFinished:      "turn_finished",
		StreamEventProviderError:     "provider_error",
	}

	for got, want := range cases {
		if string(got) != want {
			t.Fatalf("unexpected stream event type: got %q want %q", got, want)
		}
	}
}

type stubStreamingLLM struct{}

func (stubStreamingLLM) StreamChat(_ context.Context, _ Request) <-chan StreamEvent {
	return make(chan StreamEvent)
}
