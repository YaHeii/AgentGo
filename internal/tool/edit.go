package tool

// TODO:
// - Reintroduce permission checks if agentGo later gains a permission subsystem.
// - Reintroduce file history / filetracker integration if the repo adds those services.
// - Reintroduce LSP diagnostics after edit once a local LSP manager exists.

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	toolcontract "github.com/YaHeii/agentGo/internal/tool/contract"
)

const EditToolName = "edit"

type EditParams struct {
	FilePath   string `json:"file_path"`
	OldString  string `json:"old_string"`
	NewString  string `json:"new_string"`
	ReplaceAll bool   `json:"replace_all,omitempty"`
}

type EditResponseMetadata struct {
	Additions  int    `json:"additions"`
	Removals   int    `json:"removals"`
	OldContent string `json:"old_content,omitempty"`
	NewContent string `json:"new_content,omitempty"`
}

type EditTool struct{}

func NewEditTool() *EditTool {
	return &EditTool{}
}

func (t *EditTool) Metadata() toolcontract.Metadata {
	return toolcontract.Metadata{
		Name:        EditToolName,
		Description: "在工作区内创建文件或替换文件内容。",
		Parameters: json.RawMessage(`{
			"type": "object",
			"properties": {
				"file_path": { "type": "string" },
				"old_string": { "type": "string" },
				"new_string": { "type": "string" },
				"replace_all": { "type": "boolean" }
			},
			"required": ["file_path"]
		}`),
		Enabled:           true,
		SecurityLevel:     toolcontract.AttentionLevel,
		IsConcurrencySafe: false,
		Requirements:      toolcontract.RequireWorkingDir,
	}
}

func (t *EditTool) Execute(_ context.Context, req toolcontract.ToolCallRequest) toolcontract.ToolResult {
	var params EditParams
	if err := json.Unmarshal(req.Arguments, &params); err != nil {
		return toolcontract.ToolResult{
			ToolCallID: req.ToolCallID,
			Name:       req.Name,
			Status:     toolcontract.StatusSystemError,
			Content:    "failed to decode edit arguments",
			Err:        err,
		}
	}

	filePath, err := resolveToolPath(req.Context.WorkingDir, req.Context.WorkspaceRoot, params.FilePath)
	if err != nil {
		return toolcontract.ToolResult{
			ToolCallID: req.ToolCallID,
			Name:       req.Name,
			Status:     toolcontract.StatusValidationFailed,
			Content:    "file_path must be within the working directory or workspace root",
			Err:        err,
		}
	}

	if params.OldString == "" {
		return createFile(req, filePath, params.NewString)
	}
	return replaceOrDelete(req, filePath, params)
}

func createFile(req toolcontract.ToolCallRequest, filePath string, newContent string) toolcontract.ToolResult {
	if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
		return toolcontract.ToolResult{
			ToolCallID: req.ToolCallID,
			Name:       req.Name,
			Status:     toolcontract.StatusSystemError,
			Content:    "failed to create parent directory",
			Err:        err,
		}
	}
	if err := os.WriteFile(filePath, []byte(newContent), 0o644); err != nil {
		return toolcontract.ToolResult{
			ToolCallID: req.ToolCallID,
			Name:       req.Name,
			Status:     toolcontract.StatusSystemError,
			Content:    "failed to write file",
			Err:        err,
		}
	}
	return toolcontract.ToolResult{
		ToolCallID: req.ToolCallID,
		Name:       req.Name,
		Status:     toolcontract.StatusSuccess,
		Content:    "File created: " + filePath,
		Metadata: map[string]any{
			"additions":   1,
			"removals":    0,
			"old_content": "",
			"new_content": newContent,
		},
	}
}

func replaceOrDelete(req toolcontract.ToolCallRequest, filePath string, params EditParams) toolcontract.ToolResult {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return toolcontract.ToolResult{
			ToolCallID: req.ToolCallID,
			Name:       req.Name,
			Status:     toolcontract.StatusExecutionError,
			Content:    fmt.Sprintf("failed to read file: %s", filePath),
			Err:        err,
		}
	}

	oldContent := string(content)
	matchCount := strings.Count(oldContent, params.OldString)
	if matchCount == 0 {
		return toolcontract.ToolResult{
			ToolCallID: req.ToolCallID,
			Name:       req.Name,
			Status:     toolcontract.StatusExecutionError,
			Content:    "old_string not found in file. Make sure it matches exactly, including whitespace and line breaks.",
			Err:        fmt.Errorf("old_string not found"),
		}
	}
	if matchCount > 1 && !params.ReplaceAll {
		return toolcontract.ToolResult{
			ToolCallID: req.ToolCallID,
			Name:       req.Name,
			Status:     toolcontract.StatusExecutionError,
			Content:    "old_string appears multiple times in the file. Please provide more context to ensure a unique match, or set replace_all to true",
			Err:        fmt.Errorf("multiple matches found"),
		}
	}

	newContent := ""
	if params.ReplaceAll {
		newContent = strings.ReplaceAll(oldContent, params.OldString, params.NewString)
	} else {
		newContent = strings.Replace(oldContent, params.OldString, params.NewString, 1)
	}

	if err := os.WriteFile(filePath, []byte(newContent), 0o644); err != nil {
		return toolcontract.ToolResult{
			ToolCallID: req.ToolCallID,
			Name:       req.Name,
			Status:     toolcontract.StatusSystemError,
			Content:    "failed to write file",
			Err:        err,
		}
	}

	metadata := EditResponseMetadata{
		Additions:  1,
		Removals:   1,
		OldContent: oldContent,
		NewContent: newContent,
	}
	return toolcontract.ToolResult{
		ToolCallID: req.ToolCallID,
		Name:       req.Name,
		Status:     toolcontract.StatusSuccess,
		Content:    "Content updated: " + filePath,
		Metadata: map[string]any{
			"additions":   metadata.Additions,
			"removals":    metadata.Removals,
			"old_content": metadata.OldContent,
			"new_content": metadata.NewContent,
		},
	}
}

func resolveToolPath(workingDir, workspaceRoot, requestedPath string) (string, error) {
	base := strings.TrimSpace(workingDir)
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

	target := requestedPath
	if !filepath.IsAbs(target) {
		target = filepath.Join(baseAbs, target)
	}
	targetAbs, err := filepath.Abs(target)
	if err != nil {
		return "", err
	}

	if targetAbs != baseAbs && !strings.HasPrefix(targetAbs, baseAbs+string(os.PathSeparator)) {
		return "", fmt.Errorf("path %q escapes base %q", targetAbs, baseAbs)
	}
	return targetAbs, nil
}
