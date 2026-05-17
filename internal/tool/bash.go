package tool

// TODO:
// - Reintroduce background job support when agentGo has a local shell job manager.
// - Reintroduce structured permission checks if a permission subsystem lands.
// - Reintroduce richer command blocking rules if a local blocker framework is added.

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	toolcontract "github.com/YaHeii/agentGo/internal/tool/contract"
)

const BashToolName = "bash"

const maxBashOutputLength = 30000

type BashParams struct {
	Description         string `json:"description"`
	Command             string `json:"command"`
	WorkingDir          string `json:"working_dir,omitempty"`
	RunInBackground     bool   `json:"run_in_background,omitempty"`
	AutoBackgroundAfter int    `json:"auto_background_after,omitempty"`
}

type BashTool struct{}

func NewBashTool() *BashTool {
	return &BashTool{}
}

func (t *BashTool) Metadata() toolcontract.Metadata {
	return toolcontract.Metadata{
		Name:        BashToolName,
		Description: "在工作区内执行 shell 命令。",
		Parameters: json.RawMessage(`{
			"type": "object",
			"properties": {
				"description": { "type": "string" },
				"command": { "type": "string" },
				"working_dir": { "type": "string" },
				"run_in_background": { "type": "boolean" },
				"auto_background_after": { "type": "integer" }
			},
			"required": ["command"]
		}`),
		Enabled:           true,
		SecurityLevel:     toolcontract.AttentionLevel,
		IsConcurrencySafe: false,
		Requirements:      toolcontract.RequireWorkingDir,
	}
}

func (t *BashTool) Execute(_ context.Context, req toolcontract.ToolCallRequest) toolcontract.ToolResult {
	var params BashParams
	if err := json.Unmarshal(req.Arguments, &params); err != nil {
		return toolcontract.ToolResult{
			ToolCallID: req.ToolCallID,
			Name:       req.Name,
			Status:     toolcontract.StatusSystemError,
			Content:    "failed to decode bash arguments",
			Err:        err,
		}
	}

	if strings.TrimSpace(params.Command) == "" {
		return toolcontract.ToolResult{
			ToolCallID: req.ToolCallID,
			Name:       req.Name,
			Status:     toolcontract.StatusValidationFailed,
			Content:    "missing command",
			Err:        fmt.Errorf("missing command"),
		}
	}
	if params.RunInBackground {
		return toolcontract.ToolResult{
			ToolCallID: req.ToolCallID,
			Name:       req.Name,
			Status:     toolcontract.StatusValidationFailed,
			Content:    "background execution is not implemented yet",
			Err:        fmt.Errorf("background execution is not supported"),
		}
	}

	workingDir, err := resolveBashWorkingDir(req.Context.WorkingDir, req.Context.WorkspaceRoot, params.WorkingDir)
	if err != nil {
		return toolcontract.ToolResult{
			ToolCallID: req.ToolCallID,
			Name:       req.Name,
			Status:     toolcontract.StatusValidationFailed,
			Content:    "working_dir must stay within the working directory or workspace root",
			Err:        err,
		}
	}

	start := time.Now().UTC()
	cmd := exec.CommandContext(context.Background(), "bash", "-lc", params.Command)
	cmd.Dir = workingDir
	output, err := cmd.CombinedOutput()
	end := time.Now().UTC()

	text := strings.TrimSpace(string(output))
	if len(text) > maxBashOutputLength {
		text = text[:maxBashOutputLength]
	}
	if text == "" {
		text = "no output"
	}

	status := toolcontract.StatusSuccess
	if err != nil {
		status = toolcontract.StatusExecutionError
	}

	return toolcontract.ToolResult{
		ToolCallID: req.ToolCallID,
		Name:       req.Name,
		Status:     status,
		Content:    text,
		Metadata: map[string]any{
			"start_time":        start.UnixMilli(),
			"end_time":          end.UnixMilli(),
			"working_directory": workingDir,
			"output":            text,
			"description":       params.Description,
			"background":        false,
		},
		Err: err,
	}
}

func resolveBashWorkingDir(defaultWorkingDir, workspaceRoot, requested string) (string, error) {
	base := strings.TrimSpace(defaultWorkingDir)
	if base == "" {
		base = strings.TrimSpace(workspaceRoot)
	}
	if base == "" {
		base = "."
	}

	baseAbs, err := filepath.Abs(base)
	if err != nil {
		return "", err
	}

	target := strings.TrimSpace(requested)
	if target == "" {
		target = baseAbs
	} else if !filepath.IsAbs(target) {
		target = filepath.Join(baseAbs, target)
	}

	targetAbs, err := filepath.Abs(target)
	if err != nil {
		return "", err
	}
	if targetAbs != baseAbs && !strings.HasPrefix(targetAbs, baseAbs+string(os.PathSeparator)) {
		return "", fmt.Errorf("working dir %q escapes base %q", targetAbs, baseAbs)
	}
	return targetAbs, nil
}
