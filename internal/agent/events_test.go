package agent

import (
	"errors"
	"testing"

	agentcontract "github.com/YaHeii/agentGo/internal/agent/contract"
	"github.com/YaHeii/agentGo/internal/app"
	messagecontract "github.com/YaHeii/agentGo/internal/message/contract"
	providercontract "github.com/YaHeii/agentGo/internal/provider/contract"
	"github.com/stretchr/testify/require"
)

func TestNewQueryAppEventMapsStatusAndCopiesState(t *testing.T) {
	t.Parallel()

	state := LoopState{
		Messages: []messagecontract.Message{
			{
				ID: "message-1",
				Parts: []messagecontract.Part{
					{Type: messagecontract.PartTypeText, Text: "hello"},
				},
			},
		},
		TurnCount:          2,
		Transition:         "looping",
		LoopStatus:         agentcontract.LoopAwaitingTool,
		ProviderStopReason: providercontract.StopReasonStop,
		PendingToolCalls: []providercontract.ToolCall{
			{Index: 0, Name: "grep"},
		},
	}

	evt := NewQueryAppEvent(agentcontract.LoopAwaitingTool, state, errors.New("boom"))
	require.Equal(t, app.EventAgent, evt.Type())
	require.Equal(t, app.EventAgentLoopAwaitingTool, evt.Name())

	payload, ok := evt.Data().(QueryEvent)
	require.True(t, ok)
	require.Equal(t, agentcontract.LoopAwaitingTool, payload.Status)
	require.Equal(t, 2, payload.State.TurnCount)
	require.Equal(t, "looping", payload.State.Transition)
	require.Len(t, payload.State.Messages, 1)
	require.Len(t, payload.State.PendingToolCalls, 1)
	require.Equal(t, "boom", payload.Err.Error())

	state.Messages[0].ID = "changed"
	state.PendingToolCalls[0].Name = "changed"
	require.Equal(t, "message-1", payload.State.Messages[0].ID)
	require.Equal(t, "grep", payload.State.PendingToolCalls[0].Name)
}

func TestQueryEventNameMappings(t *testing.T) {
	t.Parallel()

	cases := map[agentcontract.LoopStatus]app.EventName{
		agentcontract.LoopStarted:           app.EventAgentLoopStarted,
		agentcontract.LoopAwaitingTool:      app.EventAgentLoopAwaitingTool,
		agentcontract.LoopCompleted:         app.EventAgentLoopCompleted,
		agentcontract.LoopFinishFailed:      app.EventAgentLoopFailed,
		agentcontract.FinishReasonCancelled: app.EventAgentLoopCancelled,
	}

	for status, want := range cases {
		require.Equal(t, want, queryEventName(status))
	}
}
