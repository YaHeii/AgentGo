package common

import (
	"image/color"
	"sync"

	"charm.land/glamour/v2"
	"charm.land/glamour/v2/ansi"
	"github.com/YaHeii/agentGo/internal/ui/xchroma"
	"github.com/alecthomas/chroma/v2/formatters"
)

const formatterName = "crush"

func init() {
	var zero color.Color
	formatters.Register(formatterName, xchroma.Formatter(zero, nil))
}

type markdownRenderable interface {
	Render(string) (string, error)
}

var (
	mdCacheMu    sync.Mutex
	mdCache      = map[int]*glamour.TermRenderer{}
	quietMDCache = map[int]*glamour.TermRenderer{}
)

func MarkdownRenderer(width int) *glamour.TermRenderer {
	mdCacheMu.Lock()
	defer mdCacheMu.Unlock()

	if renderer, ok := mdCache[width]; ok {
		return renderer
	}

	renderer, _ := glamour.NewTermRenderer(
		glamour.WithStyles(markdownStyle()),
		glamour.WithWordWrap(width),
		glamour.WithChromaFormatter(formatterName),
	)
	mdCache[width] = renderer
	return renderer
}

func QuietMarkdownRenderer(width int) *glamour.TermRenderer {
	mdCacheMu.Lock()
	defer mdCacheMu.Unlock()

	if renderer, ok := quietMDCache[width]; ok {
		return renderer
	}

	renderer, _ := glamour.NewTermRenderer(
		glamour.WithStyles(quietMarkdownStyle()),
		glamour.WithWordWrap(width),
		glamour.WithChromaFormatter(formatterName),
	)
	quietMDCache[width] = renderer
	return renderer
}

func InvalidateMarkdownRendererCache() {
	mdCacheMu.Lock()
	defer mdCacheMu.Unlock()
	rendererLocksMu.Lock()
	defer rendererLocksMu.Unlock()

	mdCache = map[int]*glamour.TermRenderer{}
	quietMDCache = map[int]*glamour.TermRenderer{}
	rendererLocks = map[markdownRenderable]*sync.Mutex{}
}

var (
	rendererLocksMu sync.Mutex
	rendererLocks   = map[markdownRenderable]*sync.Mutex{}
)

func LockMarkdownRenderer(renderer markdownRenderable) *sync.Mutex {
	rendererLocksMu.Lock()
	defer rendererLocksMu.Unlock()

	if mu, ok := rendererLocks[renderer]; ok {
		return mu
	}
	mu := &sync.Mutex{}
	rendererLocks[renderer] = mu
	return mu
}

func markdownStyle() ansi.StyleConfig {
	return ansi.StyleConfig{
		Document: ansi.StyleBlock{
			StylePrimitive: ansi.StylePrimitive{
				BlockPrefix: "",
				BlockSuffix: "",
			},
		},
		BlockQuote: ansi.StyleBlock{
			StylePrimitive: ansi.StylePrimitive{
				Color:  stringPtr("244"),
				Prefix: "│ ",
			},
		},
		List: ansi.StyleList{
			LevelIndent: 2,
		},
		Heading: ansi.StyleBlock{
			StylePrimitive: ansi.StylePrimitive{
				Bold:        boolPtr(true),
				BlockSuffix: "\n",
			},
		},
		Link: ansi.StylePrimitive{
			Color:     stringPtr("39"),
			Underline: boolPtr(true),
		},
		Strong: ansi.StylePrimitive{
			Bold: boolPtr(true),
		},
		Emph: ansi.StylePrimitive{
			Italic: boolPtr(true),
		},
		Code: ansi.StyleBlock{
			StylePrimitive: ansi.StylePrimitive{
				Color: stringPtr("212"),
			},
		},
		CodeBlock: ansi.StyleCodeBlock{
			StyleBlock: ansi.StyleBlock{
				StylePrimitive: ansi.StylePrimitive{
					BlockPrefix: "\n",
					BlockSuffix: "\n",
				},
			},
			Chroma: &ansi.Chroma{
				Text: ansi.StylePrimitive{
					Color: stringPtr("252"),
				},
			},
		},
	}
}

func quietMarkdownStyle() ansi.StyleConfig {
	style := markdownStyle()
	clearColors(&style)
	return style
}

func clearColors(style *ansi.StyleConfig) {
	style.Document.Color = nil
	style.BlockQuote.Color = nil
	style.Code.Color = nil
	style.Link.Color = nil
	if style.CodeBlock.Chroma != nil {
		style.CodeBlock.Chroma = &ansi.Chroma{}
	}
}

func stringPtr(value string) *string {
	return &value
}

func boolPtr(value bool) *bool {
	return &value
}
