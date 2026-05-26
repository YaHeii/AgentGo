package xchroma

import (
	"fmt"
	"image/color"
	"io"

	"github.com/alecthomas/chroma/v2"
	"github.com/charmbracelet/lipgloss"
)

// Formatter returns a Chroma formatter that applies token styles through
// lipgloss so Glamour code fences render consistently inside the TUI.
func Formatter(bgColor color.Color, processValue func(string) string) chroma.Formatter {
	return chroma.FormatterFunc(func(w io.Writer, style *chroma.Style, it chroma.Iterator) error {
		for token := it(); token != chroma.EOF; token = it() {
			value := token.Value
			if processValue != nil {
				value = processValue(value)
			}

			entry := style.Get(token.Type)
			if entry.IsZero() {
				if _, err := fmt.Fprint(w, value); err != nil {
					return err
				}
				continue
			}

			renderStyle := lipgloss.NewStyle()
			if bgColor != nil {
				if rgba, ok := color.RGBAModel.Convert(bgColor).(color.RGBA); ok {
					renderStyle = renderStyle.Background(lipgloss.Color(fmt.Sprintf("#%02x%02x%02x", rgba.R, rgba.G, rgba.B)))
				}
			}
			if entry.Bold == chroma.Yes {
				renderStyle = renderStyle.Bold(true)
			}
			if entry.Underline == chroma.Yes {
				renderStyle = renderStyle.Underline(true)
			}
			if entry.Italic == chroma.Yes {
				renderStyle = renderStyle.Italic(true)
			}
			if entry.Colour.IsSet() {
				renderStyle = renderStyle.Foreground(lipgloss.Color(entry.Colour.String()))
			}

			if _, err := fmt.Fprint(w, renderStyle.Render(value)); err != nil {
				return err
			}
		}
		return nil
	})
}
