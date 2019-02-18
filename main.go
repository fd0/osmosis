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

	"github.com/happal/osmosis/certauth"
	"github.com/happal/osmosis/proxy"
	"github.com/spf13/pflag"
)

// Options collects global settings.
type Options struct {
	CertificateFilename, KeyFilename string
	Listen                           string
}

var opts Options

func init() {
	fs := pflag.NewFlagSet("osmosis", pflag.ExitOnError)
	fs.StringVar(&opts.CertificateFilename, "cert", "ca.crt", "read certificate from `file`")
	fs.StringVar(&opts.KeyFilename, "key", "ca.key", "read private key from `file`")
	fs.StringVar(&opts.Listen, "listen", "[::1]:8080", "listen at `addr`")

	err := fs.Parse(os.Args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error parsing flags: %v\n", err)
		os.Exit(1)
	}
}

const logdir = "log"

func warn(msg string, args ...interface{}) {
	if !strings.HasSuffix(msg, "\n") {
		msg += "\n"
	}
	fmt.Fprintf(os.Stderr, msg, args...)
}

func saveRequest(id uint64, req *http.Request) {
	buf, err := httputil.DumpRequest(req, false)
	if err != nil {
		warn("unable to dump request %v: %v\n", id, err)
		return
	}

	filename := filepath.Join(logdir, fmt.Sprintf("%v.request", id))
	err = ioutil.WriteFile(filename, buf, 0644)
	if err != nil {
		warn("unable to save request %v: %v\n", id, err)
		return
	}
}

func saveResponse(id uint64, res *http.Response) {
	buf, err := httputil.DumpResponse(res, false)
	if err != nil {
		warn("unable to dump request %v: %v\n", id, err)
		return
	}

	filename := filepath.Join(logdir, fmt.Sprintf("%v.response", id))
	err = ioutil.WriteFile(filename, buf, 0644)
	if err != nil {
		warn("unable to save request %v: %v\n", id, err)
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

	p := proxy.New(opts.Listen, ca, nil)

	err = os.MkdirAll(logdir, 0755)
	if err != nil {
		panic(err)
	}

	p.OnResponse = func(req *proxy.Request, res *http.Response) {
		saveRequest(req.ID, req.Request)
		saveResponse(req.ID, res)
	}

	log.Fatal(p.ListenAndServe())
}
