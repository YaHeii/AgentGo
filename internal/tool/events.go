package tool
// Deprecated
type ToolResultEvent struct {
	Status ToolCallStatus
	Result ToolResult
	Err    error
}

type ToolCallStatus string

const (
	// StatusSuccess the result could be returned directly
	StatusSuccess ToolCallStatus = "success"

	// StatusSyntaxRepaired JSON Syntax error, but successfully fixed locally (e.g., by completing missing quotes).
	StatusSyntaxRepaired ToolCallStatus = "syntax_repaired"

	// StatusValidationFailed Parameters do not conform to the Schema validation.
	// Strategy: Must be returned to the model, accompanied by a detailed description of the Schema error.
	StatusValidationFailed ToolCallStatus = "validation_failed"

	// StatusExecutionError Tool logic execution failed (e.g., file does not exist).
	// Strategy: Return to the model, allowing it to decide what to do next
	StatusExecutionError ToolCallStatus = "execution_error"

	// StatusSystemError System-level failures (e.g., process crashes, timeouts)
	// Strategy: Do not return to the model; instead, directly interrupt the Agent loop or throw an exception.
	StatusSystemError ToolCallStatus = "system_error"
)
