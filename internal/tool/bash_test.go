package tool

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	toolcontract "github.com/YaHeii/agentGo/internal/tool/contract"
	"github.com/YaHeii/agentGo/internal/tool/sandbox"
	"github.com/stretchr/testify/require"
)

func TestBashMetadataUsesSandboxedCommandSchema(t *testing.T) {
	t.Parallel()

	tool := NewBashTool()
	meta := tool.Metadata()

	require.Equal(t, BashToolName, meta.Name)
	require.Contains(t, meta.Description, "沙箱")
	require.Contains(t, meta.Description, "工作目录")
	require.JSONEq(t, `{
		"type": "object",
		"properties": {
			"command": {
				"type": "string",
				"description": "要交给 'bash -lc' 执行的命令或多行脚本。使用标准 POSIX shell 语法，可包含换行；命令会在当前 working_dir 中运行。"
			}
		},
		"required": ["command"]
	}`, string(meta.Parameters))
}

func TestBashRejectsMissingCommand(t *testing.T) {
	t.Parallel()

	tool := NewBashTool()
	result := tool.Execute(context.Background(), toolcontract.ToolCallRequest{
		ToolCallID:      "call-1",
		Name:            BashToolName,
		Arguments:       []byte(`{"description":"empty"}`),
		PermissionLevel: toolcontract.AttentionLevel,
		Context: toolcontract.ToolCallContext{
			WorkingDir: t.TempDir(),
		},
	})

	require.Equal(t, toolcontract.StatusValidationFailed, result.Status)
	require.Contains(t, result.Content, "missing command")
}

func TestBashRunsCommandThroughSandbox(t *testing.T) {
	t.Parallel()

	projectRoot := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(projectRoot, "sample.txt"), []byte("hello\n"), 0o644))

	runner := &stubSandboxRunner{
		result: sandbox.Result{
			ExitCode: 0,
			Stdout:   projectRoot + "\n",
		},
	}
	tool := NewBashToolWithRunner(runner)
	result := tool.Execute(context.Background(), toolcontract.ToolCallRequest{
		ToolCallID:      "call-1",
		Name:            BashToolName,
		Arguments:       []byte(`{"command":"pwd"}`),
		PermissionLevel: toolcontract.AttentionLevel,
		Context: toolcontract.ToolCallContext{
			WorkingDir: projectRoot,
		},
	})

	require.Equal(t, toolcontract.StatusSuccess, result.Status)
	require.Contains(t, result.Content, projectRoot)
	require.Equal(t, projectRoot, result.Metadata["workspace_directory"])
	require.Equal(t, "sandbox", result.Metadata["executor"])
	require.Len(t, runner.specs, 1)
	require.Equal(t, "bash", runner.specs[0].Executable)
	require.Equal(t, []string{"-lc", "pwd"}, runner.specs[0].Args)
	require.Equal(t, projectRoot, runner.specs[0].WorkspaceDir)
}

func TestBashMapsSandboxExitCodeToExecutionError(t *testing.T) {
	t.Parallel()

	runner := &stubSandboxRunner{
		result: sandbox.Result{
			ExitCode: 2,
			Stdout:   "out",
			Stderr:   "err",
		},
	}
	tool := NewBashToolWithRunner(runner)

	result := tool.Execute(context.Background(), toolcontract.ToolCallRequest{
		ToolCallID:      "call-1",
		Name:            BashToolName,
		Arguments:       json.RawMessage(`{"command":"exit 2"}`),
		PermissionLevel: toolcontract.AttentionLevel,
		Context: toolcontract.ToolCallContext{
			WorkingDir: t.TempDir(),
		},
	})

	require.Equal(t, toolcontract.StatusExecutionError, result.Status)
	require.Contains(t, result.Content, "out")
	require.Contains(t, result.Content, "err")
	require.Equal(t, 2, result.Metadata["exit_code"])
}

type stubSandboxRunner struct {
	specs  []sandbox.Spec
	result sandbox.Result
	err    error
}

func (s *stubSandboxRunner) Run(_ context.Context, spec sandbox.Spec) (sandbox.Result, error) {
	s.specs = append(s.specs, spec)
	return s.result, s.err
}
