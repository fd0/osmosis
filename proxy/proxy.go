package proxy

import (
	"context"
	"crypto/tls"
	"io"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"github.com/fd0/osmosis/certauth"
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

	roundTripPipeline EventHook
}

// EventHook is a wrapper around ForwardRequest that is derived
// from the functions received through the Register function.
type EventHook func(*Event) (*Response, error)

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
func New(address string, ca *certauth.CertificateAuthority, clientConfig *tls.Config,
	logWriter io.Writer) *Proxy {
	if logWriter == nil {
		logWriter = ioutil.Discard
	}
	logger := log.New(logWriter, "", log.Ldate|log.Ltime|log.Lmicroseconds)
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

// Log exposes the proxy's logger to the user
func (p *Proxy) Log(msg string, args ...interface{}) {
	p.logger.Printf(msg, args...)
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
func (p *Proxy) ServeProxyRequest(event *Event) {
	// handle websockets
	if isWebsocketHandshake(event.Req) {
		HandleUpgradeRequest(event, p.clientConfig)
		return
	}

	err := event.prepareRequest()
	if err != nil {
		event.SendError("error preparing requests: %v", err)
		return
	}

	response, err := p.ForwardThroughPipeline(event)
	if err != nil {
		event.SendError("error executing request: %v", err)
		return
	}

	copyHeader(event.ResponseWriter.Header(), response.Header, response.Trailer)
	if len(response.Trailer) > 0 {
		event.Log("trailer detected, announcing: %v", response.Trailer)
		names := make([]string, 0, len(response.Trailer))
		for name := range response.Trailer {
			names = append(names, name)
		}

		// announce the trailers to the client
		event.ResponseWriter.Header().Set("Trailer", strings.Join(names, ", "))
	}

	event.ResponseWriter.WriteHeader(response.StatusCode)

	_, err = io.Copy(event.ResponseWriter, response.Body)
	if err != nil {
		event.Log("error copying body: %v", err)
		return
	}

	err = response.Body.Close()
	if err != nil {
		event.Log("error closing body: %v", err)
		return
	}

	// send the trailer values
	for name, values := range response.Trailer {
		for _, value := range values {
			event.ResponseWriter.Header().Add(name, value)
		}
	}
}

// ForwardRequest performs the given request using the proxy's http client.
// This function is also the core of the roundtrip pipeline.
func (p *Proxy) ForwardRequest(event *Event) (*Response, error) {
	httpResponse, err := ctxhttp.Do(event.Req.Context(), p.client, event.Req)
	if err != nil {
		return nil, err
	}
	return &Response{httpResponse}, nil
}

// ForwardThroughPipeline executes the round trip pipeline and handles the case where
// no pipeline function has been registred using the bare ForwardRequest function as
// a default.
func (p *Proxy) ForwardThroughPipeline(event *Event) (*http.Response, error) {
	if p.roundTripPipeline == nil {
		p.roundTripPipeline = p.ForwardRequest
	}
	response, err := p.roundTripPipeline(event)
	if err != nil {
		return nil, err
	}
	return response.Response, nil
}

// Register registers the given function in the proxy roundtrip pipeline
func (p *Proxy) Register(funcs ...func(*Event) (*Response, error)) {
	// the core of the pipeline (i.e. the innermost function) is ForwardRequest
	// all registered functions are wrapping layers around this initial value of
	// the roundTripPipeline
	if p.roundTripPipeline == nil {
		p.roundTripPipeline = p.ForwardRequest
	}

	for _, f := range funcs {
		// the anonymous function scope is used to create copies of the state
		// of f and p.roundTripPipeline in this loop iteration
		func(pipelineCopy func(*Event) (*Response, error),
			funcCopy func(*Event) (*Response, error)) {
			// now the function f will be wrapped around the current pipeline
			p.roundTripPipeline = func(e *Event) (*Response, error) {
				e.ForwardRequest = func() (*Response, error) {
					return pipelineCopy(e)
				}
				response, err := funcCopy(e)
				if err != nil {
					return nil, err
				}
				return response, nil
			}
		}(p.roundTripPipeline, f)

	}
}

// ResetPipeline removes all previously registered functions from the pipeline
func (p *Proxy) ResetPipeline() {
	p.roundTripPipeline = p.ForwardRequest
}

func (p *Proxy) nextRequestID() uint64 {
	return atomic.AddUint64(&p.requestID, 1)
}

// isWebsocketHandshake returns true if the request tries to initiate a websocket handshake.
func isWebsocketHandshake(req *http.Request) bool {
	upgrade := strings.ToLower(req.Header.Get("upgrade"))
	return strings.Contains(upgrade, "websocket")
}

func (p *Proxy) ServeHTTP(responseWriter http.ResponseWriter, httpRequest *http.Request) {
	event := newEvent(responseWriter, httpRequest, p.logger, p.nextRequestID())

	// handle CONNECT requests for HTTPS
	if event.Req.Method == http.MethodConnect {
		ServeConnect(event, p.serverConfig, p.Cache, p.logger, p.nextRequestID, p.ServeProxyRequest)
		return
	}

	// serve certificate for easier importing
	if event.Req.URL.Hostname() == "proxy" {
		ServeStatic(event.ResponseWriter, event.Req, p.CertificateAuthority.CertificateAsPEM())
		return
	}

	// handle all other requests
	p.ServeProxyRequest(event)
}

// ListenAndServe starts the listener and runs the proxy.
func (p *Proxy) ListenAndServe() error {
	p.logger.Printf("Listening on %s\n", p.server.Addr)
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
