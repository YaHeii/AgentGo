package tool

import (
	"context"

	"github.com/YaHeii/agentGo/internal/app"
)
//DONOT use ch to return result 
//the agent is plan and execute,he need the result 
// to decide what to do next
// just block the result ,can use the event dispatcher 
// but should consider the time delay

type ToolResult struct {
	Content  string
	Metadata map[string]any
}

type ToolReqParam struct {
	
}

type ToolService struct {
	MCPClient       MCPStore
	parentSessionID string
	dispatcher      app.Dispatcher
}

// XXX: return the full toolDefinition List
func (s *ToolService) ListTools(ctx context.Context) []Definition {
	return nil
}

func (s *ToolService) Call(ctx context.Context, param ToolReqParam) ToolResult {
	//1. check the permission
	//2. check isConcurrentSafe
	//3. ToolCall
	//4. Process / Cropp Results
	return ToolResult{}
}
