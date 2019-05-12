package tui

import (
	"fmt"
	"strings"

	"github.com/rivo/tview"
)

func (t *Tui) setupLog() *tview.TextView {
	logView := tview.NewTextView()
	logView.SetBorder(true).SetTitle("[::b] Log [::-]")
	logView.SetDynamicColors(true)
	logView.SetChangedFunc(func() {
		t.App.Draw()
	})
	return logView
}

func (t *Tui) log(msg string, args ...interface{}) {
	if !strings.HasSuffix(msg, "\n") {
		msg += "\n"
	}
	fmt.Fprintf(t.LogView, msg, args...)
}
