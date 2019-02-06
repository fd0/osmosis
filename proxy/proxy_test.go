package proxy

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"io/ioutil"
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

func wantStatus(t testing.TB, res *http.Response, code int) {
	if res.StatusCode != code {
		t.Errorf("wrong status code received: want %v, got %v", code, res.StatusCode)
	}
}

func wantBody(t testing.TB, res *http.Response, body string) {
	buf, err := ioutil.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}

	err = res.Body.Close()
	if err != nil {
		t.Fatal(err)
	}

	if string(buf) != body {
		t.Errorf("unexpected body received: want %q, got %q", body, buf)
	}
}

func wantHeader(t testing.TB, res *http.Response, want map[string]string) {
	for name, value := range want {
		if res.Header.Get(name) != value {
			t.Errorf("wrong value for header %v: want %q, got %q", name, value, res.Header.Get(name))
		}
	}
}

func wantTrailer(t testing.TB, res *http.Response, want map[string]string) {
	for name, value := range want {
		if res.Trailer.Get(name) != value {
			t.Errorf("wrong value for header %v: want %q, got %q", name, value, res.Trailer.Get(name))
		}
	}
}

func TestProxyTrailer(t *testing.T) {
	proxy, serve, shutdown := TestProxy(t, nil)
	go serve()
	defer shutdown()

	srv := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		rw.Header().Set("Trailer", "Content-Hash") // signal that this header should be sent as trailer
		rw.Header().Set("Content-Type", "text/plain; charset=utf-8")
		rw.Header().Set("Content-Hash", "1234")
		rw.WriteHeader(http.StatusOK)
		io.WriteString(rw, "body string\n")
	}))
	defer srv.Close()

	fmt.Printf("proxy listening on %v, server on %v\n", proxy.Addr, srv.URL)

	client := testClient(t, proxy.Addr, proxy.CertificateAuthority)

	res, err := client.Get(srv.URL)
	if err != nil {
		t.Fatal(err)
	}

	wantStatus(t, res, http.StatusOK)
	wantBody(t, res, "body string\n")
	wantHeader(t, res, map[string]string{"Content-Type": "text/plain; charset=utf-8"})
	wantTrailer(t, res, map[string]string{"Content-Hash": "1234"})
}
