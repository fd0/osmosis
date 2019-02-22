package main

import (
	"bufio"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gdamore/tcell"
	"github.com/rivo/tview"
)

// Request bundles an HTTP request and the corresponding response
type Request struct {
	ID int
	*http.Request
	*http.Response
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

	buf := &strings.Builder{}
	buf.WriteString(u.EscapedPath())
	if u.ForceQuery || u.RawQuery != "" {
		buf.WriteByte('?')
		buf.WriteString(u.RawQuery)
	}
	if u.Fragment != "" {
		buf.WriteByte('#')
		buf.WriteString(u.Fragment)
	}
	return buf.String()
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

func requestsTable() tview.Primitive {
	requests, err := loadRequests(os.Args[1])
	if err != nil {
		panic(err)
	}

	table := tview.NewTable()
	// make first column and row a header which is always shown
	table.SetFixed(1, 1)
	// make rows selectable
	table.SetSelectable(true, false)

	table.SetBorder(true)

	// header
	table.SetCell(0, 0, &tview.TableCell{
		Color:         tcell.ColorWhite,
		Attributes:    tcell.AttrBold,
		NotSelectable: true,
		Text:          "ID",
	})
	table.SetCell(0, 2, &tview.TableCell{
		Color:         tcell.ColorWhite,
		Attributes:    tcell.AttrBold,
		NotSelectable: true,
		Text:          "Host",
	})
	table.SetCell(0, 3, &tview.TableCell{
		Color:         tcell.ColorWhite,
		Attributes:    tcell.AttrBold,
		NotSelectable: true,
		Text:          "Method",
	})
	table.SetCell(0, 4, &tview.TableCell{
		Color:         tcell.ColorWhite,
		Attributes:    tcell.AttrBold,
		NotSelectable: true,
		Text:          "Status",
	})
	table.SetCell(0, 5, &tview.TableCell{
		Color:         tcell.ColorWhite,
		Attributes:    tcell.AttrBold,
		NotSelectable: true,
		Text:          "Params",
		Expansion:     1,
	})

	for i, req := range requests {
		table.SetCell(i+1, 0, &tview.TableCell{
			NotSelectable: true,
			Color:         tcell.ColorGreen,
			Text:          fmt.Sprintf("%d", req.ID),
		})
		table.SetCell(i+1, 2, &tview.TableCell{
			Color: tcell.ColorWhite,
			Text:  req.URL.Scheme + "://" + req.Host,
		})
		table.SetCell(i+1, 3, &tview.TableCell{
			Color: tcell.ColorWhite,
			Text:  req.Method,
		})
		table.SetCell(i+1, 4, &tview.TableCell{
			Color: tcell.ColorWhite,
			Text:  printStatus(req.Response.StatusCode),
		})
		table.SetCell(i+1, 5, &tview.TableCell{
			Color: tcell.ColorWhite,
			Text:  printPathQuery(req.URL),
		})
	}

	return table
}

var logger io.Writer

func log(msg string, args ...interface{}) {
	if !strings.HasSuffix(msg, "\n") {
		msg += "\n"
	}
	fmt.Fprintf(logger, msg, args...)
}

func logView(app *tview.Application) tview.Primitive {
	tv := tview.NewTextView()
	tv.SetBorder(true)
	tv.SetChangedFunc(func() {
		app.Draw()
	})
	logger = tv
	return tv
}

func main() {
	app := tview.NewApplication()

	grid := tview.NewGrid().SetRows(-6, -1, 1)

	table := requestsTable()
	grid.AddItem(table, 0, 0, 1, 1, 0, 0, true)

	lv := logView(app)
	grid.AddItem(lv, 1, 0, 1, 1, 0, 0, false)

	status := tview.NewTextView()
	status.SetText("status text")
	grid.AddItem(status, 2, 0, 1, 1, 0, 0, false)

	go func() {
		for {
			time.Sleep(time.Second)
			log("foobar %v", time.Now())
		}
	}()

	app.SetRoot(grid, true)
	err := app.Run()
	if err != nil {
		panic(err)
	}
}
