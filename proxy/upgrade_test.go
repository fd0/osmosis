package proxy

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/fd0/osmosis/certauth"
	"github.com/gorilla/websocket"
)

func newWebsocketDialer(t testing.TB, proxyAddress string, ca *certauth.CertificateAuthority) *websocket.Dialer {
	proxyURL, err := url.Parse("http://" + proxyAddress)
	if err != nil {
		t.Fatal(err)
	}

	// build a cert pool to use for the HTTP client
	certPool := x509.NewCertPool()
	certPool.AddCert(ca.Certificate)

	return &websocket.Dialer{
		Proxy: func(*http.Request) (*url.URL, error) {
			return proxyURL, nil
		},
		HandshakeTimeout: 10 * time.Second,
		TLSClientConfig: &tls.Config{
			RootCAs: certPool,
		},
	}
}

func sendMessage(t testing.TB, conn *websocket.Conn, tpe int, data []byte) {
	err := conn.WriteMessage(tpe, data)
	if err != nil {
		t.Fatal(err)
	}
}

func wantNextMessage(t testing.TB, conn *websocket.Conn, tpe int, data []byte) {
	msgType, buf, err := conn.ReadMessage()
	if err != nil {
		t.Fatal(err)
	}

	if msgType != tpe {
		t.Errorf("received message has wrong type, want %v, got %v", tpe, msgType)
	}

	if !bytes.Equal(data, buf) {
		t.Errorf("received message with wrong data, want %q, got %q", data, buf)
	}
}

// newWebsocktTestServer returns a new httptest.Server which upgrades the
// incoming connection to websocket and then runs the handler function f.
func newWebsocktTestServer(t testing.TB, f func(*http.Request, *websocket.Conn)) (srv *httptest.Server, cleanup func()) {
	upgrader := websocket.Upgrader{}
	srv = httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		conn, err := upgrader.Upgrade(rw, req, nil)
		if err != nil {
			t.Fatal(err)
			return
		}
		defer conn.Close()

		f(req, conn)
	}))

	cleanup = func() {
		srv.CloseClientConnections()
		srv.Close()
	}

	return srv, cleanup
}

// newWebsocktTestTLSServer returns a new httptest.Server with TLS which
// upgrades the incoming connection to websocket and then runs the handler
// function f.
func newWebsocktTestTLSServer(t testing.TB, f func(*http.Request, *websocket.Conn)) (srv *httptest.Server, cleanup func()) {
	upgrader := websocket.Upgrader{}
	srv = httptest.NewTLSServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		conn, err := upgrader.Upgrade(rw, req, nil)
		if err != nil {
			t.Fatal(err)
			return
		}
		defer conn.Close()

		f(req, conn)
	}))

	cleanup = func() {
		srv.Close()
	}

	return srv, cleanup
}

// echoHandler returns a handler which echos back all messages until the
// websocket connection is closed.
func echoHandler(t testing.TB) func(*http.Request, *websocket.Conn) {
	return func(req *http.Request, conn *websocket.Conn) {
		for {
			t.Logf("handler: waiting for next message")
			msgType, buf, err := conn.ReadMessage()
			if websocket.IsCloseError(err, websocket.CloseNormalClosure) {
				t.Logf("handler: connection closed")
				return
			}

			if websocket.IsCloseError(err, websocket.CloseAbnormalClosure) {
				t.Logf("handler: connection closed abnormally")
				return
			}

			if err != nil {
				t.Fatalf("handler: error receiving message: %T %#v", err, err)
				fmt.Printf("handler: error receiving message: %T %#v", err, err)
				time.Sleep(2 * time.Second)
				return
			}

			t.Logf("handler: read message %v %s", msgType, buf)

			// echo the same message back
			err = conn.WriteMessage(msgType, buf)
			if err != nil {
				t.Fatalf("handler: error sending message: %v", err)
				return
			}

			t.Logf("handler: sent message %v %s", msgType, buf)
		}
	}
}

func TestProxyWebsocket(t *testing.T) {
	var tests = []struct {
		startServer func(t testing.TB) (srv *httptest.Server, cleanup func())
	}{
		{
			func(t testing.TB) (*httptest.Server, func()) {
				return newWebsocktTestServer(t, echoHandler(t))
			},
		},
		{
			func(t testing.TB) (*httptest.Server, func()) {
				return newWebsocktTestTLSServer(t, echoHandler(t))
			},
		},
	}

	for _, test := range tests {
		t.Run("", func(st *testing.T) {
			// run a test server
			srv, cleanup := test.startServer(st)
			defer func() {
				st.Logf("cleanup")
				cleanup()
			}()

			// run a proxy, ignore TLS certificates for outgoing connections
			proxy, serve, shutdown := TestProxy(t, &tls.Config{
				InsecureSkipVerify: true,
			})
			go serve()
			defer shutdown()

			// connect to the test server through the proxy
			wsDialer := newWebsocketDialer(st, proxy.Addr, proxy.CertificateAuthority)
			conn, res, err := wsDialer.Dial(strings.Replace(srv.URL, "http", "ws", 1), nil)
			if err != nil {
				st.Fatal(err)
			}

			wantStatus(st, res, http.StatusSwitchingProtocols)

			sendMessage(st, conn, websocket.TextMessage, []byte("foobar"))
			wantNextMessage(st, conn, websocket.TextMessage, []byte("foobar"))

			err = conn.WriteMessage(
				websocket.CloseMessage,
				websocket.FormatCloseMessage(websocket.CloseNormalClosure, "done"),
			)
			if err != nil {
				st.Fatal(err)
			}

			err = conn.Close()
			if err != nil {
				st.Fatal(err)
			}
		})
	}
}
