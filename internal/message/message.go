package message

//manage message communicate/store
import (
	"context"
	"time"

	"github.com/YaHeii/agentGo/internal/store"
)

// basic message struct
type Message struct {
	ID        string
	SessionID string

	Kind      Kind
	CreatedAt time.Time
	UpdatedAt time.Time

	Flags Flags
	Parts []Part

	System   *SystemPayload
	Progress *ProgressPayload
}

// The kind list define the message type.
type Kind string

const (
	KindUser       Kind = "user"
	KindAssistant  Kind = "assistant"
	KindSystem     Kind = "system"
	KindProgress   Kind = "progress"
	KindAttachment Kind = "attachment"
)

type Flags struct {
	IsMeta                    bool // metadata in system
	IsCompactSummary          bool // compact summary
	IsVisibleInTranscriptOnly bool // only used in ui/transcript not for ai
}

// The part list contains the message body.
type Part struct {
	Type       PartType
	Text       string
	Image      *ImagePart
	ToolCall   *ToolCallPart
	ToolResult *ToolResultPart
	Thinking   *ThinkingPart
	Attachment *AttachmentPart
	Summary    *SummaryPart
}

// Structured metadata of system messages
type SystemPayload struct {
	Subtype string // informational, local_command, error
	Level   string // info, warning, error
}

// Structured state of progress messages
type ProgressPayload struct {
	Stage   string // thinking, calling_tool, compacting
	Current int
	Total   int
	Done    bool
}

// PartType identifies which payload in Part is populated.
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

// ImagePart references an image carried by a message.
type ImagePart struct {
	URL         string
	MediaType   string
	Alt         string
	Description string
}

// ToolCallPart describes a tool invocation requested by the model.
type ToolCallPart struct {
	ID     string
	Name   string
	Input  string
	Status string
}

// ToolResultPart stores the serialized result of a tool execution.
type ToolResultPart struct {
	ToolCallID string
	Content    string
	IsError    bool
}

// ThinkingPart keeps non-final reasoning text for rendering or debugging.
type ThinkingPart struct {
	Content string
	Summary string
}

// AttachmentPart references a local or remote attachment bound to the message.
type AttachmentPart struct {
	Name      string
	Path      string
	MediaType string
	SizeBytes int64
}

// SummaryPart stores a compacted summary generated from previous turns.
type SummaryPart struct {
	Content string
	Range   string
}

type CreateMessageParams struct {
	ID               string
	Kind             Kind
	Provider         string
	FinishedAt       time.Time
	IsCompactSummary bool
	Flags            Flags
	Parts            []Part
	System           *SystemPayload
	Progress         *ProgressPayload
}

type persistedMessage struct {
	Flags    Flags            `json:"flags"`
	Parts    []Part           `json:"parts"`
	System   *SystemPayload   `json:"system,omitempty"`
	Progress *ProgressPayload `json:"progress,omitempty"`
}

type messageStore interface {
	CreateMessage(ctx context.Context, params store.CreateMessageParams) (store.Message, error)
	ListMessages(ctx context.Context, sessionID string) ([]store.Message, error)
}
