package proxy

import (
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
	"time"

	"github.com/happal/osmosis/certauth"
	"golang.org/x/net/context/ctxhttp"
	"golang.org/x/net/http2"
)

// Proxy allows intercepting and modifying requests.
type Proxy struct {
	server       *http.Server
	client       *http.Client
	logger       *log.Logger
	serverConfig *tls.Config
	clientConfig *tls.Config

	ca *certauth.CertificateAuthority
}

// New initializes a proxy.
func New(address string, ca *certauth.CertificateAuthority) *Proxy {
	proxy := &Proxy{
		logger: log.New(os.Stdout, "server: ", 0),
		ca:     ca,
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

	// initialize HTTP client
	tr := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		Dial: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}).Dial,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		IdleConnTimeout:       15 * time.Second,
		TLSClientConfig:       proxy.clientConfig,
	}

	// enable http2
	http2.ConfigureTransport(tr)

	proxy.client = &http.Client{
		Transport: tr,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	return proxy
}

// filterHeaders contains a list of (lower-case) header names received from the
// client which are not sent to the upstream server.
var filterHeaders = map[string]struct{}{
	"proxy-connection": struct{}{},
	"connection":       struct{}{},
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

	for name, values := range proxyRequest.Header {
		if _, ok := filterHeaders[strings.ToLower(name)]; ok {
			// header is filtered, do not send it to the upstream server
			continue
		}
		req.Header[name] = values
	}
	return req, nil
}

// ServeHTTPProxy is called for each request the proxy receives.
func (p *Proxy) ServeHTTPProxy(res http.ResponseWriter, req *http.Request, forceHost, forceScheme string) (headerWritten bool, err error) {
	clientRequest, err := prepareRequest(req, forceHost, forceScheme)
	if err != nil {
		return false, err
	}

	response, err := ctxhttp.Do(req.Context(), p.client, clientRequest)
	if err != nil {
		return false, fmt.Errorf("error executing request: %v", err)
	}

	p.logger.Printf("%v %v -> %v", req.Method, req.URL, response.Status)

	// copy header
	for name, values := range response.Header {
		res.Header()[name] = values
	}

	res.WriteHeader(response.StatusCode)

	_, err = io.Copy(res, response.Body)
	if err != nil {
		return true, fmt.Errorf("error copying body: %v", err)
	}

	err = response.Body.Close()
	if err != nil {
		return true, fmt.Errorf("error closing body: %v", err)
	}

	return true, nil
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

type tlsListener struct {
	ch   chan *tls.Conn
	addr net.Addr
}

func (l *tlsListener) Accept() (net.Conn, error) {
	conn, ok := <-l.ch
	if !ok {
		return nil, nil
	}
	return conn, nil
}

func (l *tlsListener) Close() error {
	close(l.ch)
	return nil
}

func (l *tlsListener) Addr() net.Addr {
	return l.addr
}

// HandleConnect makes a connection to a target host and forwards all packets.
// If an error is returned, hijacking the connection hasn't worked.
func (p *Proxy) HandleConnect(responseWriter http.ResponseWriter, req *http.Request) error {
	p.logger.Printf("CONNECT %v from %v", req.URL.Host, req.RemoteAddr)
	forceHost := req.URL.Host

	hj, ok := responseWriter.(http.Hijacker)
	if !ok {
		return errors.New("responseWriter is not a hijacker")
	}

	conn, _, err := hj.Hijack()
	if err != nil {
		return err
	}
	defer conn.Close()

	err = writeConnectSuccess(conn)
	if err != nil {
		p.logger.Printf("unable to write proxy response: %v", err)
		writeConnectError(conn, err)
		return nil
	}

	tlsConn := tls.Server(conn, p.serverConfig)
	defer tlsConn.Close()

	err = tlsConn.Handshake()
	if err != nil {
		p.logger.Printf("TLS handshake for %v failed: %v", req.URL.Host, err)
		return nil
	}

	p.logger.Printf("TLS handshake for %v succeeded, next protocol: %v", req.URL.Host, tlsConn.ConnectionState().NegotiatedProtocol)

	srv := &http.Server{
		ErrorLog: p.logger,
		Handler: http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
			headerWritten, err := p.ServeHTTPProxy(res, req, forceHost, "https")
			if err != nil {
				p.logger.Printf("unable to prepare client request %v: %v", req.URL, err)
				if !headerWritten {
					// if we havn't written the response yet, inform the user if possible
					res.Header().Set("Content-Type", "text/plain; charset=utf-8")
					res.WriteHeader(http.StatusInternalServerError)
					fmt.Fprintf(res, "error: %v\n", err)
				}
			}
		}),
	}

	listener := &tlsListener{
		ch:   make(chan *tls.Conn, 1),
		addr: conn.RemoteAddr(),
	}
	listener.ch <- tlsConn
	err = srv.Serve(listener)
	return nil
}

func dumpResponse(res *http.Response) {
	buf, err := httputil.DumpResponse(res, false)
	if err == nil {
		fmt.Printf("--------------\n%s\n--------------\n", buf)
	}
}

func dumpRequest(req *http.Request) {
	buf, err := httputil.DumpRequest(req, false)
	if err == nil {
		fmt.Printf("--------------\n%s\n--------------\n", buf)
	}
}

func (p *Proxy) ServeHTTP(res http.ResponseWriter, req *http.Request) {
	// handle CONNECT requests for HTTPS
	if req.Method == http.MethodConnect {
		err := p.HandleConnect(res, req)
		if err != nil {
			p.logger.Printf("%v", err)
			res.WriteHeader(http.StatusInternalServerError)
			fmt.Fprintf(res, "%v", err)
		}
		return
	}

	// handle all other requests
	headerWritten, err := p.ServeHTTPProxy(res, req, "", "")
	if err != nil {
		p.logger.Printf("unable to prepare client request %v: %v", req.URL, err)
		if !headerWritten {
			// if we havn't written the response yet, inform the user if possible
			res.Header().Set("Content-Type", "text/plain; charset=utf-8")
			res.WriteHeader(http.StatusInternalServerError)
			fmt.Fprintf(res, "error: %v\n", err)
		}
	}
}

// Serve runs the proxy and answers requests.
func (p *Proxy) Serve() error {
	return p.server.ListenAndServe()
}
