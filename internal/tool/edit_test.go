package tool

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	toolcontract "github.com/YaHeii/agentGo/internal/tool/contract"
	"github.com/stretchr/testify/require"
)

func TestEditRejectsPathOutsideWorkingDir(t *testing.T) {
	t.Parallel()

	base := t.TempDir()
	projectRoot := filepath.Join(base, "project")
	outsideRoot := filepath.Join(base, "outside")
	require.NoError(t, os.MkdirAll(projectRoot, 0o755))
	require.NoError(t, os.MkdirAll(outsideRoot, 0o755))

	tool := NewEditTool()
	args := mustJSON(t, map[string]any{
		"file_path":  outsideRoot,
		"old_string": "a",
		"new_string": "b",
	})

	result := tool.Execute(context.Background(), toolcontract.ToolCallRequest{
		ToolCallID:      "call-1",
		Name:            EditToolName,
		Arguments:       args,
		PermissionLevel: toolcontract.AttentionLevel,
		Context: toolcontract.ToolCallContext{
			WorkingDir: projectRoot,
		},
	})

	require.Equal(t, toolcontract.StatusValidationFailed, result.Status)
	require.Contains(t, result.Content, "within the working directory")
}

func TestEditReplacesUniqueMatch(t *testing.T) {
	t.Parallel()

	projectRoot := t.TempDir()
	filePath := filepath.Join(projectRoot, "sample.txt")
	require.NoError(t, os.WriteFile(filePath, []byte("alpha beta gamma\n"), 0o644))

	tool := NewEditTool()
	args := mustJSON(t, map[string]any{
		"file_path":  "sample.txt",
		"old_string": "beta",
		"new_string": "delta",
	})

	result := tool.Execute(context.Background(), toolcontract.ToolCallRequest{
		ToolCallID:      "call-1",
		Name:            EditToolName,
		Arguments:       args,
		PermissionLevel: toolcontract.AttentionLevel,
		Context: toolcontract.ToolCallContext{
			WorkingDir: projectRoot,
		},
	})

	require.Equal(t, toolcontract.StatusSuccess, result.Status)
	content, err := os.ReadFile(filePath)
	require.NoError(t, err)
	require.Equal(t, "alpha delta gamma\n", string(content))
	require.Equal(t, 1, result.Metadata["additions"])
	require.Equal(t, 1, result.Metadata["removals"])
}

func TestEditRejectsMultipleMatchesWithoutReplaceAll(t *testing.T) {
	t.Parallel()

	projectRoot := t.TempDir()
	filePath := filepath.Join(projectRoot, "sample.txt")
	require.NoError(t, os.WriteFile(filePath, []byte("beta one\nbeta two\n"), 0o644))

	tool := NewEditTool()
	args := mustJSON(t, map[string]any{
		"file_path":  "sample.txt",
		"old_string": "beta",
		"new_string": "delta",
	})

	result := tool.Execute(context.Background(), toolcontract.ToolCallRequest{
		ToolCallID:      "call-1",
		Name:            EditToolName,
		Arguments:       args,
		PermissionLevel: toolcontract.AttentionLevel,
		Context: toolcontract.ToolCallContext{
			WorkingDir: projectRoot,
		},
	})

	require.Equal(t, toolcontract.StatusExecutionError, result.Status)
	require.Contains(t, result.Content, "multiple times")
}

func mustJSON(t *testing.T, v any) json.RawMessage {
	t.Helper()
	data, err := json.Marshal(v)
	require.NoError(t, err)
	return data
}
