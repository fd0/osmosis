package proxy

import (
	"bytes"
	"crypto/x509"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/happal/osmosis/certauth"
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
func newWebsocktTestServer(t testing.TB, f func(*http.Request, *websocket.Conn)) *httptest.Server {
	upgrader := websocket.Upgrader{}
	return httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		conn, err := upgrader.Upgrade(rw, req, nil)
		if err != nil {
			t.Fatal(err)
			return
		}
		defer conn.Close()

		f(req, conn)
	}))
}

func TestProxyWebsocket(t *testing.T) {
	proxy, serve, shutdown := TestProxy(t, nil)
	go serve()
	defer shutdown()

	srv := newWebsocktTestServer(t, func(req *http.Request, conn *websocket.Conn) {
		for {
			fmt.Printf("handler: waiting for next message\n")
			msgType, buf, err := conn.ReadMessage()
			if websocket.IsCloseError(err, websocket.CloseNormalClosure) {
				fmt.Printf("handler: connection closed\n")
				break
			}

			if err != nil {
				t.Fatalf("handler: error receiving message: %#v", err)
				return
			}

			fmt.Printf("handler: read message %v %s\n", msgType, buf)

			// echo the same message back
			err = conn.WriteMessage(msgType, buf)
			if err != nil {
				t.Fatalf("handler: error sending message: %v", err)
				return
			}

			fmt.Printf("handler: sent message %v %s\n", msgType, buf)
		}
	})
	defer srv.Close()

	wsDialer := newWebsocketDialer(t, proxy.Addr, proxy.CertificateAuthority)
	conn, res, err := wsDialer.Dial(strings.Replace(srv.URL, "http", "ws", 1), nil)
	if err != nil {
		t.Fatal(err)
	}

	wantStatus(t, res, http.StatusSwitchingProtocols)

	sendMessage(t, conn, websocket.TextMessage, []byte("foobar"))
	wantNextMessage(t, conn, websocket.TextMessage, []byte("foobar"))

	err = conn.WriteMessage(
		websocket.CloseMessage,
		websocket.FormatCloseMessage(websocket.CloseNormalClosure, "done"),
	)
	if err != nil {
		t.Fatal(err)
	}

	conn.Close()
}
