package tool

// TODO:
// - Reintroduce background job support when agentGo has a local shell job manager.
// - Reintroduce structured permission checks if a permission subsystem lands.
// - Reintroduce richer command blocking rules if a local blocker framework is added.

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	toolcontract "github.com/YaHeii/agentGo/internal/tool/contract"
	"github.com/YaHeii/agentGo/internal/tool/sandbox"
)

const BashToolName = "bash"

const maxBashOutputLength = 30000

type BashParams struct {
	Command string `json:"command"`
}

type BashTool struct {
	runner sandbox.Runner
}

func NewBashTool() *BashTool {
	return NewBashToolWithRunner(sandbox.NewRunner())
}

func NewBashToolWithRunner(runner sandbox.Runner) *BashTool {
	if runner == nil {
		runner = sandbox.NewRunner()
	}
	return &BashTool{runner: runner}
}

func (t *BashTool) Metadata() toolcontract.Metadata {
	return toolcontract.Metadata{
		Name:        BashToolName,
		Description: "在当前工作目录中通过受限沙箱执行一次性 shell 命令或短脚本。适合检查环境、运行测试或调用本地 CLI；命令本身可能产生文件或进程副作用。",
		Parameters: json.RawMessage(`{
			"type": "object",
			"properties": {
				"command": {
					"type": "string",
					"description": "要交给 'bash -lc' 执行的命令或多行脚本。使用标准 POSIX shell 语法，可包含换行；命令会在当前 working_dir 中运行。"
				}
			},
			"required": ["command"]
		}`),
		Enabled:           true,
		SecurityLevel:     toolcontract.AttentionLevel,
		IsConcurrencySafe: false,
		Requirements:      toolcontract.RequireWorkingDir,
	}
}

func (t *BashTool) Execute(ctx context.Context, req toolcontract.ToolCallRequest) toolcontract.ToolResult {
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

	workspaceDir, err := resolveBashWorkspaceDir(req.Context.WorkingDir, req.Context.WorkspaceRoot)
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
	result, err := t.runner.Run(ctx, sandbox.Spec{
		Executable:   "bash",
		Args:         []string{"-lc", params.Command},
		WorkspaceDir: workspaceDir,
	})
	end := time.Now().UTC()
	if err != nil {
		return toolcontract.ToolResult{
			ToolCallID: req.ToolCallID,
			Name:       req.Name,
			Status:     toolcontract.StatusSystemError,
			Content:    "sandbox command failed",
			Metadata: map[string]any{
				"start_time":          start.UnixMilli(),
				"end_time":            end.UnixMilli(),
				"workspace_directory": workspaceDir,
				"executor":            "sandbox",
			},
			Err: err,
		}
	}

	text := formatBashOutput(result.Stdout, result.Stderr)
	if len(text) > maxBashOutputLength {
		text = text[:maxBashOutputLength]
	}

	status := toolcontract.StatusSuccess
	if result.ExitCode != 0 {
		status = toolcontract.StatusExecutionError
	}

	return toolcontract.ToolResult{
		ToolCallID: req.ToolCallID,
		Name:       req.Name,
		Status:     status,
		Content:    text,
		Metadata: map[string]any{
			"start_time":          start.UnixMilli(),
			"end_time":            end.UnixMilli(),
			"workspace_directory": workspaceDir,
			"exit_code":           result.ExitCode,
			"output":              text,
			"executor":            "sandbox",
		},
	}
}

func resolveBashWorkspaceDir(defaultWorkingDir, workspaceRoot string) (string, error) {
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
	return baseAbs, nil
}

func formatBashOutput(stdout string, stderr string) string {
	parts := make([]string, 0, 2)
	if text := strings.TrimSpace(stdout); text != "" {
		parts = append(parts, text)
	}
	if text := strings.TrimSpace(stderr); text != "" {
		parts = append(parts, text)
	}
	if len(parts) == 0 {
		return "no output"
	}
	return strings.Join(parts, "\n")
}
