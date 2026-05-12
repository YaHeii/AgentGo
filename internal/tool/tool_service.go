package tool

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"slices"
	"strings"
	"sync"
)

type ToolResult struct {
	ToolCallID string
	Name       string
	Status     ToolCallStatus
	Content    string
	Metadata   map[string]any
	Err        error
}

type Service struct {
	mu       sync.RWMutex
	registry map[string]Tool
	locksMu  sync.Mutex
	locks    map[string]*sync.Mutex
}

type validatedToolCall struct {
	tool Tool
	meta Metadata
	call ToolCallRequest
}

type parameterSchema struct {
	Type       string                     `json:"type"`
	Properties map[string]parameterSchema `json:"properties"`
	Required   []string                   `json:"required"`
	Items      *parameterSchema           `json:"items"`
}

func NewService(tools ...Tool) *Service {
	svc := &Service{
		registry: make(map[string]Tool, len(tools)),
		locks:    make(map[string]*sync.Mutex),
	}
	for _, tool := range tools {
		if tool == nil {
			continue
		}
		meta := tool.Metadata()
		if meta.Name == "" {
			continue
		}
		svc.registry[meta.Name] = tool
	}
	return svc
}

func (s *Service) ListTools(_ context.Context) []Metadata {
	s.mu.RLock()
	defer s.mu.RUnlock()

	names := make([]string, 0, len(s.registry))
	for name := range s.registry {
		names = append(names, name)
	}
	slices.Sort(names)

	metas := make([]Metadata, 0, len(names))
	for _, name := range names {
		metas = append(metas, s.registry[name].Metadata())
	}
	return metas
}

func (s *Service) Call(ctx context.Context, req BatchRequest) ([]ToolResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if len(req.Calls) == 0 {
		return nil, nil
	}

	validated := make([]validatedToolCall, len(req.Calls))
	for i := range req.Calls {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		call := req.Calls[i]
		tool, ok := s.lookup(call.Name)
		if !ok {
			return nil, fmt.Errorf("%w: %q", errToolNotFound, call.Name)
		}

		meta := tool.Metadata()
		if err := validateCall(meta, call); err != nil {
			return nil, err
		}
		validated[i] = validatedToolCall{
			tool: tool,
			meta: meta,
			call: call,
		}
	}

	results := make([]ToolResult, len(req.Calls))
	var wg sync.WaitGroup

	for i := range validated {
		run := func(index int, vc validatedToolCall) {
			defer wg.Done()
			results[index] = s.executeCall(ctx, vc)
		}

		call := validated[i]
		wg.Add(1)
		if call.meta.IsConcurrencySafe {
			go run(i, call)
			continue
		}
		run(i, call)
	}

	wg.Wait()

	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return results, nil
}
// TODO: Using zog makes the implementation even more elegant. 
func validateCall(meta Metadata, call ToolCallRequest) error {
	if err := validateCallPermission(meta, call); err != nil {
		return err
	}
	if err := validateCallArguments(meta, call.Arguments); err != nil {
		return err
	}
	return nil
}

// get permissionLevelfrom Environment  if don't match send the event
func validateCallPermission(meta Metadata, call ToolCallRequest) error {
	if !meta.Enabled {
		return fmt.Errorf("tool %q is disabled", call.Name)
	}
	if meta.Requirements&RequireWorkingDir != 0 && strings.TrimSpace(call.Context.WorkingDir) == "" {
		return fmt.Errorf("tool %q requires working_dir", call.Name)
	}
	if meta.Requirements&RequireWorkspaceRoot != 0 && strings.TrimSpace(call.Context.WorkspaceRoot) == "" {
		return fmt.Errorf("tool %q requires workspace_root", call.Name)
	}
	return nil
}

func validateCallArguments(meta Metadata, args json.RawMessage) error {
	value, err := decodeJSONValue(args)
	if err != nil {
		return fmt.Errorf("tool %q arguments must be valid JSON: %w", meta.Name, err)
	}

	if len(bytes.TrimSpace(meta.Parameters)) == 0 {
		return nil
	}

	var schema parameterSchema
	if err := json.Unmarshal(meta.Parameters, &schema); err != nil {
		return fmt.Errorf("tool %q parameters schema is invalid: %w", meta.Name, err)
	}

	if err := validateSchemaValue(schema, value, "arguments"); err != nil {
		return fmt.Errorf("tool %q arguments validation failed: %w", meta.Name, err)
	}
	return nil
}

func validateSchemaValue(schema parameterSchema, value any, path string) error {
	if schema.Type != "" {
		match, err := schemaTypeMatches(schema.Type, value)
		if err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		if !match {
			return fmt.Errorf("%s must be %s", path, schema.Type)
		}
	}

	if len(schema.Properties) == 0 && len(schema.Required) == 0 && schema.Items == nil {
		return nil
	}

	switch typed := value.(type) {
	case map[string]any:
		for _, name := range schema.Required {
			if _, ok := typed[name]; !ok {
				return fmt.Errorf("%s.%s is required", path, name)
			}
		}
		for name, propSchema := range schema.Properties {
			propValue, ok := typed[name]
			if !ok {
				continue
			}
			if err := validateSchemaValue(propSchema, propValue, path+"."+name); err != nil {
				return err
			}
		}
	case []any:
		if schema.Items == nil {
			return nil
		}
		for i, item := range typed {
			itemPath := fmt.Sprintf("%s[%d]", path, i)
			if err := validateSchemaValue(*schema.Items, item, itemPath); err != nil {
				return err
			}
		}
	default:
		if len(schema.Properties) > 0 || len(schema.Required) > 0 {
			return fmt.Errorf("%s must be object", path)
		}
		if schema.Items != nil {
			return fmt.Errorf("%s must be array", path)
		}
	}

	return nil
}

func schemaTypeMatches(expected string, value any) (bool, error) {
	switch expected {
	case "string":
		_, ok := value.(string)
		return ok, nil
	case "boolean":
		_, ok := value.(bool)
		return ok, nil
	case "number":
		switch value.(type) {
		case float64, json.Number:
			return true, nil
		default:
			return false, nil
		}
	case "integer":
		switch typed := value.(type) {
		case json.Number:
			f, err := typed.Float64()
			if err != nil {
				return false, nil
			}
			return math.Trunc(f) == f, nil
		case float64:
			return math.Trunc(typed) == typed, nil
		default:
			return false, nil
		}
	case "object":
		_, ok := value.(map[string]any)
		return ok, nil
	case "array":
		_, ok := value.([]any)
		return ok, nil
	case "":
		return true, nil
	default:
		return false, fmt.Errorf("unsupported schema type %q", expected)
	}
}

func decodeJSONValue(data json.RawMessage) (any, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()

	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return nil, fmt.Errorf("unexpected trailing data")
	}
	return value, nil
}

func (s *Service) executeCall(ctx context.Context, vc validatedToolCall) ToolResult {
	call := vc.call
	if err := ctx.Err(); err != nil {
		return ToolResult{
			ToolCallID: call.ToolCallID,
			Name:       call.Name,
			Status:     StatusSystemError,
			Err:        err,
		}
	}

	if !vc.meta.IsConcurrencySafe {
		lock := s.lockFor(call.Name)
		lock.Lock()
		defer lock.Unlock()
	}

	result := vc.tool.Execute(ctx, call)
	return normalizeResult(result, call)
}

func normalizeResult(result ToolResult, call ToolCallRequest) ToolResult {
	if result.ToolCallID == "" {
		result.ToolCallID = call.ToolCallID
	}
	if result.Name == "" {
		result.Name = call.Name
	}
	if result.Status == "" {
		result.Status = StatusSuccess
		if result.Err != nil {
			result.Status = StatusExecutionError
		}
	}
	return result
}

func (s *Service) lookup(name string) (Tool, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	tool, ok := s.registry[name]
	return tool, ok
}

func (s *Service) lockFor(name string) *sync.Mutex {
	s.locksMu.Lock()
	defer s.locksMu.Unlock()

	lock := s.locks[name]
	if lock == nil {
		lock = &sync.Mutex{}
		s.locks[name] = lock
	}
	return lock
}

var errToolNotFound = errors.New("tool not found")
