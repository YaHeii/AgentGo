package provider

import (
	"context"
	"errors"

	"github.com/YaHeii/agentGo/internal/app"
)

type ProviderService struct {
	client     streamClient
	dispatcher app.Dispatcher
}

func NewProviderService(client streamClient, d app.Dispatcher) *ProviderService {
	return &ProviderService{
		client:     client,
		dispatcher: d,
	}
}

func (s *ProviderService) RunTurn(ctx context.Context, req Request) (TurnResult, error) {
	if len(req.Messages) == 0 {
		return TurnResult{}, errors.New("provider: messages cannot be empty")
	}
	if s.client == nil {
		return TurnResult{}, errors.New("provider: stream client is required")
	}

	var (
		result      TurnResult
		accumulator toolCallAccumulator
	)

	stream := s.client.Stream(ctx, req)
	for event := range stream {
		s.publish(event)

		switch event.Type {
		case StreamEventTextDelta:
			result.Text += event.TextDelta
			result.SystemFingerprint = preferFingerprint(result.SystemFingerprint, event.SystemFingerprint)
		case StreamEventReasoningDelta:
			result.Reasoning += event.ReasoningDelta
			result.SystemFingerprint = preferFingerprint(result.SystemFingerprint, event.SystemFingerprint)
		case StreamEventRefusalDelta:
			result.Refusal += event.RefusalDelta
			result.SystemFingerprint = preferFingerprint(result.SystemFingerprint, event.SystemFingerprint)
		case StreamEventToolCallDelta:
			if event.ToolCallDelta != nil {
				accumulator.Apply(*event.ToolCallDelta)
			}
			result.SystemFingerprint = preferFingerprint(result.SystemFingerprint, event.SystemFingerprint)
		case StreamEventUsageAvailable:
			result.Usage = event.Usage
			result.SystemFingerprint = preferFingerprint(result.SystemFingerprint, event.SystemFingerprint)
		case StreamEventTurnFinished:
			result.StopReason = event.StopReason
			result.SystemFingerprint = preferFingerprint(result.SystemFingerprint, event.SystemFingerprint)
			result.ToolCalls = accumulator.Completed()
			return result, nil
		case StreamEventProviderError:
			result.ToolCalls = accumulator.Completed()
			result.SystemFingerprint = preferFingerprint(result.SystemFingerprint, event.SystemFingerprint)
			if event.Err != nil {
				return result, event.Err
			}
			return result, errors.New("provider: stream failed")
		}

		if err := ctx.Err(); err != nil {
			result.ToolCalls = accumulator.Completed()
			return result, err
		}
	}

	result.ToolCalls = accumulator.Completed()
	if err := ctx.Err(); err != nil {
		return result, err
	}
	return result, errors.New("provider: stream ended without turn_finished")
}

func (s *ProviderService) publish(event StreamEvent) {
	if s.dispatcher == nil {
		return
	}
	s.dispatcher.Dispatch(app.BaseEvent{
		T:       app.EventProvider,
		Payload: event,
	})
}

func preferFingerprint(current string, next string) string {
	if next != "" {
		return next
	}
	return current
}

type toolCallAccumulator struct {
	calls map[int]ToolCall
	order []int
}

func (a *toolCallAccumulator) Apply(delta ToolCallDelta) {
	if a.calls == nil {
		a.calls = make(map[int]ToolCall)
	}

	call, ok := a.calls[delta.Index]
	if !ok {
		call = ToolCall{Index: delta.Index}
		a.order = append(a.order, delta.Index)
	}
	if delta.ID != "" {
		call.ID = delta.ID
	}
	if delta.NameDelta != "" {
		call.Name += delta.NameDelta
	}
	if delta.ArgumentsDelta != "" {
		call.Arguments += delta.ArgumentsDelta
	}
	a.calls[delta.Index] = call
}

func (a *toolCallAccumulator) Completed() []ToolCall {
	if len(a.order) == 0 {
		return nil
	}

	out := make([]ToolCall, 0, len(a.order))
	for _, index := range a.order {
		out = append(out, a.calls[index])
	}
	return out
}

var _ TurnRunner = (*ProviderService)(nil)

func (r Request) Validate() error {
	if len(r.Messages) == 0 {
		return errors.New("provider: messages cannot be empty")
	}
	return nil
}
