package grep

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	toolpkg "github.com/YaHeii/agentGo/internal/tool"
	"github.com/stretchr/testify/require"
)

func TestParseRipgrepJSONParsesMatches(t *testing.T) {
	data := []byte(strings.Join([]string{
		`{"type":"match","data":{"path":{"text":"a.txt"},"lines":{"text":"hello world\n"},"line_number":3,"absolute_offset":12,"submatches":[{"match":{"text":"world"},"start":6,"end":11}]}}`,
		`{"type":"match","data":{"path":{"text":"b.go"},"lines":{"text":"fmt.Println(x)\n"},"line_number":8,"absolute_offset":30,"submatches":[{"match":{"text":"Println"},"start":4,"end":11}]}}`,
	}, "\n"))

	matches := parseRipgrepJSON(data, "")
	require.Len(t, matches, 2)
	require.Equal(t, "a.txt", matches[0].path)
	require.Equal(t, 3, matches[0].lineNum)
	require.Equal(t, 7, matches[0].charNum)
	require.Equal(t, "hello world", matches[0].lineText)
	require.Equal(t, "b.go", matches[1].path)
	require.Equal(t, 8, matches[1].lineNum)
	require.Equal(t, 5, matches[1].charNum)
	require.Equal(t, "fmt.Println(x)", matches[1].lineText)
}

func TestExecuteRejectsSiblingPathOutsideProjectRoot(t *testing.T) {
	base := t.TempDir()
	projectRoot := filepath.Join(base, "project")
	siblingRoot := filepath.Join(base, "project-other")
	require.NoError(t, os.MkdirAll(projectRoot, 0o755))
	require.NoError(t, os.MkdirAll(siblingRoot, 0o755))

	tool := NewGrepTool(projectRoot)

	args, err := json.Marshal(map[string]any{
		"pattern": "hello",
		"path":    siblingRoot,
	})
	require.NoError(t, err)

	result := tool.Execute(context.Background(), toolpkgReq("call-1", args, toolpkg.ToolCallContext{
		WorkingDir: projectRoot,
	}))
	require.Equal(t, toolpkg.StatusValidationFailed, result.Status)
	require.Contains(t, result.Content, "Access Denied")
}

func TestExecuteFindsLiteralTextInWorkingDir(t *testing.T) {
	projectRoot := t.TempDir()
	nestedDir := filepath.Join(projectRoot, "internal")
	require.NoError(t, os.MkdirAll(nestedDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(nestedDir, "sample.txt"), []byte("alpha\nneedle beta\n"), 0o644))

	tool := NewGrepTool(projectRoot)

	args, err := json.Marshal(map[string]any{
		"pattern":      "needle beta",
		"literal_text": true,
	})
	require.NoError(t, err)

	result := tool.Execute(context.Background(), toolpkgReq("call-1", args, toolpkg.ToolCallContext{
		WorkingDir: projectRoot,
	}))
	require.Equal(t, toolpkg.StatusSuccess, result.Status)
	require.Contains(t, result.Content, "sample.txt")
	require.Contains(t, result.Content, "needle beta")
	require.Equal(t, 1, result.Metadata["match_count"])
	require.Equal(t, false, result.Metadata["truncated"])
}

func TestExecuteResolvesRelativePathFromWorkingDir(t *testing.T) {
	projectRoot := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(projectRoot, "internal"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(projectRoot, "internal", "sample.txt"), []byte("needle\n"), 0o644))

	tool := NewGrepTool(projectRoot)

	args, err := json.Marshal(map[string]any{
		"pattern": "needle",
		"path":    "internal",
	})
	require.NoError(t, err)

	result := tool.Execute(context.Background(), toolpkgReq("call-1", args, toolpkg.ToolCallContext{
		WorkingDir: projectRoot,
	}))
	require.Equal(t, toolpkg.StatusSuccess, result.Status)
	require.Contains(t, result.Content, "internal/sample.txt")
}

func TestExecuteRejectsSymlinkPathEscapingProjectRoot(t *testing.T) {
	base := t.TempDir()
	projectRoot := filepath.Join(base, "project")
	outsideRoot := filepath.Join(base, "outside")
	require.NoError(t, os.MkdirAll(projectRoot, 0o755))
	require.NoError(t, os.MkdirAll(outsideRoot, 0o755))

	outsideFile := filepath.Join(outsideRoot, "secret.txt")
	require.NoError(t, os.WriteFile(outsideFile, []byte("needle\n"), 0o644))

	linkPath := filepath.Join(projectRoot, "escape.txt")
	require.NoError(t, os.Symlink(outsideFile, linkPath))

	tool := NewGrepTool(projectRoot)

	args, err := json.Marshal(map[string]any{
		"pattern": "needle",
		"path":    linkPath,
	})
	require.NoError(t, err)

	result := tool.Execute(context.Background(), toolpkgReq("call-1", args, toolpkg.ToolCallContext{
		WorkingDir: projectRoot,
	}))
	require.Equal(t, toolpkg.StatusValidationFailed, result.Status)
	require.Contains(t, result.Content, "Access Denied")
}

func TestServiceRejectsInvalidGrepArgumentsBeforeExecute(t *testing.T) {
	projectRoot := t.TempDir()
	svc := toolpkg.NewService(NewGrepTool(projectRoot))

	results, err := svc.Call(context.Background(), toolpkg.NewBatchRequest(
		toolpkg.NewToolCallRequest(
			"call-1",
			GrepToolName,
			json.RawMessage(`{"literal_text":"yes"}`),
			toolpkg.ToolCallContext{WorkingDir: projectRoot},
		),
	))
	require.Nil(t, results)
	require.Error(t, err)
}

func TestSearchWithNativeSkipsHiddenFilesAndTruncatesLongLines(t *testing.T) {
	projectRoot := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(projectRoot, ".git"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(projectRoot, "visible"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(projectRoot, ".hidden.txt"), []byte("needle\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(projectRoot, ".git", "config"), []byte("needle\n"), 0o644))

	longLine := "needle " + strings.Repeat("x", MaxLineContentWidth+25)
	require.NoError(t, os.WriteFile(filepath.Join(projectRoot, "visible", "match.txt"), []byte(longLine+"\n"), 0o644))

	tool := NewGrepTool(projectRoot)
	matches, truncated, err := tool.searchWithNative(context.Background(), "needle", projectRoot, "")
	require.NoError(t, err)
	require.False(t, truncated)
	require.Len(t, matches, 1)

	out := tool.renderOutput(matches, false)
	require.Contains(t, out, "visible/match.txt")
	require.NotContains(t, out, ".hidden.txt")
	require.NotContains(t, out, ".git")
	require.Contains(t, out, "...")
}

func TestSearchWithNativeRespectsGitignore(t *testing.T) {
	projectRoot := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(projectRoot, "ignored-dir"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(projectRoot, "visible"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(projectRoot, ".gitignore"), []byte("ignored.txt\nignored-dir/\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(projectRoot, "ignored.txt"), []byte("needle\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(projectRoot, "ignored-dir", "nested.txt"), []byte("needle\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(projectRoot, "visible", "match.txt"), []byte("needle visible\n"), 0o644))

	tool := NewGrepTool(projectRoot)
	matches, truncated, err := tool.searchWithNative(context.Background(), "needle", projectRoot, "")
	require.NoError(t, err)
	require.False(t, truncated)
	require.Len(t, matches, 1)
	require.Equal(t, filepath.Join(projectRoot, "visible", "match.txt"), matches[0].path)
}

func TestRunSearchSortsByNewestFileFirst(t *testing.T) {
	projectRoot := t.TempDir()
	oldPath := filepath.Join(projectRoot, "old.txt")
	newPath := filepath.Join(projectRoot, "new.txt")
	require.NoError(t, os.WriteFile(oldPath, []byte("needle old\n"), 0o644))
	require.NoError(t, os.WriteFile(newPath, []byte("needle new\n"), 0o644))

	oldTime := time.Now().Add(-2 * time.Hour)
	newTime := time.Now().Add(-1 * time.Hour)
	require.NoError(t, os.Chtimes(oldPath, oldTime, oldTime))
	require.NoError(t, os.Chtimes(newPath, newTime, newTime))

	tool := NewGrepTool(projectRoot)
	matches, truncated, err := tool.searchWithNative(context.Background(), "needle", projectRoot, "")
	require.NoError(t, err)
	require.False(t, truncated)
	require.Len(t, matches, 2)

	out := tool.renderOutput(matches, false)
	require.True(t, strings.Index(out, "new.txt") < strings.Index(out, "old.txt"))
}

func TestParseRipgrepJSONResolvesAbsolutePathAndModTime(t *testing.T) {
	projectRoot := t.TempDir()
	filePath := filepath.Join(projectRoot, "nested", "match.txt")
	require.NoError(t, os.MkdirAll(filepath.Dir(filePath), 0o755))
	require.NoError(t, os.WriteFile(filePath, []byte("hello world\n"), 0o644))

	wantTime := time.Now().Add(-30 * time.Minute)
	require.NoError(t, os.Chtimes(filePath, wantTime, wantTime))

	data := []byte(strings.Join([]string{
		`{"type":"match","data":{"path":{"text":"nested/match.txt"},"lines":{"text":"hello world\n"},"line_number":3,"absolute_offset":12,"submatches":[{"match":{"text":"world"},"start":6,"end":11}]}}`,
	}, "\n"))

	matches := parseRipgrepJSON(data, projectRoot)
	require.Len(t, matches, 1)
	require.Equal(t, filePath, matches[0].path)
	require.WithinDuration(t, wantTime, matches[0].modTime, time.Second)
}

func toolpkgReq(id string, args json.RawMessage, ctx toolpkg.ToolCallContext) toolpkg.ToolCallRequest {
	return toolpkg.NewToolCallRequest(id, GrepToolName, args, ctx)
}
