package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/http/httputil"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/fd0/osmosis/certauth"
	"github.com/fd0/osmosis/proxy"
	"github.com/fd0/osmosis/tui"
	"github.com/spf13/pflag"
)

// Options collects global settings.
type Options struct {
	CertificateFilename, KeyFilename string
	Listen                           string
	Logdir                           string
	NoGui                            bool
}

var opts Options

func init() {
	fs := pflag.NewFlagSet("osmosis", pflag.ExitOnError)
	fs.StringVar(&opts.CertificateFilename, "cert", "ca.crt", "read certificate from `file`")
	fs.StringVar(&opts.KeyFilename, "key", "ca.key", "read private key from `file`")
	fs.StringVar(&opts.Listen, "listen", "[::1]:8080", "listen at `addr`")
	fs.StringVar(&opts.Logdir, "log-dir", "", "set log `directory` (default: log-YYYMMMDDD-HHMMSS)")
	fs.BoolVar(&opts.NoGui, "no-gui", false, "Disable graphical user interface")

	err := fs.Parse(os.Args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error parsing flags: %v\n", err)
		os.Exit(1)
	}

	if opts.Logdir == "" {
		opts.Logdir = "log-" + time.Now().Format("20060201-150405")
	}
}

func warn(msg string, args ...interface{}) {
	if !strings.HasSuffix(msg, "\n") {
		msg += "\n"
	}
	fmt.Fprintf(os.Stderr, msg, args...)
}

func saveRequest(id uint64, req *http.Request) {
	// fmt.Printf("dump request for %v %v\n", req.URL, req.RequestURI)
	req.RequestURI = req.URL.String()

	// buf, err := httputil.DumpRequestOut(req, true)

	filename := filepath.Join(opts.Logdir, fmt.Sprintf("%v.request", id))
	f, err := os.Create(filename)
	if err != nil {
		warn("unable to create file %v: %v\n", filename, err)
		return
	}

	err = req.WriteProxy(f)
	if err != nil {
		warn("unable to dump request %v: %v\n", id, err)
		_ = f.Close()
		return
	}

	err = f.Close()
	if err != nil {
		warn("unable to save to file %v: %v\n", filename, err)
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

	// cfg := &tls.Config{
	// 	InsecureSkipVerify: true,
	// }

	err = os.MkdirAll(opts.Logdir, 0755)
	if err != nil {
		panic(err)
	}

	if opts.NoGui {
		p := proxy.New(opts.Listen, ca, nil, nil)
		log.SetFlags(log.Ldate | log.Ltime | log.Lmicroseconds)
		log.Printf("CA loaded: %v\n", ca.Certificate.Subject)
		log.Printf("logging requests to directory %q", opts.Logdir)

		p.OnResponse = func(req *proxy.Request, res *http.Response) {
			log.Printf("dump request for %v %v\n", req.Request.URL, req.Request.RequestURI)
			saveRequest(req.ID, req.Request)
			saveResponse(req.ID, res)
		}
		go func() {
			ticker := time.NewTicker(2 * time.Second)
			defer ticker.Stop()

			var last int
			for range ticker.C {
				var cur = runtime.NumGoroutine()
				if cur != last {
					log.Printf("%d active goroutines", cur)
					last = cur
				}
			}
		}()

		log.Fatal(p.ListenAndServe())
	} else {
		ui := tui.New(opts.Logdir)
		p := proxy.New(opts.Listen, ca, nil, ui.LogView)

		p.OnResponse = func(req *proxy.Request, res *http.Response) {
			var id uint64
			if ui.Requests != nil {
				id = ui.Requests[len(ui.Requests)-1].ID + 1
			}
			ui.AppendToHistory(tui.Request{
				ID:       id,
				Request:  req.Request,
				Response: res,
			})
			fmt.Fprintf(ui.LogView, "dump request for %v %v\n", req.Request.URL,
				req.Request.RequestURI)
			saveRequest(id, req.Request)
			saveResponse(id, res)

		}
		go func() {
			err := p.ListenAndServe()
			fmt.Fprintf(ui.LogView, "[red::bu]Proxy Stopped:[::-] %s[-::]\n", err.Error())
			fmt.Fprintf(ui.LogView, "Press [yellow::b]q[-::-] to exit...")
			ui.MainView.SwitchToPage("log")
			ui.App.SetFocus(ui.LogView)
		}()

		err = ui.App.Run()
		if err != nil {
			panic(err)
		}

	}

}
