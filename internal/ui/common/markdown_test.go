package common

import "testing"

func TestMarkdownRendererCachesByWidthAndMode(t *testing.T) {
	InvalidateMarkdownRendererCache()

	normalA := MarkdownRenderer(80)
	normalB := MarkdownRenderer(80)
	quiet := QuietMarkdownRenderer(80)
	narrow := MarkdownRenderer(40)

	if normalA == nil {
		t.Fatal("expected normal renderer")
	}
	if normalA != normalB {
		t.Fatal("expected same renderer instance for same width and mode")
	}
	if normalA == quiet {
		t.Fatal("expected quiet renderer to use a separate instance")
	}
	if normalA == narrow {
		t.Fatal("expected different width to use a separate renderer instance")
	}
}

func TestInvalidateMarkdownRendererCacheDropsRendererInstances(t *testing.T) {
	InvalidateMarkdownRendererCache()

	before := MarkdownRenderer(80)
	InvalidateMarkdownRendererCache()
	after := MarkdownRenderer(80)

	if before == after {
		t.Fatal("expected cache invalidation to rebuild renderer")
	}
}

func TestLockMarkdownRendererReturnsStableMutexPerRenderer(t *testing.T) {
	InvalidateMarkdownRendererCache()

	renderer := MarkdownRenderer(80)
	first := LockMarkdownRenderer(renderer)
	second := LockMarkdownRenderer(renderer)
	other := LockMarkdownRenderer(QuietMarkdownRenderer(80))

	if first == nil || second == nil || other == nil {
		t.Fatal("expected renderer locks")
	}
	if first != second {
		t.Fatal("expected stable mutex for the same renderer")
	}
	if first == other {
		t.Fatal("expected different renderer to use a different mutex")
	}
}
