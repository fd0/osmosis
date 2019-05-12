package tui

import (
	"net/http/httputil"

	"github.com/rivo/tview"
)

type editor struct {
	root          *tview.Grid
	editorField   *tview.InputField
	responseField *tview.TextView
}

func (t *Tui) setupEditor() *editor {
	grid := tview.NewGrid().SetColumns(-1, -1)
	editorField := tview.NewInputField()
	editorField.SetBorder(true).SetTitle("[::b] Request [::-]")
	responseField := tview.NewTextView()
	responseField.SetBorder(true).SetTitle("[::b] Response [::-]")
	responseField.SetText(`The editor is currently broken as the InputField only
	supports a single line of input.`)
	grid.AddItem(editorField, 0, 0, 1, 1, 0, 0, false)
	grid.AddItem(responseField, 0, 1, 1, 1, 0, 0, false)
	return &editor{
		root:          grid,
		editorField:   editorField,
		responseField: responseField,
	}
}

func (t *Tui) selectEditedRequest(selected *Request) {
	req, err := httputil.DumpRequest(selected.Request, true)
	if err != nil {
		req = []byte(err.Error())
	}
	t.editor.editorField.SetText(string(req))
}
