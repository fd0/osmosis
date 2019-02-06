package proxy

import (
	"context"
	"crypto/tls"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/happal/osmosis/certauth"
)

// newLocalListener returns a new listener using a tcp port selected
// dynamically.
func newLocalListener(t testing.TB) net.Listener {
	listener, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatal(err)
	}

	return listener
}

// TestProxy returns a proxy suitable for testing, running on a dynamic port on
// localhost. The returned function serve needs to be run in order for the
// proxy to process requests. Use shutdown to properly close the proxy.
func TestProxy(t testing.TB, cfg *tls.Config) (proxy *Proxy, serve, shutdown func()) {
	ca := certauth.TestCA(t)
	listener := newLocalListener(t)

	proxy = New(listener.Addr().String(), ca, cfg)

	serve = func() {
		err := proxy.Serve(listener)
		if err == http.ErrServerClosed {
			err = nil
		}

		if err != nil {
			t.Fatal(err)
		}
	}

	shutdown = func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		err := proxy.Shutdown(ctx)
		if err != nil {
			t.Fatal(err)
		}
	}

	return proxy, serve, shutdown
}
