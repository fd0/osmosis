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
	"github.com/spf13/pflag"
)

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

func warn(msg string, args ...interface{}) {
	if !strings.HasSuffix(msg, "\n") {
		msg += "\n"
	}
	fmt.Fprintf(os.Stderr, msg, args...)
}

func saveRequest(id uint64, req *http.Request) {
	fmt.Printf("dump request for %v %v\n", req.URL, req.RequestURI)
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

	log.SetFlags(log.Ldate | log.Ltime | log.Lmicroseconds)

	log.Printf("CA loaded: %v\n", ca.Certificate.Subject)

	log.Printf("logging requests to directory %q", opts.Logdir)

	// cfg := &tls.Config{
	// 	InsecureSkipVerify: true,
	// }

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

	p := proxy.New(opts.Listen, ca, nil, nil)

	err = os.MkdirAll(opts.Logdir, 0755)
	if err != nil {
		panic(err)
	}

	p.OnResponse = func(req *proxy.Request, res *http.Response) {
		saveRequest(req.ID, req.Request)
		saveResponse(req.ID, res)
	}

	log.Fatal(p.ListenAndServe())
}
