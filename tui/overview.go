package tui

import (
	"fmt"
	"net/url"

	"github.com/gdamore/tcell"
	"github.com/rivo/tview"
)

// AppendToHistory appends multiple requests to the history
func (t *Tui) AppendToHistory(requests ...Request) {
	t.App.QueueUpdateDraw(func() {
		t.Requests = append(t.Requests, requests...)
		for _, req := range requests {
			row := t.history.GetRowCount()
			t.history.SetCell(row, 0, &tview.TableCell{
				NotSelectable: true,
				Color:         tcell.ColorGreen,
				Text:          fmt.Sprintf("%d", req.ID),
			})
			t.history.SetCell(row, 1, &tview.TableCell{
				Color: tcell.ColorWhite,
				Text:  req.URL.Scheme + "://" + req.Host,
			})
			t.history.SetCell(row, 2, &tview.TableCell{
				Color: tcell.ColorWhite,
				Text:  req.Method,
			})
			t.history.SetCell(row, 3, &tview.TableCell{
				Color: tcell.ColorWhite,
				Text:  printStatus(req.Response.StatusCode),
			})
			t.history.SetCell(row, 4, &tview.TableCell{
				Color: tcell.ColorWhite,
				Text:  printPathQuery(req.URL),
			})
		}
	})
}

// setupHistory creates the list of requests
func (t *Tui) setupHistory() *tview.Table {
	table := tview.NewTable()
	table.SetSelectedFunc(func(row int, column int) {
		if t.Requests != nil {
			t.selectViewedRequest(&t.Requests[row-1])
			t.MainView.SwitchToPage("viewer")
			t.App.SetFocus(t.requestView.root)
		}
	})
	// make first column and row a header which is always shown
	table.SetFixed(1, 1)
	// make rows selectable
	table.SetSelectable(true, false)

	table.SetBorder(true).SetTitle("Request History")

	// header
	table.SetCell(0, 0, &tview.TableCell{NotSelectable: true, Text: "[white::b]ID"})
	table.SetCell(0, 1, &tview.TableCell{NotSelectable: true, Text: "[white::b]Host"})
	table.SetCell(0, 2, &tview.TableCell{NotSelectable: true, Text: "[white::b]Method"})
	table.SetCell(0, 3, &tview.TableCell{NotSelectable: true, Text: "[white::b]Status"})
	table.SetCell(0, 4, &tview.TableCell{NotSelectable: true, Text: "[white::b]Params",
		Expansion: 1})
	return table
}

func printPathQuery(u *url.URL) string {
	var url = *u
	url.Scheme = ""
	url.Host = ""
	return url.String()

	// buf := &strings.Builder{}
	// buf.WriteString(u.EscapedPath())
	// if u.ForceQuery || u.RawQuery != "" {
	// 	buf.WriteByte('?')
	// 	buf.WriteString(u.RawQuery)
	// }
	// if u.Fragment != "" {
	// 	buf.WriteByte('#')
	// 	buf.WriteString(u.Fragment)
	// }
	// return buf.String()
}

func printStatus(code int) string {
	var color = "default"
	switch {
	case code >= 200 && code < 300:
		color = "green"
	case code >= 300 && code < 400:
		color = "orange"
	case code >= 400 && code < 500:
		color = "yellow"
	case code >= 500 && code < 600:
		color = "red"
	}
	return fmt.Sprintf("[%s]%d[default]", color, code)
}
