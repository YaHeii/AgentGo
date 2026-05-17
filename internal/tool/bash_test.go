package tool

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	toolcontract "github.com/YaHeii/agentGo/internal/tool/contract"
	"github.com/stretchr/testify/require"
)

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

func TestBashRejectsBackgroundExecution(t *testing.T) {
	t.Parallel()

	tool := NewBashTool()
	result := tool.Execute(context.Background(), toolcontract.ToolCallRequest{
		ToolCallID:      "call-1",
		Name:            BashToolName,
		Arguments:       []byte(`{"command":"pwd","run_in_background":true}`),
		PermissionLevel: toolcontract.AttentionLevel,
		Context: toolcontract.ToolCallContext{
			WorkingDir: t.TempDir(),
		},
	})

	require.Equal(t, toolcontract.StatusValidationFailed, result.Status)
	require.Contains(t, result.Content, "background")
}

func TestBashRunsSimpleCommandInWorkingDir(t *testing.T) {
	t.Parallel()

	projectRoot := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(projectRoot, "sample.txt"), []byte("hello\n"), 0o644))

	tool := NewBashTool()
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
	require.Equal(t, projectRoot, result.Metadata["working_directory"])
}
