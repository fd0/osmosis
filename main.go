package main

import (
	"fmt"
	"log"
	"os"

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

	log.Printf("CA loaded: %v\n", ca.Certificate.Subject)

	proxy := proxy.New(opts.Listen, ca)
	log.Fatal(proxy.Serve())
}
