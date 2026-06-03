package tool

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	toolcontract "github.com/YaHeii/agentGo/internal/tool/contract"
)

const (
	WebSearchToolName              = "websearch"
	webSearchDefaultTimeout        = 30 * time.Second
	maxWebSearchResultContentBytes = 2000
	maxWebSearchOutputBytes        = 30000
)

type WebSearchTool struct {
	baseURL string
	apiKey  string
	client  *http.Client
}

type WebSearchParams struct {
	Query        string   `json:"query"`
	MaxResults   int      `json:"max_results,omitempty"`
	Domains      []string `json:"domains,omitempty"`
	Tags         []string `json:"tags,omitempty"`
	ContentTypes []string `json:"content_types,omitempty"`
	Zone         string   `json:"zone,omitempty"`
	Language     string   `json:"language,omitempty"`
	Providers    []string `json:"providers,omitempty"`
	Freshness    string   `json:"freshness,omitempty"`
	From         string   `json:"from,omitempty"`
	To           string   `json:"to,omitempty"`
}

type anySearchRequest struct {
	Query        string               `json:"query"`
	MaxResults   int                  `json:"max_results,omitempty"`
	Domains      []string             `json:"domains,omitempty"`
	Tags         []string             `json:"tags,omitempty"`
	ContentTypes []string             `json:"content_types,omitempty"`
	Zone         string               `json:"zone,omitempty"`
	Language     string               `json:"language,omitempty"`
	Providers    []string             `json:"providers,omitempty"`
	Constraint   *anySearchConstraint `json:"constraint,omitempty"`
}

type anySearchConstraint struct {
	Freshness string `json:"freshness,omitempty"`
	From      string `json:"from,omitempty"`
	To        string `json:"to,omitempty"`
}

type anySearchResponse struct {
	Results  []anySearchResult `json:"results"`
	Metadata anySearchMetadata `json:"metadata"`
}

type anySearchResult struct {
	Title        string         `json:"title"`
	URL          string         `json:"url"`
	Description  string         `json:"description"`
	Content      string         `json:"content"`
	RawContent   string         `json:"raw_content"`
	Source       string         `json:"source"`
	Score        float64        `json:"score"`
	QualityScore float64        `json:"quality_score"`
	SignalScores map[string]any `json:"signal_scores"`
	PublishedAt  string         `json:"published_at"`
}

type anySearchMetadata struct {
	TotalResults     int    `json:"total_results"`
	SearchTimeMS     int    `json:"search_time_ms"`
	RoutesQueried    int    `json:"routes_queried"`
	RoutesSucceeded  int    `json:"routes_succeeded"`
	Cached           bool   `json:"cached"`
	RequestID        string `json:"request_id"`
	CapabilityErrors []any  `json:"capability_errors"`
}

type anySearchErrorResponse struct {
	Code      int    `json:"code"`
	Symbol    string `json:"symbol"`
	Message   string `json:"message"`
	RequestID string `json:"request_id"`
	Data      any    `json:"data"`
}

func NewWebSearchTool(baseURL string, apiKey string) *WebSearchTool {
	return &WebSearchTool{
		baseURL: strings.TrimSpace(baseURL),
		apiKey:  strings.TrimSpace(apiKey),
		client:  http.DefaultClient,
	}
}

func (t *WebSearchTool) Metadata() toolcontract.Metadata {
	return toolcontract.Metadata{
		Name:        WebSearchToolName,
		Description: "通过 AnySearch 查询外部网页、文档、代码或新闻结果，并返回可继续处理的结构化搜索输出。适合补充本地仓库之外的信息，依赖外部网络访问。",
		Parameters: json.RawMessage(`{
			"type": "object",
			"properties": {
				"query": { "type": "string", "description": "搜索查询文本，应直接描述要查找的主题、问题或事实。" },
				"max_results": { "type": "integer", "description": "期望返回的最大结果数。留空时使用服务端默认值；如果提供，必须在 1 到 100 之间。" },
				"domains": { "type": "array", "description": "可选的搜索域或数据源类别列表，用于限制检索范围，例如特定站点组或垂直域。", "items": { "type": "string" } },
				"tags": { "type": "array", "description": "可选的标签过滤条件，用于进一步缩小结果范围。", "items": { "type": "string" } },
				"content_types": { "type": "array", "description": "可选的内容类型过滤条件，例如网页、文档或其他服务端支持的类型。", "items": { "type": "string" } },
				"zone": { "type": "string", "description": "可选的区域或市场范围，用于偏向某个地理搜索区域。" },
				"language": { "type": "string", "description": "可选的语言过滤条件，用于偏向某种结果语言。" },
				"providers": { "type": "array", "description": "可选的底层搜索提供方列表，用于限制 AnySearch 使用的 provider。", "items": { "type": "string" } },
				"freshness": { "type": "string", "description": "可选的时间新鲜度约束，例如 day、week、month 或 year；会作为 AnySearch constraint 发送。" },
				"from": { "type": "string", "description": "可选的起始日期，通常使用 YYYY-MM-DD 或服务端可识别的日期格式，与 to 共同限定时间范围。" },
				"to": { "type": "string", "description": "可选的结束日期，通常使用 YYYY-MM-DD 或服务端可识别的日期格式，与 from 共同限定时间范围。" }
			},
			"required": ["query"]
		}`),
		Enabled:           true,
		SecurityLevel:     toolcontract.AttentionLevel,
		IsConcurrencySafe: true,
		Requirements:      toolcontract.RequireNetwork,
	}
}

func (t *WebSearchTool) Execute(ctx context.Context, req toolcontract.ToolCallRequest) toolcontract.ToolResult {
	var params WebSearchParams
	if err := json.Unmarshal(req.Arguments, &params); err != nil {
		return toolcontract.ToolResult{
			ToolCallID: req.ToolCallID,
			Name:       req.Name,
			Status:     toolcontract.StatusSystemError,
			Content:    "failed to decode websearch arguments",
			Err:        err,
		}
	}

	searchReq, err := buildAnySearchRequest(params)
	if err != nil {
		return toolcontract.ToolResult{
			ToolCallID: req.ToolCallID,
			Name:       req.Name,
			Status:     toolcontract.StatusValidationFailed,
			Content:    err.Error(),
			Err:        err,
		}
	}
	if strings.TrimSpace(t.baseURL) == "" {
		err := fmt.Errorf("anysearch base URL is required")
		return toolcontract.ToolResult{
			ToolCallID: req.ToolCallID,
			Name:       req.Name,
			Status:     toolcontract.StatusSystemError,
			Content:    err.Error(),
			Err:        err,
		}
	}

	execCtx, cancel := context.WithTimeout(ctx, webSearchDefaultTimeout)
	defer cancel()

	response, errResult, err := t.callAnySearch(execCtx, searchReq)
	if err != nil {
		return toolcontract.ToolResult{
			ToolCallID: req.ToolCallID,
			Name:       req.Name,
			Status:     toolcontract.StatusSystemError,
			Content:    errResult,
			Metadata:   errorMetadata(err),
			Err:        err,
		}
	}

	if len(response.Results) == 0 {
		err := fmt.Errorf("no search results found")
		return toolcontract.ToolResult{
			ToolCallID: req.ToolCallID,
			Name:       req.Name,
			Status:     toolcontract.StatusExecutionError,
			Content:    "no search results found",
			Metadata:   webSearchMetadata(response),
			Err:        err,
		}
	}

	return toolcontract.ToolResult{
		ToolCallID: req.ToolCallID,
		Name:       req.Name,
		Status:     toolcontract.StatusSuccess,
		Content:    renderWebSearchOutput(response),
		Metadata:   webSearchMetadata(response),
	}
}

func buildAnySearchRequest(params WebSearchParams) (anySearchRequest, error) {
	query := strings.TrimSpace(params.Query)
	if query == "" {
		return anySearchRequest{}, fmt.Errorf("query is required")
	}
	if params.MaxResults != 0 && (params.MaxResults < 1 || params.MaxResults > 100) {
		return anySearchRequest{}, fmt.Errorf("max_results must be between 1 and 100")
	}

	searchReq := anySearchRequest{
		Query:        query,
		MaxResults:   params.MaxResults,
		Domains:      cleanStringSlice(params.Domains),
		Tags:         cleanStringSlice(params.Tags),
		ContentTypes: cleanStringSlice(params.ContentTypes),
		Zone:         strings.TrimSpace(params.Zone),
		Language:     strings.TrimSpace(params.Language),
		Providers:    cleanStringSlice(params.Providers),
	}

	constraint := anySearchConstraint{
		Freshness: strings.TrimSpace(params.Freshness),
		From:      strings.TrimSpace(params.From),
		To:        strings.TrimSpace(params.To),
	}
	if constraint.Freshness != "" || constraint.From != "" || constraint.To != "" {
		searchReq.Constraint = &constraint
	}
	return searchReq, nil
}

func (t *WebSearchTool) callAnySearch(ctx context.Context, searchReq anySearchRequest) (anySearchResponse, string, error) {
	body, err := json.Marshal(searchReq)
	if err != nil {
		return anySearchResponse{}, "", err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, t.baseURL, bytes.NewReader(body))
	if err != nil {
		return anySearchResponse{}, "", err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if t.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+t.apiKey)
	}

	resp, err := t.client.Do(httpReq)
	if err != nil {
		return anySearchResponse{}, "", err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(io.LimitReader(resp.Body, maxWebSearchOutputBytes))
	if err != nil {
		return anySearchResponse{}, "", err
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return anySearchResponse{}, renderAnySearchError(resp.StatusCode, data), decodeAnySearchError(resp.StatusCode, data)
	}

	var parsed anySearchResponse
	if err := json.Unmarshal(data, &parsed); err != nil {
		return anySearchResponse{}, "", err
	}
	return parsed, "", nil
}

func renderAnySearchError(statusCode int, data []byte) string {
	var parsed anySearchErrorResponse
	if err := json.Unmarshal(data, &parsed); err != nil {
		return fmt.Sprintf("AnySearch request failed with HTTP %d", statusCode)
	}

	parts := []string{fmt.Sprintf("AnySearch request failed with HTTP %d", statusCode)}
	if parsed.Symbol != "" {
		parts = append(parts, parsed.Symbol)
	}
	if parsed.Message != "" {
		parts = append(parts, parsed.Message)
	}
	if parsed.RequestID != "" {
		parts = append(parts, "request_id="+parsed.RequestID)
	}
	return strings.Join(parts, ": ")
}

func decodeAnySearchError(statusCode int, data []byte) error {
	var parsed anySearchErrorResponse
	if err := json.Unmarshal(data, &parsed); err != nil {
		return &anySearchAPIError{
			StatusCode: statusCode,
		}
	}
	return &anySearchAPIError{
		StatusCode: statusCode,
		Symbol:     parsed.Symbol,
		Message:    parsed.Message,
		RequestID:  parsed.RequestID,
	}
}

func errorMetadata(err error) map[string]any {
	var apiErr *anySearchAPIError
	if !errors.As(err, &apiErr) {
		return nil
	}
	metadata := map[string]any{}
	if apiErr.RequestID != "" {
		metadata["request_id"] = apiErr.RequestID
	}
	return metadata
}

type anySearchAPIError struct {
	StatusCode int
	Symbol     string
	Message    string
	RequestID  string
}

func (e *anySearchAPIError) Error() string {
	if e == nil {
		return "anysearch request failed"
	}
	if e.Symbol != "" || e.Message != "" {
		return fmt.Sprintf("anysearch request failed with HTTP %d: %s: %s", e.StatusCode, e.Symbol, e.Message)
	}
	return fmt.Sprintf("anysearch request failed with HTTP %d", e.StatusCode)
}

func webSearchMetadata(response anySearchResponse) map[string]any {
	return map[string]any{
		"result_count":      len(response.Results),
		"total_results":     response.Metadata.TotalResults,
		"search_time_ms":    response.Metadata.SearchTimeMS,
		"routes_queried":    response.Metadata.RoutesQueried,
		"routes_succeeded":  response.Metadata.RoutesSucceeded,
		"cached":            response.Metadata.Cached,
		"request_id":        response.Metadata.RequestID,
		"capability_errors": response.Metadata.CapabilityErrors,
	}
}

func renderWebSearchOutput(response anySearchResponse) string {
	var builder strings.Builder
	fmt.Fprintf(&builder, "Found %d results", len(response.Results))
	if response.Metadata.TotalResults > 0 {
		fmt.Fprintf(&builder, " (total: %d)", response.Metadata.TotalResults)
	}
	if response.Metadata.RequestID != "" {
		fmt.Fprintf(&builder, " [request_id: %s]", response.Metadata.RequestID)
	}
	builder.WriteString("\n\n")

	for i, result := range response.Results {
		if builder.Len() >= maxWebSearchOutputBytes {
			break
		}
		fmt.Fprintf(&builder, "%d. %s\n", i+1, fallbackTitle(result.Title, result.URL))
		if result.URL != "" {
			fmt.Fprintf(&builder, "URL: %s\n", result.URL)
		}
		if result.Source != "" {
			fmt.Fprintf(&builder, "Source: %s\n", result.Source)
		}
		if result.PublishedAt != "" {
			fmt.Fprintf(&builder, "Published: %s\n", result.PublishedAt)
		}
		if result.Description != "" {
			fmt.Fprintf(&builder, "Description: %s\n", strings.TrimSpace(result.Description))
		}
		content := strings.TrimSpace(result.Content)
		if content == "" {
			content = strings.TrimSpace(result.RawContent)
		}
		if content != "" {
			fmt.Fprintf(&builder, "Content: %s\n", truncateString(content, maxWebSearchResultContentBytes))
		}
		builder.WriteString("\n")
	}

	return truncateString(strings.TrimSpace(builder.String()), maxWebSearchOutputBytes)
}

func fallbackTitle(title string, url string) string {
	if strings.TrimSpace(title) != "" {
		return strings.TrimSpace(title)
	}
	if strings.TrimSpace(url) != "" {
		return strings.TrimSpace(url)
	}
	return "Untitled result"
}

func cleanStringSlice(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	cleaned := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		cleaned = append(cleaned, value)
	}
	if len(cleaned) == 0 {
		return nil
	}
	return cleaned
}

func truncateString(value string, maxBytes int) string {
	if maxBytes <= 0 || len(value) <= maxBytes {
		return value
	}
	return value[:maxBytes] + "...[truncated]"
}
