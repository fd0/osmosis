package tui

import (
	"fmt"
	"runtime"
	"time"

	"github.com/rivo/tview"
)

type statusBar struct {
	root          *tview.Grid
	stats         *tview.TextView
	intercept     *tview.TextView
	interceptMode bool
}

// StatusUpdate is used to describe the new status of the statusBar
type StatusUpdate struct {
	stats     bool
	intercept string
}

func (t *Tui) setupStatusBar() *statusBar {
	bar := tview.NewGrid().SetColumns(-1, -1)
	stats := tview.NewTextView()
	intercept := tview.NewTextView()
	intercept.SetDynamicColors(true).SetTextAlign(tview.AlignRight)
	intercept.SetText(interceptText[false])
	bar.AddItem(stats, 0, 0, 1, 1, 0, 0, false)
	bar.AddItem(intercept, 0, 1, 1, 1, 0, 0, false)

	return &statusBar{
		root:      bar,
		stats:     stats,
		intercept: intercept,
	}
}

var interceptText = map[bool]string{
	true:  "[red]⚫︎Intercept On[-]  ",
	false: "[grey]✘ Intercept Off[-] ",
}

func (t *Tui) updateStatusBar(update *StatusUpdate) {
	t.App.QueueUpdateDraw(func() {
		if update.stats == true {
			t.statusBar.stats.SetText(
				fmt.Sprintf(" %s | %d Requests | %d Goroutines | Press ? for help",
					time.Now().Format("03:04PM"), len(t.Requests), runtime.NumGoroutine()))
		}
		if update.intercept != "" {
			switch update.intercept {
			case "on":
				t.statusBar.interceptMode = true
				t.statusBar.intercept.SetText(interceptText[true])
			case "off":
				t.statusBar.interceptMode = true
				t.statusBar.intercept.SetText(interceptText[false])
			case "toggle":
				t.statusBar.interceptMode = !t.statusBar.interceptMode
				t.statusBar.intercept.SetText(interceptText[t.statusBar.interceptMode])
			default:
				t.log("Debug: Invalid status bar update: intercept=%s", update.intercept)
			}
		}
	})
}
