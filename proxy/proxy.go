package proxy

import (
	"bufio"
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"github.com/happal/osmosis/certauth"
	"golang.org/x/net/context/ctxhttp"
	"golang.org/x/net/http2"
)

// Proxy allows intercepting and modifying requests.
type Proxy struct {
	server       *http.Server
	serverConfig *tls.Config

	requestID uint64

	client       *http.Client
	clientConfig *tls.Config

	logger *log.Logger

	*certauth.CertificateAuthority
	*Cache
	Addr string

	OnResponse func(*Request, *http.Response)
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

// New returns a new proxy which generates certificates on demand and signs
// them with using ca. The clientConfig is used for outgoing TLS client
// connections.
func New(address string, ca *certauth.CertificateAuthority, clientConfig *tls.Config) *Proxy {
	logger := log.New(os.Stdout, "", log.Ldate|log.Ltime|log.Lmicroseconds)
	proxy := &Proxy{
		logger:               logger,
		CertificateAuthority: ca,
		Cache:                NewCache(ca, clientConfig, logger),
		Addr:                 address,
	}

	// TLS server configuration
	proxy.serverConfig = &tls.Config{
		// advertise HTTP2
		NextProtos:    []string{"h2", "http/1.1"},
		Renegotiation: 0,
	}

	// initialize HTTP server
	proxy.server = &http.Server{
		Addr:     address,
		ErrorLog: proxy.logger,
		Handler:  proxy,
	}

	// initialize HTTP client to use
	proxy.client = newHTTPClient(true, clientConfig)
	proxy.clientConfig = clientConfig

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
	"sec-websocket-key":        "Sec-WebSocket-Key",
	"sec-websocket-version":    "Sec-WebSocket-Version",
	"sec-websocket-protocol":   "Sec-WebSocket-Protocol",
	"sec-websocket-extensions": "Sec-WebSocket-Extensions",
}

type bufferedReadCloser struct {
	io.Reader
	io.Closer
}

func prepareRequest(proxyRequest *http.Request, host, scheme string) (*http.Request, error) {
	url := proxyRequest.URL
	if host != "" {
		url.Scheme = scheme
		url.Host = host
	}

	// try to find out if the body is non-nil but won't yield any data
	var body = proxyRequest.Body
	if proxyRequest.Body != nil {
		rd := bufio.NewReader(proxyRequest.Body)
		buf, err := rd.Peek(1)
		if err == io.EOF || len(buf) == 0 {
			// if the body is non-nil but nothing can be read from it we set the body to http.NoBody
			// this happens for incoming http2 connections
			body = http.NoBody
		} else {
			body = bufferedReadCloser{
				Reader: rd,
				Closer: proxyRequest.Body,
			}
		}
	}

	req, err := http.NewRequest(proxyRequest.Method, url.String(), body)
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
	// handle websockets
	if isWebsocketHandshake(req.Request) {
		HandleUpgradeRequest(req, p.clientConfig)
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

	req.Log("%v %v %v", response.StatusCode, req.Request.Method, req.Request.URL)

	if p.OnResponse != nil {
		p.OnResponse(req, response)
	}

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
			req.ResponseWriter.Header().Add(name, value)
		}
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

func (p *Proxy) nextRequestID() uint64 {
	return atomic.AddUint64(&p.requestID, 1)
}

func newRequest(rw http.ResponseWriter, req *http.Request, logger *log.Logger, id uint64) *Request {
	return &Request{
		ID:             id,
		Request:        req,
		ResponseWriter: rw,
		Logger:         logger,
	}
}

// isWebsocketHandshake returns true if the request tries to initiate a websocket handshake.
func isWebsocketHandshake(req *http.Request) bool {
	upgrade := strings.ToLower(req.Header.Get("upgrade"))
	return strings.Contains(upgrade, "websocket")
}

func (p *Proxy) ServeHTTP(responseWriter http.ResponseWriter, httpRequest *http.Request) {
	req := newRequest(responseWriter, httpRequest, p.logger, p.nextRequestID())

	// handle CONNECT requests for HTTPS
	if req.Method == http.MethodConnect {
		ServeConnect(req, p.serverConfig, p.Cache, p.logger, p.nextRequestID, p.ServeProxyRequest)
		return
	}

	// serve certificate for easier importing
	if req.URL.Hostname() == "proxy" {
		ServeStatic(req.ResponseWriter, req.Request, p.CertificateAuthority.CertificateAsPEM())
		return
	}

	// handle all other requests
	p.ServeProxyRequest(req)
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
