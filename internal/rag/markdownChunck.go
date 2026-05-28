package rag

import (
	"errors"
	"fmt"
	"strings"
	"sync"

	tiktoken "github.com/pkoukk/tiktoken-go"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
)

// SemanticBlock is the smallest semantic retrieval unit extracted from Markdown.
type SemanticBlock struct {
	NodeType string
	Level    int
	Text     string
	Path     []string
}

// ChunkDraft is the normalized chunk content before persistence and embedding.
type ChunkDraft struct {
	Text        string
	TokenLength int
}

const defaultChunkEncoding = "cl100k_base"

type tokenCounterInterface interface {
	Count(text string) (int, error)
}

type tokenCounterFunc func(text string) (int, error)

func (f tokenCounterFunc) Count(text string) (int, error) {
	return f(text)
}

var (
	tokenCounter        tokenCounterInterface = defaultTokenCounter
	defaultTokenCounter                       = &tiktokenCounter{encodingName: defaultChunkEncoding}
)

func FlattenAST(source []byte) []SemanticBlock {
	root := goldmark.New().Parser().Parse(text.NewReader(source))

	var blocks []SemanticBlock
	var currentHeaders []string

	ast.Walk(root, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}

		switch node := n.(type) {
		case *ast.Heading:
			title := strings.TrimSpace(renderInlineText(node, source))
			if title == "" {
				return ast.WalkSkipChildren, nil
			}
			if node.Level <= len(currentHeaders) {
				currentHeaders = currentHeaders[:node.Level-1]
			}
			currentHeaders = append(currentHeaders, title)
			return ast.WalkSkipChildren, nil
		case *ast.Paragraph:
			appendBlock(&blocks, "paragraph", normalizeWhitespace(renderInlineText(node, source)), currentHeaders, 0)
			return ast.WalkSkipChildren, nil
		case *ast.List:
			appendBlock(&blocks, "list", renderList(node, source), currentHeaders, 0)
			return ast.WalkSkipChildren, nil
		case *ast.Blockquote:
			appendBlock(&blocks, "blockquote", normalizeWhitespace(renderInlineText(node, source)), currentHeaders, 0)
			return ast.WalkSkipChildren, nil
		case *ast.FencedCodeBlock:
			appendBlock(&blocks, "fenced_code", renderFencedCodeBlock(node, source), currentHeaders, 0)
			return ast.WalkSkipChildren, nil
		case *ast.CodeBlock:
			appendBlock(&blocks, "code_block", strings.TrimSpace(string(node.Lines().Value(source))), currentHeaders, 0)
			return ast.WalkSkipChildren, nil
		}

		return ast.WalkContinue, nil
	})

	return blocks
}

func AssembleChunks(blocks []SemanticBlock, maxLen int) ([]ChunkDraft, error) {
	if maxLen <= 0 {
		return nil, nil
	}

	var chunks []ChunkDraft
	var currentText strings.Builder
	var currentPath string

	flush := func() error {
		if currentText.Len() == 0 {
			return nil
		}
		text := strings.TrimSpace(currentText.String())
		if text == "" {
			currentText.Reset()
			currentPath = ""
			return nil
		}
		tokenLen, err := tokenCounter.Count(text)
		if err != nil {
			return err
		}
		chunks = append(chunks, ChunkDraft{
			Text:        text,
			TokenLength: tokenLen,
		})
		currentText.Reset()
		currentPath = ""
		return nil
	}

	for _, block := range blocks {
		segments, err := splitBlock(block, maxLen)
		if err != nil {
			return nil, err
		}
		for _, segment := range segments {
			if segment.text == "" {
				continue
			}

			candidateText, nextPath := buildCandidateChunkText(currentText.String(), currentPath, segment)
			candidateTokens, err := tokenCounter.Count(candidateText)
			if err != nil {
				return nil, err
			}
			if currentText.Len() > 0 && (candidateTokens > maxLen || currentPath != nextPath) {
				if err := flush(); err != nil {
					return nil, err
				}
				candidateText, nextPath = buildCandidateChunkText("", "", segment)
				candidateTokens, err = tokenCounter.Count(candidateText)
				if err != nil {
					return nil, err
				}
			}
			if candidateTokens > maxLen {
				return nil, fmt.Errorf("chunk segment exceeds max token length: %d > %d", candidateTokens, maxLen)
			}
			currentText.Reset()
			currentText.WriteString(candidateText)
			currentPath = nextPath
		}
	}

	if err := flush(); err != nil {
		return nil, err
	}
	return chunks, nil
}

type blockSegment struct {
	path string
	text string
}

func appendBlock(blocks *[]SemanticBlock, nodeType string, content string, path []string, level int) {
	content = strings.TrimSpace(content)
	if content == "" {
		return
	}
	*blocks = append(*blocks, SemanticBlock{
		NodeType: nodeType,
		Level:    level,
		Text:     content,
		Path:     append([]string(nil), path...),
	})
}

func splitBlock(block SemanticBlock, maxLen int) ([]blockSegment, error) {
	path := formatPath(block.Path)
	text := strings.TrimSpace(block.Text)
	if text == "" {
		return nil, nil
	}

	parts, err := splitTextByTokens(text, maxLen, path)
	if err != nil {
		return nil, err
	}

	segments := make([]blockSegment, 0, len(parts))
	for _, part := range parts {
		if strings.TrimSpace(part) == "" {
			continue
		}
		segments = append(segments, blockSegment{
			path: path,
			text: part,
		})
	}
	return segments, nil
}

func splitTextByTokens(text string, maxLen int, path string) ([]string, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil, nil
	}
	candidate := text
	if path != "" {
		candidate = path + "\n\n" + text
	}
	tokenLen, err := tokenCounter.Count(candidate)
	if err != nil {
		return nil, err
	}
	if tokenLen <= maxLen {
		return []string{text}, nil
	}

	runes := []rune(text)
	if len(runes) <= 1 {
		return []string{text}, nil
	}

	mid := len(runes) / 2
	left, err := splitTextByTokens(string(runes[:mid]), maxLen, path)
	if err != nil {
		return nil, err
	}
	right, err := splitTextByTokens(string(runes[mid:]), maxLen, path)
	if err != nil {
		return nil, err
	}
	return append(left, right...), nil
}

func buildCandidateChunkText(currentText string, currentPath string, segment blockSegment) (string, string) {
	if currentText == "" {
		if segment.path == "" {
			return segment.text, ""
		}
		return segment.path + "\n\n" + segment.text, segment.path
	}
	if currentPath != segment.path {
		if segment.path == "" {
			return segment.text, ""
		}
		return segment.path + "\n\n" + segment.text, segment.path
	}
	return currentText + "\n\n" + segment.text, currentPath
}

func formatPath(path []string) string {
	if len(path) == 0 {
		return ""
	}
	return fmt.Sprintf("[%s]", strings.Join(path, " > "))
}

func normalizeWhitespace(text string) string {
	lines := strings.Split(text, "\n")
	for i := range lines {
		lines[i] = strings.TrimSpace(lines[i])
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

func renderFencedCodeBlock(node *ast.FencedCodeBlock, source []byte) string {
	content := strings.TrimSpace(string(node.Lines().Value(source)))
	lang := strings.TrimSpace(string(node.Language(source)))
	if lang == "" {
		return content
	}
	if content == "" {
		return fmt.Sprintf("[code:%s]", lang)
	}
	return fmt.Sprintf("[code:%s]\n%s", lang, content)
}

func renderList(node *ast.List, source []byte) string {
	var lines []string
	index := node.Start
	if index == 0 {
		index = 1
	}

	for item := node.FirstChild(); item != nil; item = item.NextSibling() {
		listItem, ok := item.(*ast.ListItem)
		if !ok {
			continue
		}
		text := normalizeWhitespace(renderInlineText(listItem, source))
		if text == "" {
			continue
		}
		prefix := "-"
		if node.IsOrdered() {
			prefix = fmt.Sprintf("%d%c", index, node.Marker)
			index++
		}
		lines = append(lines, fmt.Sprintf("%s %s", prefix, text))
	}

	return strings.Join(lines, "\n")
}

func renderInlineText(node ast.Node, source []byte) string {
	var buf strings.Builder
	ast.Walk(node, func(current ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}

		switch typed := current.(type) {
		case *ast.Text:
			buf.Write(typed.Segment.Value(source))
			if typed.HardLineBreak() || typed.SoftLineBreak() {
				buf.WriteByte('\n')
			}
		case *ast.String:
			buf.Write(typed.Value)
		case *ast.CodeSpan:
			buf.WriteString(normalizeWhitespace(string(typed.Text(source))))
			return ast.WalkSkipChildren, nil
		}

		return ast.WalkContinue, nil
	})
	return strings.TrimSpace(buf.String())
}

type tiktokenCounter struct {
	encodingName string
	once         sync.Once
	tokenizer    *tiktoken.Tiktoken
	err          error
}

func (c *tiktokenCounter) Count(text string) (int, error) {
	c.once.Do(func() {
		c.tokenizer, c.err = tiktoken.GetEncoding(c.encodingName)
		if c.err != nil {
			c.err = fmt.Errorf("load tiktoken encoding %q: %w", c.encodingName, c.err)
		}
	})
	if c.err != nil {
		return 0, c.err
	}
	if c.tokenizer == nil {
		return 0, errors.New("tiktoken tokenizer is nil")
	}
	return len(c.tokenizer.Encode(text, nil, nil)), nil
}
