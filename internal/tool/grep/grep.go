package grep

// https://zhuanlan.zhihu.com/p/2028510583213835970
// as the article says
// the greptool is very important, we decide not to use the embeding,
// just grep to serch the main imformation, can use the sub agent or crop the imformation to Prevent Context Overflow
//1. first to search Ripgrep then grep
//2. Reads the first 512 bytes of the file to detect its MIME type, automatically skipping non-text files such as images and executables.
//3. Supports `.gitignore`. When performing native searches, hidden files (files starting with a dot) are also skipped.
//4. Dual Truncation Mechanism:
//Inline Truncation: Single lines exceeding 500 characters are truncated to prevent excessively long log entries from overwhelming the LLM's context window.
//Global Truncation: Processing automatically halts—returning `Truncated: true`—when the number of matches exceeds a predefined limit (e.g., 100 or 200 items).
//Metadata Feedback: Returns the `NumberOfMatches` count and `Truncated` status, enabling the Agent to determine whether it needs to narrow the search scope or utilize a more specific path.
// safety
//Timeout Control: Enforces the use of `context.WithTimeout` to prevent process hangs caused by searching through massive projects from the root directory.
//On-Demand Reading: The native implementation utilizes `bufio.Scanner` to read files line by line, avoiding the need to load large files entirely into memory at once.
//Result Sorting: Sorts results in descending order based on the file modification time (`modTime`), ensuring that the Agent prioritizes the most recently updated code.

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	toolcontract "github.com/YaHeii/agentGo/internal/tool/contract"
)

const (
	GrepToolName        = "grep"
	MaxGrepMatches      = 100 // Limit the maximum number of matches returned to the LLM.
	MaxLineContentWidth = 500 // Limit Single-Line Text Width
	GrepDefaultTimeout  = 30 * time.Second
)

type GrepTool struct {
	projectRoot string    // Enforced Security Boundary
	searchCache *sync.Map // Cache compiled regular expressions.
	globCache   *sync.Map // Cache File Filtering Regex
}

// NewGrepTool initialization. The project root directory must be passed as a security boundary.
func NewGrepTool(projectRoot string) *GrepTool {
	absRoot, _ := filepath.Abs(projectRoot)
	return &GrepTool{
		projectRoot: absRoot,
		searchCache: &sync.Map{},
		globCache:   &sync.Map{},
	}
}

// Metadata implements interfaces to describe capabilities to the model and policies to the system.
func (t *GrepTool) Metadata() toolcontract.Metadata {
	return toolcontract.Metadata{
		Name:        GrepToolName,
		Description: "在项目文件内容中搜索正则表达式。支持 .gitignore 过滤并自动跳过二进制文件。",
		Parameters: json.RawMessage(`{
			"type": "object",
			"properties": {
				"pattern": { "type": "string", "description": "正则表达式搜索模式" },
				"path": { "type": "string", "description": "搜索起始目录，必须在项目范围内" },
				"include": { "type": "string", "description": "包含的文件模式，如 '*.go' 或 '*.{js,ts}'" },
				"literal_text": { "type": "boolean", "description": "是否关闭正则，将 pattern 视为普通字符串" }
			},
			"required": ["pattern"]
			}`),
		Enabled:           true,
		SecurityLevel:     toolcontract.SafeLevel,
		IsConcurrencySafe: true,
		Requirements:      toolcontract.RequireWorkingDir,
	}
}

func (t *GrepTool) Execute(ctx context.Context, req toolcontract.ToolCallRequest) toolcontract.ToolResult {
	var params struct {
		Pattern     string `json:"pattern"`
		Path        string `json:"path"`
		Include     string `json:"include"`
		LiteralText bool   `json:"literal_text"`
	}
	if err := json.Unmarshal(req.Arguments, &params); err != nil {
		return toolcontract.ToolResult{
			ToolCallID: req.ToolCallID,
			Name:       req.Name,
			Status:     toolcontract.StatusSystemError,
			Content:    "Tool argument decoding drifted from service validation.",
			Err:        err,
		}
	}

	searchPath, err := resolveSearchPath(req.Context.WorkingDir, params.Path)
	if err != nil {
		return toolcontract.ToolResult{
			ToolCallID: req.ToolCallID,
			Name:       req.Name,
			Status:     toolcontract.StatusValidationFailed,
			Content:    "Access Denied: The search path must be within the project directory.",
			Err:        err,
		}
	}

	resolvedSearchPath, err := filepath.EvalSymlinks(searchPath)
	if err != nil {
		return toolcontract.ToolResult{
			ToolCallID: req.ToolCallID,
			Name:       req.Name,
			Status:     toolcontract.StatusValidationFailed,
			Content:    "Access Denied: The search path must be within the project directory.",
			Err:        err,
		}
	}
	if !isWithinRoot(resolvedSearchPath, t.projectRoot) {
		return toolcontract.ToolResult{
			ToolCallID: req.ToolCallID,
			Name:       req.Name,
			Status:     toolcontract.StatusValidationFailed,
			Content:    "Access Denied: The search path must be within the project directory.",
			Err:        fmt.Errorf("search path %q escapes project root %q", resolvedSearchPath, t.projectRoot),
		}
	}

	pattern := params.Pattern
	if params.LiteralText {
		pattern = regexp.QuoteMeta(pattern)
	}

	execCtx, cancel := context.WithTimeout(ctx, GrepDefaultTimeout)
	defer cancel()

	matches, truncated, err := t.runSearch(execCtx, pattern, resolvedSearchPath, params.Include)
	if err != nil {
		return toolcontract.ToolResult{
			ToolCallID: req.ToolCallID,
			Name:       req.Name,
			Status:     toolcontract.StatusSystemError,
			Err:        fmt.Errorf("grep system error: %w", err),
		}
	}

	status := toolcontract.StatusSuccess
	errResult := error(nil)
	if len(matches) == 0 {
		status = toolcontract.StatusExecutionError
		errResult = fmt.Errorf("no matches found")
	}

	return toolcontract.ToolResult{
		ToolCallID: req.ToolCallID,
		Name:       req.Name,
		Status:     status,
		Content:    t.renderOutput(matches, truncated),
		Metadata: map[string]any{
			"match_count": len(matches),
			"truncated":   truncated,
		},
		Err: errResult,
	}
}

func resolveSearchPath(workingDir, requestedPath string) (string, error) {
	base := workingDir
	if strings.TrimSpace(base) == "" {
		base = "."
	}

	if strings.TrimSpace(requestedPath) == "" {
		return filepath.Abs(base)
	}
	if filepath.IsAbs(requestedPath) {
		return filepath.Abs(requestedPath)
	}
	return filepath.Abs(filepath.Join(base, requestedPath))
}

type grepMatch struct {
	path     string
	modTime  time.Time
	lineNum  int
	charNum  int
	lineText string
}

func (t *GrepTool) runSearch(ctx context.Context, pattern, root, include string) ([]grepMatch, bool, error) {
	matches, err := t.searchWithRipgrep(ctx, pattern, root, include)
	if err != nil {
		return t.searchWithNative(ctx, pattern, root, include)
	}

	sort.Slice(matches, func(i, j int) bool {
		return matches[i].modTime.After(matches[j].modTime)
	})

	truncated := len(matches) > MaxGrepMatches
	if truncated {
		matches = matches[:MaxGrepMatches]
	}
	return matches, truncated, nil
}

func (t *GrepTool) searchWithNative(ctx context.Context, pattern, root, include string) ([]grepMatch, bool, error) {
	re, err := t.getRegex(pattern, false)
	if err != nil {
		return nil, false, err
	}

	var includeRe *regexp.Regexp
	if include != "" {
		includeRe, _ = t.getRegex(globToRegex(include), true)
	}

	ignores, err := loadIgnoreMatchers(root)
	if err != nil {
		return nil, false, err
	}

	var matches []grepMatch
	err = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if err != nil || d.IsDir() {
			if d != nil && shouldIgnorePath(root, path, d.IsDir(), ignores) {
				return filepath.SkipDir
			}
			if d.IsDir() && (strings.HasPrefix(d.Name(), ".") && d.Name() != ".") {
				return filepath.SkipDir
			}
			return nil
		}

		if shouldIgnorePath(root, path, false, ignores) {
			return nil
		}

		if includeRe != nil && !includeRe.MatchString(path) {
			return nil
		}
		if strings.HasPrefix(filepath.Base(path), ".") {
			return nil
		}

		ok, line, char, text, _ := t.checkFile(path, re)
		if ok {
			info, _ := d.Info()
			matches = append(matches, grepMatch{
				path: path, modTime: info.ModTime(), lineNum: line, charNum: char, lineText: text,
			})
			if len(matches) >= MaxGrepMatches+1 {
				return io.EOF
			}
		}
		return nil
	})

	if err != nil && err != io.EOF {
		return nil, false, err
	}

	truncated := len(matches) > MaxGrepMatches
	sort.Slice(matches, func(i, j int) bool {
		return matches[i].modTime.After(matches[j].modTime)
	})
	return matches, truncated, nil
}

func (t *GrepTool) checkFile(path string, re *regexp.Regexp) (bool, int, int, string, error) {
	f, err := os.Open(path)
	if err != nil {
		return false, 0, 0, "", err
	}
	defer f.Close()

	head := make([]byte, 512)
	n, _ := f.Read(head)
	if !isText(head[:n]) {
		return false, 0, 0, "", nil
	}
	f.Seek(0, 0)

	scanner := bufio.NewScanner(f)
	buf := make([]byte, 256*1024)
	scanner.Buffer(buf, 256*1024)

	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := scanner.Text()
		if loc := re.FindStringIndex(line); loc != nil {
			return true, lineNum, loc[0] + 1, strings.TrimSpace(line), nil
		}
	}
	return false, 0, 0, "", nil
}

func (t *GrepTool) searchWithRipgrep(ctx context.Context, pattern, path, include string) ([]grepMatch, error) {
	rgPath, err := exec.LookPath("rg")
	if err != nil {
		return nil, err
	}

	args := []string{"--json", "--max-columns", "500", "--ignore-case", "-e", pattern, path}
	if include != "" {
		args = append(args, "-g", include)
	}

	cmd := exec.CommandContext(ctx, rgPath, args...)
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			return nil, nil
		}
		return nil, err
	}

	return parseRipgrepJSON(stdout.Bytes(), path), nil
}

func isText(data []byte) bool {
	contentType := http.DetectContentType(data)
	return strings.HasPrefix(contentType, "text/") || strings.Contains(contentType, "javascript") || strings.Contains(contentType, "json")
}

func (t *GrepTool) getRegex(p string, isGlob bool) (*regexp.Regexp, error) {
	cache := t.searchCache
	if isGlob {
		cache = t.globCache
	}
	if val, ok := cache.Load(p); ok {
		return val.(*regexp.Regexp), nil
	}
	re, err := regexp.Compile(p)
	if err != nil {
		return nil, err
	}
	cache.Store(p, re)
	return re, nil
}

func (t *GrepTool) renderOutput(matches []grepMatch, truncated bool) string {
	if len(matches) == 0 {
		return "not find Matches"
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "find %d ：Matches \n", len(matches))
	lastFile := ""
	for _, m := range matches {
		relPath, _ := filepath.Rel(t.projectRoot, m.path)
		if relPath != lastFile {
			sb.WriteString("\n" + relPath + ":\n")
			lastFile = relPath
		}
		text := m.lineText
		if len(text) > MaxLineContentWidth {
			text = text[:MaxLineContentWidth] + "..."
		}
		fmt.Fprintf(&sb, "  第 %d 行: %s\n", m.lineNum, text)
	}
	if truncated {
		sb.WriteString("\n(Results truncated; please try narrowing your search scope or path.)")
	}
	return sb.String()
}

func globToRegex(glob string) string {
	r := strings.NewReplacer(".", "\\.", "*", ".*", "?", ".", "{", "(", "}", ")", ",", "|")
	return "^" + r.Replace(glob) + "$"
}

type ignoreMatcher struct {
	dirOnly bool
	regex   *regexp.Regexp
}

func loadIgnoreMatchers(root string) ([]ignoreMatcher, error) {
	ignoreFile := filepath.Join(root, ".gitignore")
	data, err := os.ReadFile(ignoreFile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	lines := strings.Split(string(data), "\n")
	matchers := make([]ignoreMatcher, 0, len(lines))
	for _, line := range lines {
		pattern := strings.TrimSpace(line)
		if pattern == "" || strings.HasPrefix(pattern, "#") {
			continue
		}

		dirOnly := strings.HasSuffix(pattern, "/")
		pattern = strings.TrimSuffix(pattern, "/")
		if pattern == "" {
			continue
		}

		re, err := regexp.Compile(globToRegex(pattern))
		if err != nil {
			return nil, err
		}
		matchers = append(matchers, ignoreMatcher{
			dirOnly: dirOnly,
			regex:   re,
		})
	}
	return matchers, nil
}

func shouldIgnorePath(root, path string, isDir bool, matchers []ignoreMatcher) bool {
	if len(matchers) == 0 {
		return false
	}

	relPath, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	relPath = filepath.ToSlash(relPath)
	for _, matcher := range matchers {
		if matcher.dirOnly && !isDir {
			continue
		}
		if matcher.regex.MatchString(relPath) || matcher.regex.MatchString(filepath.Base(relPath)) {
			return true
		}
	}
	return false
}

func parseRipgrepJSON(data []byte, root string) []grepMatch {
	type rgPath struct {
		Text string `json:"text"`
	}
	type rgLines struct {
		Text string `json:"text"`
	}
	type rgSubmatch struct {
		Start int `json:"start"`
	}
	type rgMatchMessage struct {
		Type string `json:"type"`
		Data struct {
			Path       rgPath       `json:"path"`
			Lines      rgLines      `json:"lines"`
			LineNumber int          `json:"line_number"`
			Submatches []rgSubmatch `json:"submatches"`
		} `json:"data"`
	}

	lines := bytes.Split(data, []byte{'\n'})
	out := make([]grepMatch, 0, len(lines))
	for _, line := range lines {
		line = bytes.TrimSpace(line)
		if len(line) == 0 {
			continue
		}

		var msg rgMatchMessage
		if err := json.Unmarshal(line, &msg); err != nil {
			continue
		}
		if msg.Type != "match" {
			continue
		}

		charNum := 1
		if len(msg.Data.Submatches) > 0 {
			charNum = msg.Data.Submatches[0].Start + 1
		}
		matchPath := msg.Data.Path.Text
		if root != "" && !filepath.IsAbs(matchPath) {
			matchPath = filepath.Join(root, filepath.FromSlash(matchPath))
		}

		var modTime time.Time
		if info, err := os.Stat(matchPath); err == nil {
			modTime = info.ModTime()
		}
		out = append(out, grepMatch{
			path:     matchPath,
			modTime:  modTime,
			lineNum:  msg.Data.LineNumber,
			charNum:  charNum,
			lineText: strings.TrimSpace(msg.Data.Lines.Text),
		})
	}
	return out
}

func isWithinRoot(path string, root string) bool {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
}
