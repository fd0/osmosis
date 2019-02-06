package proxy

import (
	"crypto/tls"
	"crypto/x509"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/happal/osmosis/certauth"
)

func testClient(t testing.TB, proxyAddress string, ca *certauth.CertificateAuthority) *http.Client {
	proxyURL, err := url.Parse("http://" + proxyAddress)
	if err != nil {
		t.Fatal(err)
	}

	// build a cert pool to use for the HTTP client
	certPool := x509.NewCertPool()
	certPool.AddCert(ca.Certificate)

	return &http.Client{
		Transport: &http.Transport{
			Proxy: func(*http.Request) (*url.URL, error) {
				return proxyURL, nil
			},
			TLSClientConfig: &tls.Config{
				// make sure TLS interception works by only allowing the given
				// CA certificate as root
				RootCAs: certPool,
			},
		},
	}
}

func TestProxySimple(t *testing.T) {
	proxy, serve, shutdown := TestProxy(t, nil)
	go serve()
	defer shutdown()

	var requestReceived bool
	srv := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		t.Logf("request %v", req.URL)
		requestReceived = true
	}))

	defer srv.Close()

	client := testClient(t, proxy.Addr, proxy.CertificateAuthority)

	res, err := client.Get(srv.URL)
	if err != nil {
		t.Fatal(err)
	}

	t.Logf("response received: %v", res.Status)

	if !requestReceived {
		t.Logf("expected request was not received")
	}
}
