package main

import (
	"bufio"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/fd0/osmosis/certauth"
	"github.com/fd0/osmosis/proxy"
	"github.com/gdamore/tcell"
	"github.com/rivo/tview"
	"github.com/spf13/pflag"
)

// Request bundles an HTTP request and the corresponding response
type Request struct {
	ID int
	*http.Request
	*http.Response
}

// OsmosisGui is a container through which the elements to "see" each other
type OsmosisGui struct {
	app   *tview.Application
	pages *tview.Pages

	requests []Request

	newRequestEvent   chan *Request
	requestViewEvents chan *Request
	statusBarEvents   chan string
}

// NewOsmosisGui returns a new GUI container
func NewOsmosisGui() *OsmosisGui {
	g := &OsmosisGui{
		app:               tview.NewApplication(),
		pages:             tview.NewPages(),
		newRequestEvent:   make(chan *Request),
		requestViewEvents: make(chan *Request),
		statusBarEvents:   make(chan string),
	}
	g.pages.AddPage("overview", g.overviewPage(), true, true)
	g.pages.AddPage("viewer", g.viewerPage(), true, false)
	g.app.SetRoot(g.pages, true).SetFocus(g.pages)

	// periodically update status bar
	go func() {
		for {
			g.statusBarEvents <- fmt.Sprintf("%s | %d Requests | %d Goroutines",
				time.Now().Format("15:04PM"), len(g.requests), runtime.NumGoroutine())
			g.app.Draw()
			time.Sleep(time.Second)
		}
	}()

	// read initial requests from log dir
	go func() {
		reqs, err := loadRequests(opts.Logdir)
		if err != nil {
			panic(err)
		}
		for _, req := range reqs {
			g.requests = append(g.requests, req)
			g.newRequestEvent <- &g.requests[len(g.requests)-1]
		}
	}()
	return g
}

// viewerPage create a page visualizing a single request
func (g *OsmosisGui) viewerPage() *tview.Grid {
	grid := tview.NewGrid().SetColumns(-1, -1).SetRows(-1, 3)
	requestField := tview.NewTextView()
	requestField.SetBorder(true).SetTitle("Request")
	responseField := tview.NewTextView()
	responseField.SetBorder(true).SetTitle("Response")
	grid.AddItem(requestField, 0, 0, 1, 1, 0, 0, false)
	grid.AddItem(responseField, 0, 1, 1, 1, 0, 0, false)
	grid.AddItem(
		tview.NewButton("back").SetSelectedFunc(func() {
			g.pages.SwitchToPage("overview")
		}), 1, 0, 1, 2, 0, 0, true)

	// set the view content if a request ist selected
	go func() {
		for {
			event := <-g.requestViewEvents
			req, err := httputil.DumpRequest(event.Request, true)
			if err != nil {
				req = []byte(err.Error())
			}
			res, err := httputil.DumpResponse(event.Response, false)
			if err != nil {
				res = []byte(err.Error())
			}
			requestField.SetText(string(req))
			responseField.SetText(string(res) + "No idea how to get the body")
		}
	}()
	return grid
}

// overviewPage creates a page containing the requests list
func (g *OsmosisGui) overviewPage() *tview.Grid {
	grid := tview.NewGrid().SetRows(-6, -1, 1)

	// create logview first in case logging is done during setup
	lv := g.logView()
	grid.AddItem(lv, 1, 0, 1, 1, 0, 0, false)

	table := g.requestsTable()
	grid.AddItem(table, 0, 0, 1, 1, 0, 0, true)

	statusBar := tview.NewTextView()
	go func() {
		for {
			statusBar.SetText(<-g.statusBarEvents)
		}
	}()
	grid.AddItem(statusBar, 2, 0, 1, 1, 0, 0, false)
	return grid
}

func addRequestRow(table *tview.Table, req *Request) {
	row := table.GetRowCount()
	table.SetCell(row, 0, &tview.TableCell{
		NotSelectable: true,
		Color:         tcell.ColorGreen,
		Text:          fmt.Sprintf("%d", req.ID),
	})
	table.SetCell(row, 1, &tview.TableCell{
		Color: tcell.ColorWhite,
		Text:  req.URL.Scheme + "://" + req.Host,
	})
	table.SetCell(row, 2, &tview.TableCell{
		Color: tcell.ColorWhite,
		Text:  req.Method,
	})
	table.SetCell(row, 3, &tview.TableCell{
		Color: tcell.ColorWhite,
		Text:  printStatus(req.Response.StatusCode),
	})
	table.SetCell(row, 4, &tview.TableCell{
		Color: tcell.ColorWhite,
		Text:  printPathQuery(req.URL),
	})

}

// requestsTable creates the list of requests within the overview page
func (g *OsmosisGui) requestsTable() *tview.Table {
	table := tview.NewTable()
	table.SetSelectedFunc(func(row int, column int) {
		if g.requests == nil {
			return
		}
		g.requestViewEvents <- &g.requests[row-1]
		g.pages.SwitchToPage("viewer")
	})
	// make first column and row a header which is always shown
	table.SetFixed(1, 1)
	// make rows selectable
	table.SetSelectable(true, false)

	table.SetBorder(true).SetTitle("Requests")

	// header
	table.SetCell(0, 0, &tview.TableCell{NotSelectable: true, Text: "[white::b]ID"})
	table.SetCell(0, 1, &tview.TableCell{NotSelectable: true, Text: "[white::b]Host"})
	table.SetCell(0, 2, &tview.TableCell{NotSelectable: true, Text: "[white::b]Method"})
	table.SetCell(0, 3, &tview.TableCell{NotSelectable: true, Text: "[white::b]Status"})
	table.SetCell(0, 4, &tview.TableCell{NotSelectable: true, Text: "[white::b]Params",
		Expansion: 1})

	go func() {
		for {
			req := <-g.newRequestEvent
			addRequestRow(table, req)
		}
	}()

	return table
}

func readRequest(dir string, id int) (*http.Request, error) {
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

func readResponse(dir string, id int) (*http.Response, error) {
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
	id := 0
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

var logger io.Writer

func log(msg string, args ...interface{}) {
	if !strings.HasSuffix(msg, "\n") {
		msg += "\n"
	}
	fmt.Fprintf(logger, msg, args...)
}

func (g *OsmosisGui) logView() tview.Primitive {
	tv := tview.NewTextView()
	tv.SetBorder(true).SetTitle("Log")
	tv.SetChangedFunc(func() {
		g.app.Draw()
	})
	logger = tv
	return tv
}

func warn(msg string, args ...interface{}) {
	if !strings.HasSuffix(msg, "\n") {
		msg += "\n"
	}
	fmt.Fprintf(os.Stderr, msg, args...)
}

func saveRequest(id uint64, req *http.Request) {
	req.RequestURI = req.URL.String()

	filename := filepath.Join(opts.Logdir, fmt.Sprintf("%v.request", id))
	f, err := os.Create(filename)
	if err != nil {
		log("unable to create file %v: %v\n", filename, err)
		return
	}

	err = req.WriteProxy(f)
	if err != nil {
		log("unable to dump request %v: %v\n", id, err)
		_ = f.Close()
		return
	}

	err = f.Close()
	if err != nil {
		log("unable to save to file %v: %v\n", filename, err)
		return
	}
}

func saveResponse(id uint64, res *http.Response) {
	buf, err := httputil.DumpResponse(res, true)
	if err != nil {
		warn("unable to dump response %v: %v\n", id, err)
		return
	}

	filename := filepath.Join(opts.Logdir, fmt.Sprintf("%v.response", id))
	err = ioutil.WriteFile(filename, buf, 0644)
	if err != nil {
		warn("unable to save response %v: %v\n", id, err)
		return
	}
}

// Options collects global settings.
type Options struct {
	CertificateFilename, KeyFilename string
	Listen                           string
	Logdir                           string
}

var opts Options

func init() {
	fs := pflag.NewFlagSet("osmosis", pflag.ExitOnError)
	fs.StringVar(&opts.CertificateFilename, "cert", "ca.crt", "read certificate from `file`")
	fs.StringVar(&opts.KeyFilename, "key", "ca.key", "read private key from `file`")
	fs.StringVar(&opts.Listen, "listen", "[::1]:8080", "listen at `addr`")
	fs.StringVar(&opts.Logdir, "log-dir", "", "set log `directory` (default: log-YYYMMMDDD-HHMMSS)")

	err := fs.Parse(os.Args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error parsing flags: %v\n", err)
		os.Exit(1)
	}

	if opts.Logdir == "" {
		opts.Logdir = "log-" + time.Now().Format("20060201-150405")
	}
}

func main() {
	ca, err := certauth.Load(opts.CertificateFilename, opts.KeyFilename)
	if os.IsNotExist(err) {
		fmt.Printf("generate new CA certificate\n")
		ca, err = certauth.NewCA()
		if err != nil {
			panic(err)
		}

		err = ca.Save(opts.CertificateFilename, opts.KeyFilename)
		if err != nil {
			panic(err)
		}
	}

	gui := NewOsmosisGui()
	p := proxy.New(opts.Listen, ca, nil, logger)

	err = os.MkdirAll(opts.Logdir, 0755)
	if err != nil {
		panic(err)
	}

	p.OnResponse = func(req *proxy.Request, res *http.Response) {
		var id int
		if gui.requests != nil {
			id = gui.requests[len(gui.requests)-1].ID + 1
		}
		gui.requests = append(gui.requests, Request{
			ID:       id,
			Request:  req.Request,
			Response: res,
		})
		gui.newRequestEvent <- &gui.requests[len(gui.requests)-1]
		saveRequest(req.ID, req.Request)
		saveResponse(req.ID, res)
	}

	go p.ListenAndServe()

	err = gui.app.Run()
	if err != nil {
		panic(err)
	}
}
