package agent

import (
	agentcontract "github.com/YaHeii/agentGo/internal/agent/contract"
	"github.com/YaHeii/agentGo/internal/app"
)

type QueryEvent struct {
	Status agentcontract.LoopStatus
	State  LoopState
	Err    error
}

func NewQueryAppEvent(status agentcontract.LoopStatus, state LoopState, err error) app.Event {
	return app.NewEvent(queryEventName(status), QueryEvent{
		Status: status,
		State:  copyLoopState(state),
		Err:    err,
	})
}

func queryEventName(status agentcontract.LoopStatus) app.EventName {
	switch status {
	case agentcontract.LoopStarted:
		return app.EventAgentLoopStarted
	case agentcontract.LoopAwaitingTool:
		return app.EventAgentLoopAwaitingTool
	case agentcontract.LoopCompleted:
		return app.EventAgentLoopCompleted
	case agentcontract.FinishReasonCancelled:
		return app.EventAgentLoopCancelled
	case agentcontract.LoopFinishFailed:
		return app.EventAgentLoopFailed
	default:
		return app.EventAgentLoopFailed
	}
}
