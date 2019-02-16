package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"net"
	"os"
	"strings"

	"github.com/pkg/errors"
)

// getCertificate connects to the host, attempts a TLS handshake, and then
// disconnects. It returns the first leaf (=non-CA) certificate.
func getCertificate(ctx context.Context, host string, clientConfig *tls.Config) (*x509.Certificate, error) {
	// create new dialer so that we can use DialContext
	dialer := &net.Dialer{}

	// connect with timeout context
	conn, err := dialer.DialContext(ctx, "tcp", host)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	var cfg = &tls.Config{}
	if clientConfig != nil {
		// use settings from the tlsClient
		cfg = clientConfig.Clone()
	}

	// set server name to host name without port
	cfg.ServerName = strings.Split(host, ":")[0]

	// try a TLS client handshake
	client := tls.Client(conn, cfg)
	err = client.Handshake()
	if err != nil {
		_ = conn.Close()
		return nil, errors.WithStack(err)
	}

	// close the TLS client (which also closes the underlying connection)
	err = client.Close()
	if err != nil {
		_ = conn.Close()
		return nil, errors.WithStack(err)
	}

	for _, cert := range client.ConnectionState().PeerCertificates {
		if !cert.IsCA {
			return cert, nil
		}
	}

	return nil, errors.New("no certificate could be found")
}

func main() {
	target := "fd0.me:443"
	if len(os.Args) > 1 {
		target = os.Args[1]
	}

	cfg := &tls.Config{InsecureSkipVerify: true}

	cert, err := getCertificate(context.Background(), target, cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %+v\n", err)
		return
	}

	fmt.Fprintf(os.Stderr, "cert %v\n  names %v\n  ips %v\n", cert.Subject.CommonName, cert.DNSNames, cert.IPAddresses)
	fmt.Fprintf(os.Stderr, "  valid %v -> %v\n", cert.NotBefore.Local(), cert.NotAfter.Local())

	fmt.Printf("%s\n", pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert.Raw}))
}
