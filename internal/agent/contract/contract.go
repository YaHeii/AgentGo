package contract

import "github.com/YaHeii/agentGo/internal/tool"

// RuntimeSnapshot is a read-only view of runtime state consumed by agent code.
type RuntimeSnapshot struct {
	AppVersion      string
	ProjectRoot     string
	Cwd             string
	PermissionLevel tool.SecurityLevel
	ModelLimit      int
	Model           string
	Temperature     float32
}

// TokenEncoder is the minimal tokenizer capability the agent needs.
type TokenEncoder interface {
	Encode(text string, allowedSpecial []string, disallowedSpecial []string) []int
}

// RuntimeProvider supplies runtime data needed by the query loop.
type RuntimeProvider interface {
	Snapshot() RuntimeSnapshot
	TokenizerForModel(model string) (TokenEncoder, error)
}
