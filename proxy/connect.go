package proxy

import (
	"bufio"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
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

// CertificateCreater creates a new certificate.
type CertificateCreater interface {
	NewCertificate(name string, altNames []string) (*x509.Certificate, *rsa.PrivateKey, error)
}

// ServeConnect makes a connection to a target host and forwards all packets.
// If an error is returned, hijacking the connection hasn't worked.
func ServeConnect(req *Request, tlsConfig *tls.Config, ca CertificateCreater, errorLogger *log.Logger, nextRequestID func() uint64, serveProxyRequest func(*Request)) {
	req.Log("CONNECT %v %v %v", req.ForceScheme, req.ForceHost, req.URL.Host)

	hj, ok := req.ResponseWriter.(http.Hijacker)
	if !ok {
		req.SendError("unable to reuse connection for CONNECT")
		return
	}

	conn, rw, err := hj.Hijack()
	if err != nil {
		req.SendError("reusing connection failed: %v", err)
		return
	}

	err = rw.Flush()
	if err != nil {
		req.Log("flush failed: %v", err)
		writeConnectError(conn, err)
		conn.Close()
		return
	}

	err = writeConnectSuccess(conn)
	if err != nil {
		req.Log("unable to write proxy response: %v", err)
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
		req.Log("peek(1) failed: %v", err)
		conn.Close()
		return
	}

	listener := &fakeListener{
		ch:   make(chan net.Conn, 1),
		addr: conn.RemoteAddr(),
	}

	var forceHost = req.URL.Host
	var forceScheme string
	var parentID = req.ID

	// TLS client hello starts with 0x16
	if buf[0] == 0x16 {

		// create new TLS config for this server, copying all values from tlsConfig
		var cfg = tlsConfig.Clone()

		// generate a new certificate on the fly for the client
		cfg.GetCertificate = func(ch *tls.ClientHelloInfo) (*tls.Certificate, error) {
			name := ch.ServerName

			// client did not include SNI in ClientHello, so we'll use forceHost instead
			if name == "" {
				data := strings.Split(forceHost, ":")
				req.Log("client did not include SNI, using %v", data[0])
				name = data[0]
			}

			crt, key, err := ca.NewCertificate(name, []string{name})
			if err != nil {
				return nil, err
			}

			req.Log("new certificate names: %v", crt.DNSNames)
			req.Log("new certificate ips: %v", crt.IPAddresses)

			tlscrt := &tls.Certificate{
				Certificate: [][]byte{
					crt.Raw,
				},
				PrivateKey: key,
			}

			return tlscrt, nil
		}

		tlsConn := tls.Server(bconn, cfg)

		err = tlsConn.Handshake()
		if err != nil {
			req.Log("TLS handshake for %v failed: %v", req.URL.Host, err)
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

	logger := req.Logger

	srv := &http.Server{
		ErrorLog: errorLogger,
		Handler: http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
			nextID := parentID
			if nextID == 0 {
				nextID = nextRequestID()
			}
			preq := newRequest(res, req, logger, nextID)
			// send all requests to the host we were told to connect to
			preq.ForceHost = forceHost
			preq.ForceScheme = forceScheme

			serveProxyRequest(preq)
		}),
	}

	// handle all incoming requests
	err = srv.Serve(listener)
	if err == errFakeListenerEOF {
		err = nil
	}

	if err != nil {
		req.Log("error serving connection: %v", err)
	}
}
