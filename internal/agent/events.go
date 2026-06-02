package agent

import (
	agentcontract "github.com/YaHeii/agentGo/internal/agent/contract"
)
type QueryEvent struct {
	Status agentcontract.LoopStatus
	State  LoopState
	Err    error
}
