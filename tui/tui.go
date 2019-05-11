package tui

import (
	"bufio"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/gdamore/tcell"
	"github.com/rivo/tview"
)

// Request bundles an HTTP request and the corresponding response
type Request struct {
	ID uint64
	*http.Request
	*http.Response
}

// Tui is a container through which the GUI elements communicate
type Tui struct {
	App      *tview.Application
	Requests []Request

	root        *tview.Grid
	statusBar   *tview.TextView
	MainView    *tview.Pages
	history     *tview.Table
	LogView     *tview.TextView
	help        *tview.TextView
	requestView *requestView
}

// New returns a new tui
// TODO: remove logDir and move request logging out of the user interface code
func New(logDir string) *Tui {
	t := &Tui{}
	t.App = tview.NewApplication()

	t.root = tview.NewGrid().SetRows(-1, 1)
	t.LogView = t.setupLog()
	t.MainView = tview.NewPages()
	t.history = t.setupHistory()
	t.requestView = t.setupRequestView()
	t.statusBar = tview.NewTextView()
	t.help = t.setupHelp()

	t.MainView.AddPage("history", t.history, true, true)
	t.MainView.AddPage("viewer", t.requestView.root, true, false)
	t.MainView.AddPage("log", t.LogView, true, false)
	t.MainView.AddPage("help", t.help, true, false)
	t.root.AddItem(t.MainView, 0, 0, 1, 1, 0, 0, false)
	t.root.AddItem(t.statusBar, 1, 0, 1, 1, 0, 0, false)

	t.App.SetRoot(t.root, true).SetFocus(t.history)
	t.App.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Rune() {
		case 'l', 'L':
			t.MainView.SwitchToPage("log")
			t.App.SetFocus(t.LogView)
		case 'q', 'Q':
			t.App.Stop()
		case 'h', 'H', rune(0), rune(127):
			t.MainView.SwitchToPage("history")
			t.App.SetFocus(t.history)
		case '?':
			t.MainView.SwitchToPage("help")
			t.App.SetFocus(t.help)
		}
		t.App.Draw()
		return event
	})

	// periodically update status bar
	go func() {
		for {
			t.statusBar.SetText(fmt.Sprintf("%s | %d Requests | %d Goroutines | Press ? for help",
				time.Now().Format("03:04PM"), len(t.Requests), runtime.NumGoroutine()))
			t.App.Draw()
			time.Sleep(time.Second)
		}
	}()

	// read initial requests from log dir
	reqs, err := loadRequests(logDir)
	if err != nil {
		panic(err)
	}
	t.AppendToHistory(reqs...)
	return t
}

func readRequest(dir string, id uint64) (*http.Request, error) {
	filename := filepath.Join(dir, fmt.Sprintf("%d.request", id))
	f, err := os.Open(filename)
	if err != nil {
		return nil, err
	}

	rd := bufio.NewReader(f)

	req, err := http.ReadRequest(rd)
	if err != nil {
		return nil, err
	}

	err = f.Close()
	if err != nil {
		return nil, err
	}

	return req, nil
}

func readResponse(dir string, id uint64) (*http.Response, error) {
	filename := filepath.Join(dir, fmt.Sprintf("%d.response", id))
	f, err := os.Open(filename)
	if err != nil {
		return nil, err
	}

	rd := bufio.NewReader(f)

	res, err := http.ReadResponse(rd, nil)
	if err != nil {
		return nil, err
	}

	res.Body.Close()

	err = f.Close()
	if err != nil {
		return nil, err
	}

	return res, nil
}

func loadRequests(dir string) (reqs []Request, err error) {
	var id uint64
	notfound := 0
	for {
		id++

		req, err := readRequest(dir, id)
		if os.IsNotExist(err) {
			notfound++
		}

		if notfound > 50 {
			break
		}

		if err != nil {
			continue
		}

		notfound = 0

		res, err := readResponse(dir, id)
		if err != nil {
			continue
		}

		reqs = append(reqs, Request{
			ID:       id,
			Request:  req,
			Response: res,
		})
	}

	return reqs, nil
}

func (t *Tui) setupHelp() *tview.TextView {
	help := tview.NewTextView()
	help.SetBorder(true).SetTitle("Help")
	help.SetDynamicColors(true)
	help.SetText(`
	[orange::bu]Request History[-::-]

	By default Osmosis shows the request history. You can always return back
	to the request history by pressing [yellow::b]h[-::-], [yellow::b]Backspace[-::-]
	or [yellow::b]Escape[-::-].

	[orange::bu]Request View[-::-]

	In the history view, the request in focus can be viewed by pressing [yellow::b]Enter[-::-].
	This will open the request view showing the request and the corresponding response.

	[orange::bu]Interception[-::-]

	Not implemented.

	[orange::bu]Log[-::-]

	Pressing [yellow::b]l[-::-] brings up the log.

	[orange::bu]Help[-::-]

	This help page can be opened by pressing [yellow::b]h[-::-] or [yellow::b]i[-::-].`)
	return help
}
