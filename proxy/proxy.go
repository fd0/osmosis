package proxy

import (
	"bufio"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"github.com/happal/osmosis/certauth"
	"golang.org/x/net/context/ctxhttp"
	"golang.org/x/net/http2"
	"golang.org/x/sync/errgroup"
)

// Proxy allows intercepting and modifying requests.
type Proxy struct {
	server       *http.Server
	serverConfig *tls.Config

	requestID uint64

	client *http.Client

	logger *log.Logger

	*certauth.CertificateAuthority
	Addr string
}

func newHTTPClient(enableHTTP2 bool, cfg *tls.Config) *http.Client {
	// initialize HTTP client
	tr := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		Dial: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}).Dial,
		TLSHandshakeTimeout:   30 * time.Second,
		ResponseHeaderTimeout: 60 * time.Second,
		ExpectContinueTimeout: 5 * time.Second,
		IdleConnTimeout:       60 * time.Second,
		TLSClientConfig:       cfg,
	}

	if enableHTTP2 {
		http2.ConfigureTransport(tr)
	}

	return &http.Client{
		Transport: tr,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

// New initializes a proxy.
func New(address string, ca *certauth.CertificateAuthority, clientConfig *tls.Config) *Proxy {
	proxy := &Proxy{
		logger:               log.New(os.Stdout, "", log.Ldate|log.Ltime|log.Lmicroseconds),
		CertificateAuthority: ca,
		Addr:                 address,
	}
	proxy.serverConfig = &tls.Config{
		NextProtos: []string{"h2", "http/1.1"},
		GetCertificate: func(ch *tls.ClientHelloInfo) (*tls.Certificate, error) {
			crt, err := ca.NewCertificate(ch.ServerName, []string{ch.ServerName})
			if err != nil {
				return nil, err
			}

			tlscrt := &tls.Certificate{
				Certificate: [][]byte{
					crt.Raw,
				},
				PrivateKey: ca.Key,
			}

			return tlscrt, nil
		},
		Renegotiation: 0,
	}

	// initialize HTTP server
	proxy.server = &http.Server{
		Addr:     address,
		ErrorLog: proxy.logger,
		Handler:  proxy,
	}

	proxy.client = newHTTPClient(true, clientConfig)

	return proxy
}

// filterHeaders contains a list of (lower-case) header names received from the
// client which are not sent to the upstream server.
var filterHeaders = map[string]struct{}{
	"proxy-connection": struct{}{},
	"connection":       struct{}{},
}

// renameHeaders contains a list of header names which must be have a special
// (mixed-case)representation, which is normalized away by default by the Go
// http.Header struct.
var renameHeaders = map[string]string{
	"sec-websocket-key":      "Sec-WebSocket-Key",
	"sec-websocket-version":  "Sec-WebSocket-Version",
	"sec-websocket-protocol": "Sec-WebSocket-Protocol",
}

func prepareRequest(proxyRequest *http.Request, host, scheme string) (*http.Request, error) {
	url := proxyRequest.URL
	if host != "" {
		url.Scheme = scheme
		url.Host = host
	}

	req, err := http.NewRequest(proxyRequest.Method, url.String(), proxyRequest.Body)
	if err != nil {
		return nil, err
	}

	// use Host header from received request
	req.Host = proxyRequest.Host

	for name, values := range proxyRequest.Header {
		if _, ok := filterHeaders[strings.ToLower(name)]; ok {
			// header is filtered, do not send it to the upstream server
			continue
		}

		if newname, ok := renameHeaders[strings.ToLower(name)]; ok {
			name = newname
		}
		req.Header[name] = values
	}

	req.ContentLength = proxyRequest.ContentLength

	return req, nil
}

// isWebsocketHandshake returns true if the request tries to initiate a websocket handshake.
func isWebsocketHandshake(req *http.Request) bool {
	upgrade := strings.ToLower(req.Header.Get("upgrade"))
	return strings.Contains(upgrade, "websocket")
}

func copyHeader(dst, src, trailer http.Header) {
	for name, values := range src {
		for _, value := range values {
			// ignore the field if it should be a trailer
			if _, ok := trailer[name]; ok {
				continue
			}
			dst.Add(name, value)
		}
	}
}

// ServeProxyRequest is called for each request the proxy receives.
func (p *Proxy) ServeProxyRequest(req *Request) {
	req.Log("%v %v %v %v", req.Request.Method, req.ForceScheme, req.ForceHost, req.Request.URL)

	if isWebsocketHandshake(req.Request) {
		p.HandleUpgradeRequest(req)
		return
	}

	clientRequest, err := prepareRequest(req.Request, req.ForceHost, req.ForceScheme)
	if err != nil {
		req.SendError("error preparing request: %v", err)
		return
	}

	response, err := ctxhttp.Do(req.Context(), p.client, clientRequest)
	if err != nil {
		req.SendError("error executing request: %v", err)
		return
	}

	req.Log("   -> %v", response.Status)

	copyHeader(req.ResponseWriter.Header(), response.Header, response.Trailer)
	if len(response.Trailer) > 0 {
		req.Log("trailer detected, announcing: %v", response.Trailer)
		names := make([]string, 0, len(response.Trailer))
		for name := range response.Trailer {
			names = append(names, name)
		}

		// announce the trailers to the client
		req.ResponseWriter.Header().Set("Trailer", strings.Join(names, ", "))
	}

	req.ResponseWriter.WriteHeader(response.StatusCode)

	_, err = io.Copy(req.ResponseWriter, response.Body)
	if err != nil {
		req.Log("error copying body: %v", err)
		return
	}

	err = response.Body.Close()
	if err != nil {
		req.Log("error closing body: %v", err)
		return
	}

	// send the trailer values
	for name, values := range response.Trailer {
		for _, value := range values {
			req.ResponseWriter.Header().Set(name, value)
		}
	}
}

func copyUntilError(src, dst io.ReadWriteCloser) error {
	var g errgroup.Group
	g.Go(func() error {
		_, err := io.Copy(src, dst)
		src.Close()
		dst.Close()
		return err
	})

	g.Go(func() error {
		_, err := io.Copy(dst, src)
		src.Close()
		dst.Close()
		return err
	})

	return g.Wait()
}

// HandleUpgradeRequest handles an upgraded connection (e.g. websockets).
func (p *Proxy) HandleUpgradeRequest(req *Request) {
	reqUpgrade := req.Request.Header.Get("upgrade")
	req.Log("handle upgrade request to %v", reqUpgrade)

	host := req.URL.Host
	if req.ForceHost != "" {
		host = req.ForceHost
	}

	scheme := req.URL.Scheme
	if req.ForceHost != "" {
		scheme = req.ForceScheme
	}

	var outgoingConn net.Conn
	var err error

	if scheme == "https" {
		outgoingConn, err = tls.Dial("tcp", host, nil)
	} else {
		outgoingConn, err = net.Dial("tcp", host)
	}

	if err != nil {
		req.SendError("connecting to %v failed: %v", host, err)
		req.Body.Close()
		return
	}

	defer outgoingConn.Close()

	req.Log("connected to %v", host)

	outReq, err := prepareRequest(req.Request, host, scheme)
	if err != nil {
		req.SendError("preparing request to %v failed: %v", host, err)
		req.Body.Close()
		return
	}

	// put back the "Connection" header
	outReq.Header.Set("connection", req.Request.Header.Get("connection"))

	dumpRequest(req.Request)
	dumpRequest(outReq)

	err = outReq.Write(outgoingConn)
	if err != nil {
		req.SendError("unable to forward request to %v: %v", host, err)
		req.Body.Close()
		return
	}

	outgoingReader := bufio.NewReader(outgoingConn)
	outRes, err := http.ReadResponse(outgoingReader, outReq)
	if err != nil {
		req.SendError("unable to read response from %v: %v", host, err)
		req.Body.Close()
		return
	}

	dumpResponse(outRes)

	hj, ok := req.ResponseWriter.(http.Hijacker)
	if !ok {
		req.SendError("switching protocols failed, incoming connection is not bidirectional")
		req.Body.Close()
		return
	}

	clientConn, _, err := hj.Hijack()
	if !ok {
		req.SendError("switching protocols failed, hijacking incoming connection failed: %v", err)
		req.Body.Close()
		return
	}
	defer clientConn.Close()

	err = outRes.Write(clientConn)
	if err != nil {
		req.Log("writing response to client failed: %v", err)
		return
	}

	req.Log("start forwarding data")
	err = copyUntilError(outgoingConn, clientConn)
	if err != nil {
		req.Log("copying data for websocket returned error: %v", err)
	}
	req.Log("connection done")
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

type buffConn struct {
	*bufio.Reader
	net.Conn
}

func (b buffConn) Read(p []byte) (int, error) {
	return b.Reader.Read(p)
}

// HandleConnect makes a connection to a target host and forwards all packets.
// If an error is returned, hijacking the connection hasn't worked.
func (p *Proxy) HandleConnect(req *Request) {
	req.Log("CONNECT %v %v %v", req.ForceScheme, req.ForceHost, req.URL.Host)
	forceHost := req.URL.Host

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

	rw.Flush()

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

	var srv *http.Server

	// TLS client hello starts with 0x16
	if buf[0] == 0x16 {
		tlsConn := tls.Server(bconn, p.serverConfig)

		err = tlsConn.Handshake()
		if err != nil {
			req.Log("TLS handshake for %v failed: %v", req.URL.Host, err)
			return
		}

		// req.Log("TLS handshake for %v succeeded, next protocol: %v", req.URL.Host, tlsConn.ConnectionState().NegotiatedProtocol)

		listener.ch <- tlsConn
		close(listener.ch)

		parentID := req.ID

		// use new request IDs for HTTP2
		if tlsConn.ConnectionState().NegotiatedProtocol == "h2" {
			parentID = 0
		}

		// handle the next requests as HTTPS
		srv = &http.Server{
			ErrorLog: p.logger,
			Handler: http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {

				preq := p.newRequest(res, req, parentID)
				// send all requests to the host we were told to connect to
				preq.ForceHost = forceHost
				preq.ForceScheme = "https"

				p.ServeProxyRequest(preq)
			}),
		}
	} else {
		listener.ch <- bconn
		close(listener.ch)

		parentID := req.ID

		// handle the next requests as HTTP
		srv = &http.Server{
			ErrorLog: p.logger,
			Handler: http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
				preq := p.newRequest(res, req, parentID)
				// send all requests to the host we were told to connect to
				preq.ForceHost = forceHost
				preq.ForceScheme = "http"

				p.ServeProxyRequest(preq)
			}),
		}
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

func dumpResponse(res *http.Response) {
	buf, err := httputil.DumpResponse(res, true)
	if err == nil {
		fmt.Printf("--------------\n%s\n--------------\n", buf)
		// fmt.Printf("body: %#v\n", res.Body)
	}
}

func dumpRequest(req *http.Request) {
	buf, err := httputil.DumpRequest(req, true)
	if err == nil {
		fmt.Printf("--------------\n%s\n--------------\n", buf)
		// fmt.Printf("body: %#v\n", req.Body)
	}
}

// Request is a request received by the proxy.
type Request struct {
	ID uint64

	*http.Request
	http.ResponseWriter

	ForceHost, ForceScheme string

	*log.Logger
}

// Log logs a message through the embedded logger, prefixed with the request.
func (req *Request) Log(msg string, args ...interface{}) {
	args = append([]interface{}{req.ID, req.Request.RemoteAddr}, args...)
	req.Logger.Printf("[%4d %v] "+msg, args...)
}

// SendError responds with an error (which is also logged).
func (req *Request) SendError(msg string, args ...interface{}) {
	req.Log(msg, args...)
	req.ResponseWriter.Header().Set("Content-Type", "text/plain")
	req.ResponseWriter.WriteHeader(http.StatusInternalServerError)
	fmt.Fprintf(req.ResponseWriter, msg, args...)
}

func (p *Proxy) newRequest(rw http.ResponseWriter, req *http.Request, id uint64) *Request {
	if id == 0 {
		id = atomic.AddUint64(&p.requestID, 1)
	}

	return &Request{
		ID:             id,
		Request:        req,
		ResponseWriter: rw,
		Logger:         p.logger,
	}
}

func (p *Proxy) ServeHTTP(responseWriter http.ResponseWriter, httpRequest *http.Request) {
	req := p.newRequest(responseWriter, httpRequest, 0)
	defer func() {
		req.Log("done")
	}()

	// handle CONNECT requests for HTTPS
	if req.Method == http.MethodConnect {
		p.HandleConnect(req)
		return
	}

	// serve certificate for easier importing
	if req.URL.Hostname() == "proxy" && req.URL.Path == "/ca" {
		p.ServeCA(req)
		return
	}

	// handle all other requests
	p.ServeProxyRequest(req)
}

// ServeCA returns the PEM encoded CA certificate.
func (p *Proxy) ServeCA(req *Request) {
	req.ResponseWriter.Header().Set("Content-Type", "application/x-x509-ca-cert")
	req.ResponseWriter.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	req.ResponseWriter.Header().Set("Pragma", "no-cache")
	req.ResponseWriter.Header().Set("Expires", "0")

	req.ResponseWriter.WriteHeader(http.StatusOK)
	req.ResponseWriter.Write(p.CertificateAuthority.CertificateAsPEM())
}

// ListenAndServe starts the listener and runs the proxy.
func (p *Proxy) ListenAndServe() error {
	listener, err := net.Listen("tcp", p.server.Addr)
	if err != nil {
		return err
	}

	return p.Serve(listener)
}

// Serve runs the proxy and answers requests.
func (p *Proxy) Serve(listener net.Listener) error {
	return p.server.Serve(listener)
}

// Shutdown closes the proxy gracefully.
func (p *Proxy) Shutdown(ctx context.Context) error {
	return p.server.Shutdown(ctx)
}
