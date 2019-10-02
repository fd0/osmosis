package main

import (
	"context"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/signal"
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
}

func warn(msg string, args ...interface{}) {
	if !strings.HasSuffix(msg, "\n") {
		msg += "\n"
	}
	fmt.Fprintf(os.Stderr, msg, args...)
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

	if opts.Logdir != "" {
		opts.Logdir = "log-" + time.Now().Format("20060201-150405")
		err = os.MkdirAll(opts.Logdir, 0755)
		if err != nil {
			panic(err)
		}
	} else {
		dirName, err := ioutil.TempDir(".", "log-tmp-")
		if err != nil {
			panic(err)
		}
		opts.Logdir = dirName
		defer func() {
			err := os.RemoveAll(dirName)
			if err != nil {
				log.Println(err)
			}
		}()
	}

	p := proxy.New(opts.Listen, ca, nil, nil)
	// store, err := store.New(opts.Logdir)
	// if err != nil {
	// 	log.Println(err)
	// 	return
	// }

	p.Register(func(req *proxy.Request, forward proxy.PipelineFunc) (*http.Response, error) {
		res, err := forward(req)
		if err != nil {
			return nil, err
		}
		req.Log("%v %v %v %v\n", res.StatusCode, req.Method, req.URL, req.Proto)
		return res, err
	})

	p.Register(func(req *proxy.Request, forward proxy.PipelineFunc) (*http.Response, error) {
		req.Request.Header["User-Agent"] = []string{"Osmosis Proxy"}
		return forward(req)
	})

	log.SetFlags(log.Ldate | log.Ltime | log.Lmicroseconds)
	log.Printf("CA loaded: %v\n", ca.Certificate.Subject)

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

	go func() {
		sigchan := make(chan os.Signal, 10)
		signal.Notify(sigchan, os.Interrupt)
		<-sigchan
		p.Shutdown(context.Background())
	}()

	log.Println(p.ListenAndServe())
}
