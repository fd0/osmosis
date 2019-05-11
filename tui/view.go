package tui

import (
	"net/http/httputil"

	"github.com/rivo/tview"
)

type requestView struct {
	root          *tview.Grid
	requestField  *tview.TextView
	responseField *tview.TextView
}

func (t *Tui) selectViewedRequest(selected *Request) {
	req, err := httputil.DumpRequest(selected.Request, true)
	if err != nil {
		req = []byte(err.Error())
	}
	res, err := httputil.DumpResponse(selected.Response, false)
	if err != nil {
		res = []byte(err.Error())
	}
	t.requestView.requestField.SetText(string(req))
	t.requestView.responseField.SetText(string(res) + "No idea how to get the body")

}

// setupRequestView create a page visualizing a single request
func (t *Tui) setupRequestView() *requestView {
	grid := tview.NewGrid().SetColumns(-1, -1)
	requestField := tview.NewTextView()
	requestField.SetBorder(true).SetTitle("Request")
	responseField := tview.NewTextView()
	responseField.SetBorder(true).SetTitle("Response")
	grid.AddItem(requestField, 0, 0, 1, 1, 0, 0, false)
	grid.AddItem(responseField, 0, 1, 1, 1, 0, 0, false)
	return &requestView{
		root:          grid,
		requestField:  requestField,
		responseField: responseField,
	}
}
