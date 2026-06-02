package model

import (
	"image"

	tea "charm.land/bubbletea/v2"
	uv "github.com/charmbracelet/ultraviolet"
)

type uiRegion string

const (
	uiRegionNone      uiRegion = "none"
	uiRegionHeader    uiRegion = "header"
	uiRegionMain      uiRegion = "main"
	uiRegionInput     uiRegion = "input"
	uiRegionSlashMenu uiRegion = "slash_menu"
)

type uiLayout struct {
	header    uv.Rectangle
	main      uv.Rectangle
	input     uv.Rectangle
	slashMenu uv.Rectangle
}

func generateLayout(area uv.Rectangle, headerHeight int, inputHeight int, slashOpen bool) uiLayout {
	if area.Empty() {
		return uiLayout{}
	}

	header := image.Rect(area.Min.X, area.Min.Y, area.Max.X, min(area.Max.Y, area.Min.Y+headerHeight))
	inputBottom := area.Max.Y
	inputTop := max(header.Max.Y, inputBottom-inputHeight)
	input := image.Rect(area.Min.X, inputTop, area.Max.X, inputBottom)

	slashHeight := 0
	if slashOpen {
		slashHeight = min(slashMenuHeight, max(0, inputTop-header.Max.Y))
	}
	slashTop := inputTop - slashHeight
	slash := image.Rect(area.Min.X, slashTop, area.Max.X, inputTop)
	main := image.Rect(area.Min.X, header.Max.Y, area.Max.X, slashTop)
	if main.Max.Y < main.Min.Y {
		main.Max.Y = main.Min.Y
	}
	if !slashOpen {
		slash = image.Rect(area.Min.X, inputTop, area.Max.X, inputTop)
	}

	return uiLayout{
		header:    header,
		main:      main,
		input:     input,
		slashMenu: slash,
	}
}

func localMouse(layout uiLayout, msg tea.MouseMsg) (uiRegion, tea.MouseMsg) {
	mouse := msg.Mouse()
	point := image.Pt(mouse.X, mouse.Y)

	switch {
	case point.In(layout.header):
		return uiRegionHeader, withLocalMouse(msg, layout.header.Min.X, layout.header.Min.Y)
	case point.In(layout.main):
		return uiRegionMain, withLocalMouse(msg, layout.main.Min.X, layout.main.Min.Y)
	case point.In(layout.slashMenu):
		return uiRegionSlashMenu, withLocalMouse(msg, layout.slashMenu.Min.X, layout.slashMenu.Min.Y)
	case point.In(layout.input):
		return uiRegionInput, withLocalMouse(msg, layout.input.Min.X, layout.input.Min.Y)
	default:
		return uiRegionNone, msg
	}
}

func withLocalMouse(msg tea.MouseMsg, offsetX int, offsetY int) tea.MouseMsg {
	mouse := msg.Mouse()
	mouse.X -= offsetX
	mouse.Y -= offsetY

	switch msg.(type) {
	case tea.MouseClickMsg:
		return tea.MouseClickMsg(mouse)
	case tea.MouseReleaseMsg:
		return tea.MouseReleaseMsg(mouse)
	case tea.MouseWheelMsg:
		return tea.MouseWheelMsg(mouse)
	case tea.MouseMotionMsg:
		return tea.MouseMotionMsg(mouse)
	default:
		return tea.MouseClickMsg(mouse)
	}
}
