package main

import (
	"crypto/tls"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/happal/osmosis/certauth"
)

func exists(filename string) bool {
	_, err := os.Stat(filename)
	if os.IsNotExist(err) {
		return false
	}

	if err != nil {
		panic(err)
	}

	return true
}

const (
	cacert = "ca.crt"
	cakey  = "ca.key"
)

func main() {
	ca, err := certauth.Load(cacert, cakey)
	if os.IsNotExist(err) {
		ca, err = certauth.NewCA()
		if err != nil {
			panic(err)
		}

		err = ca.Save(cacert, cakey)
		if err != nil {
			panic(err)
		}
	}

	fmt.Printf("CA loaded: %v\n", ca.Certificate.Subject)

	fmt.Printf("generate host certificate\n")
	crt, err := ca.NewCertificate("foo.example.com", []string{"foo.example.com", "bar.example.com", "localhost"})
	if err != nil {
		panic(err)
	}

	fmt.Printf("start listener\n")
	cfg := &tls.Config{
		Certificates: []tls.Certificate{
			tls.Certificate{
				Certificate: [][]byte{
					crt.Raw,
					ca.Certificate.Raw,
				},
				PrivateKey: ca.Key,
			},
		},
	}

	srv := &http.Server{
		TLSConfig: cfg,
		Handler: http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
			res.WriteHeader(http.StatusOK)
			fmt.Fprintf(res, "foobar\n")
		}),
		Addr: "localhost:8443",
	}

	log.Fatal(srv.ListenAndServeTLS("", ""))
}
