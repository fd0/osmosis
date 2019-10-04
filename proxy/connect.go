package proxy

import (
	"bufio"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
)

type buffConn struct {
	*bufio.Reader
	net.Conn
}

func (b buffConn) Read(p []byte) (int, error) {
	return b.Reader.Read(p)
}

var errFakeListenerEOF = errors.New("listener has no more connections")

type fakeListener struct {
	ch   chan net.Conn
	addr net.Addr
}

func (l *fakeListener) Accept() (net.Conn, error) {
	conn, ok := <-l.ch
	if !ok {
		return nil, errFakeListenerEOF
	}
	return conn, nil
}

func (l *fakeListener) Close() error {
	return nil
}

func (l *fakeListener) Addr() net.Addr {
	return l.addr
}

func writeConnectSuccess(wr io.Writer) error {
	res := http.Response{
		Proto:         "HTTP/1.0",
		ProtoMajor:    1,
		ProtoMinor:    0,
		Status:        http.StatusText(http.StatusOK),
		StatusCode:    http.StatusOK,
		ContentLength: -1,
	}

	return res.Write(wr)
}

func writeConnectError(wr io.WriteCloser, err error) {
	res := http.Response{
		Proto:         "HTTP/1.0",
		ProtoMajor:    1,
		ProtoMinor:    0,
		Status:        http.StatusText(http.StatusInternalServerError),
		StatusCode:    http.StatusInternalServerError,
		ContentLength: -1,
	}

	res.Write(wr)
	fmt.Fprintf(wr, "error: %v\n", err)
	wr.Close()
}

// ServeConnect makes a connection to a target host and forwards all packets.
// If an error is returned, hijacking the connection hasn't worked.
func ServeConnect(event *Event, tlsConfig *tls.Config, certCache *Cache, errorLogger *log.Logger, nextRequestID func() uint64, serveProxyRequest func(*Event)) {
	hj, ok := event.ResponseWriter.(http.Hijacker)
	if !ok {
		event.SendError("unable to reuse connection for CONNECT")
		return
	}

	conn, rw, err := hj.Hijack()
	if err != nil {
		event.SendError("reusing connection failed: %v", err)
		return
	}

	err = rw.Flush()
	if err != nil {
		event.Log("flush failed: %v", err)
		writeConnectError(conn, err)
		conn.Close()
		return
	}

	err = writeConnectSuccess(conn)
	if err != nil {
		event.Log("unable to write proxy response: %v", err)
		writeConnectError(conn, err)
		conn.Close()
		return
	}

	// try to find out if the client tries to setup TLS
	bconn := buffConn{
		Reader: bufio.NewReader(conn),
		Conn:   conn,
	}

	buf, err := bconn.Peek(1)
	if err != nil {
		event.Log("peek(1) failed: %v", err)
		conn.Close()
		return
	}

	listener := &fakeListener{
		ch:   make(chan net.Conn, 1),
		addr: conn.RemoteAddr(),
	}

	var forceHost = event.Req.URL.Host
	if event.ForceHost != "" {
		forceHost = event.ForceHost
	}
	var forceScheme string
	var parentID = event.ID

	// TLS client hello starts with 0x16
	if buf[0] == 0x16 {

		// create new TLS config for this server, copying all values from tlsConfig
		var cfg = tlsConfig.Clone()

		// generate a new certificate on the fly for the client
		cfg.GetCertificate = func(ch *tls.ClientHelloInfo) (*tls.Certificate, error) {
			return certCache.Get(event.Req.Context(), forceHost, ch.ServerName)
		}

		tlsConn := tls.Server(bconn, cfg)

		err = tlsConn.Handshake()
		if err != nil {
			event.Log("TLS handshake for %v failed: %v", event.Req.URL.Host, err)
			return
		}

		// req.Log("TLS handshake for %v succeeded, next protocol: %v", req.URL.Host, tlsConn.ConnectionState().NegotiatedProtocol)

		listener.ch <- tlsConn
		close(listener.ch)

		// use new request IDs for HTTP2
		if tlsConn.ConnectionState().NegotiatedProtocol == "h2" {
			parentID = 0
		}

		// handle the next requests as HTTPS
		forceScheme = "https"

	} else {
		listener.ch <- bconn
		close(listener.ch)

		// handle the next requests as HTTP
		forceScheme = "http"
	}

	logger := event.Logger

	srv := &http.Server{
		ErrorLog: errorLogger,
		Handler: http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
			nextID := parentID
			if nextID == 0 {
				nextID = nextRequestID()
			}
			event := newEvent(res, req, logger, nextID)
			// send all requests to the host we were told to connect to
			event.ForceHost = forceHost
			event.ForceScheme = forceScheme

			serveProxyRequest(event)
		}),
	}

	// handle all incoming requests
	err = srv.Serve(listener)
	if err == errFakeListenerEOF {
		err = nil
	}

	if err != nil {
		event.Log("error serving connection: %v", err)
	}
}
