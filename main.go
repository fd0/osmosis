package main

import (
	"log"
	"os"

	"github.com/happal/osmosis/certauth"
	"github.com/happal/osmosis/proxy"
)

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

	log.Printf("CA loaded: %v\n", ca.Certificate.Subject)

	proxy := proxy.New("[::1]:9090", ca)
	log.Fatal(proxy.Serve())
}
