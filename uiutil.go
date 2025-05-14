package main

import (
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// Utility: Remove any right-side panels (bucket list, codepipeline, lambda, content, etc.)
func removeRightPanels(flex *tview.Flex, mainPanel *tview.TextView) {
	for flex.GetItemCount() > 1 {
		flex.RemoveItem(flex.GetItem(1))
	}
	flex.AddItem(mainPanel, 0, 3, false)
}

// Utility: Set border color based on focus
func setPanelBorderColor(app *tview.Application, panel tview.Primitive, focused bool) {
	if tv, ok := panel.(*tview.TextView); ok {
		if focused {
			tv.SetBorderColor(tcell.ColorGreenYellow)
		} else {
			tv.SetBorderColor(tcell.ColorDefault)
		}
	}
	if l, ok := panel.(*tview.List); ok {
		if focused {
			l.SetBorderColor(tcell.ColorGreenYellow)
		} else {
			l.SetBorderColor(tcell.ColorDefault)
		}
	}
}

// Set transparent background for all tview primitives and remove borders for text panels
func setTransparentBackground(p tview.Primitive) {
	switch v := p.(type) {
	case *tview.TextView:
		v.SetBackgroundColor(tcell.ColorDefault)
		v.SetBorder(false)
	case *tview.List:
		v.SetBackgroundColor(tcell.ColorDefault)
	case *tview.InputField:
		v.SetBackgroundColor(tcell.ColorDefault)
	case *tview.Flex:
		for i := 0; i < v.GetItemCount(); i++ {
			setTransparentBackground(v.GetItem(i))
		}
	}
}
