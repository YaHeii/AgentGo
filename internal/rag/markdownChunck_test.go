package rag

import (
	"errors"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestFlattenASTPreservesRetrievableBlocks(t *testing.T) {
	source := []byte(`# Guide

Paragraph with [link](https://example.com) and *emphasis* plus ` + "`code`" + `.

- first item
- second item

> quote line 1
> quote line 2

` + "```go" + `
fmt.Println("hi")
` + "```" + `

    indented code
`)

	blocks := FlattenAST(source)
	texts := make([]string, 0, len(blocks))
	for _, block := range blocks {
		if block.NodeType == "Heading" {
			t.Fatalf("expected headings to become context only, got standalone heading block: %#v", block)
		}
		if len(block.Path) != 1 || block.Path[0] != "Guide" {
			t.Fatalf("expected heading path [Guide], got %#v", block.Path)
		}
		texts = append(texts, block.Text)
	}

	want := []string{
		"Paragraph with link and emphasis plus code.",
		"- first item\n- second item",
		"quote line 1\nquote line 2",
		"[code:go]\nfmt.Println(\"hi\")",
		"indented code",
	}

	if len(texts) != len(want) {
		t.Fatalf("expected %d blocks, got %d: %#v", len(want), len(texts), texts)
	}
	for i := range want {
		if texts[i] != want[i] {
			t.Fatalf("block %d mismatch\nwant: %q\ngot:  %q", i, want[i], texts[i])
		}
	}
}

func TestAssembleChunksSplitsOversizedBlock(t *testing.T) {
	t.Cleanup(func() {
		tokenCounter = defaultTokenCounter
	})
	tokenCounter = tokenCounterFunc(func(text string) (int, error) {
		return len(text), nil
	})

	blocks := []SemanticBlock{
		{Text: "abcdefghijk"},
	}

	chunks, err := AssembleChunks(blocks, 5)
	if err != nil {
		t.Fatalf("AssembleChunks returned error: %v", err)
	}
	if len(chunks) != 3 {
		t.Fatalf("expected 3 chunks, got %d", len(chunks))
	}

	for i, chunk := range chunks {
		if got := chunk.TokenLength; got > 5 {
			t.Fatalf("chunk %d exceeds hard maxLen: %d", i, got)
		}
	}
}

func TestAssembleChunksUsesHeadingPathWithoutStandaloneHeadingChunk(t *testing.T) {
	t.Cleanup(func() {
		tokenCounter = defaultTokenCounter
	})
	tokenCounter = tokenCounterFunc(func(text string) (int, error) {
		return len(text), nil
	})

	blocks := FlattenAST([]byte(`# Guide

first paragraph

second paragraph
`))

	chunks, err := AssembleChunks(blocks, 100)
	if err != nil {
		t.Fatalf("AssembleChunks returned error: %v", err)
	}
	if len(chunks) != 1 {
		t.Fatalf("expected 1 chunk, got %d", len(chunks))
	}

	chunkText := chunks[0].Text
	if strings.Count(chunkText, "[Guide]") != 1 {
		t.Fatalf("expected heading context once, got %q", chunkText)
	}
	if !strings.Contains(chunkText, "first paragraph") || !strings.Contains(chunkText, "second paragraph") {
		t.Fatalf("expected chunk to contain both paragraphs, got %q", chunkText)
	}
}

func TestAssembleChunksUsesTokenCounterInsteadOfRuneCount(t *testing.T) {
	t.Cleanup(func() {
		tokenCounter = defaultTokenCounter
	})
	tokenCounter = tokenCounterFunc(func(text string) (int, error) {
		return utf8.RuneCountInString(text) * 2, nil
	})

	chunks, err := AssembleChunks([]SemanticBlock{{Text: "你好你好"}}, 4)
	if err != nil {
		t.Fatalf("AssembleChunks returned error: %v", err)
	}
	if len(chunks) != 2 {
		t.Fatalf("expected 2 chunks after token-based split, got %d", len(chunks))
	}
	for i, chunk := range chunks {
		if chunk.TokenLength > 4 {
			t.Fatalf("chunk %d token length exceeds limit: %d", i, chunk.TokenLength)
		}
	}
}

func TestAssembleChunksReturnsTokenizerError(t *testing.T) {
	t.Cleanup(func() {
		tokenCounter = defaultTokenCounter
	})
	tokenCounter = tokenCounterFunc(func(text string) (int, error) {
		return 0, errors.New("tokenizer unavailable")
	})

	_, err := AssembleChunks([]SemanticBlock{{Text: "hello"}}, 4)
	if err == nil {
		t.Fatal("expected tokenizer error")
	}
	if !strings.Contains(err.Error(), "tokenizer unavailable") {
		t.Fatalf("expected tokenizer error to bubble up, got %v", err)
	}
}
