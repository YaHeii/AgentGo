package contract

import "time"

type Message struct {
	ID               string
	SessionID        string
	Kind             Kind
	CreatedAt        time.Time
	UpdatedAt        time.Time
	IsCompactSummary bool
	Parts            []Part
	System           *SystemPayload
	Progress         *ProgressPayload
}

type Kind string

const (
	KindUser      Kind = "user"
	KindAssistant Kind = "assistant"
	KindSystem    Kind = "system"
)

type Part struct {
	Type       PartType
	Text       string
	Image      *ImagePart
	ToolCall   *ToolCallPart
	ToolResult *ToolResultPart
	Thinking   *ThinkingPart
}

type SystemPayload struct {
	Subtype string
	Level   string
}

type ProgressPayload struct {
	Stage   string
	Current int
	Total   int
	Done    bool
}

type PartType string

const (
	PartTypeText       PartType = "text"
	PartTypeImage      PartType = "image"
	PartTypeToolCall   PartType = "tool_call"
	PartTypeToolResult PartType = "tool_result"
	PartTypeThinking   PartType = "thinking"
	PartTypeAttachment PartType = "attachment"
	PartTypeSummary    PartType = "summary"
)

type ImagePart struct {
	URL         string
	MediaType   string
	Alt         string
	Description string
}

type ToolCallPart struct {
	ID     string
	Name   string
	Input  string
	Status string
}

type ToolResultPart struct {
	ToolCallID string
	Content    string
	IsError    bool
}

type ThinkingPart struct {
	Content string
	Summary string
}

type AttachmentPart struct {
	Name      string
	Path      string
	MediaType string
	SizeBytes int64
}

type SummaryPart struct {
	Content string
	Range   string
}

type CreateMessageParams struct {
	ID               string
	SessionID        string
	Kind             Kind
	Provider         string
	IsCompactSummary bool
	Parts            []Part
	System           *SystemPayload
	Progress         *ProgressPayload
}
