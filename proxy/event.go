package proxy

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/http/httputil"
	"strings"
)

// Event represents the event of an incoming request into the proxy.
// In addition to the request itself, the event contains the proxy
// context such as a contextual logger or the request ID. Such an
// event is the data structure on which the registered hooks operate.
// These hooks can also use the event to eventually forwared the
// initial request to the originally intended target.
type Event struct {
	ID uint64

	Req *http.Request
	http.ResponseWriter

	ForceHost, ForceScheme string

	ForwardRequest func() (*Response, error)
	Abort          context.CancelFunc

	*log.Logger
}

func newEvent(rw http.ResponseWriter, req *http.Request, logger *log.Logger, id uint64) *Event {
	return &Event{
		ID:             id,
		Req:            req,
		ResponseWriter: rw,
		ForwardRequest: func() (*Response, error) {
			return nil, fmt.Errorf("no forward action defined")
		},
		Abort:  func() {},
		Logger: logger,
	}
}

// readWithoutClose returns the content as byte slice by
// reading it it fully and replacing the original body
// ReadClose with a NopCloser over the byte slice.
func readWithoutClose(body *io.ReadCloser) ([]byte, error) {
	savedBody, err := ioutil.ReadAll(*body)
	if err != nil {
		return nil, fmt.Errorf("ReadAll: %v", err)
	}
	err = (*body).Close()
	if err != nil {
		return nil, fmt.Errorf("closing body: %v", err)
	}
	*body = ioutil.NopCloser(bytes.NewBuffer(savedBody))
	return savedBody, nil
}

// RawRequest returns the raw request bytes in HTTP/1.1
// wire format
func (e *Event) RawRequest() ([]byte, error) {
	// make sure that the body is a NopCloser
	_, err := readWithoutClose(&e.Req.Body)
	if err != nil {
		return nil, fmt.Errorf("readWithoutClose: %v", err)
	}
	dump, err := httputil.DumpRequest(e.Req, true)
	if err != nil {
		return nil, fmt.Errorf("writing request: %v", err)
	}
	return dump, nil
}

// RawRequestBody body returns the request body as a
// byte slice leaving the original Body as an unread
// io.NopCloser over the same bytes.
func (e *Event) RawRequestBody() ([]byte, error) {
	return readWithoutClose(&e.Req.Body)
}

// SetRequestBody sets the Body of the underlying event
// to a NopCloser over the given bytes.
func (e *Event) SetRequestBody(body []byte) {
	e.Req.Body = ioutil.NopCloser(bytes.NewBuffer(body))
}

// SetRequest sets the event's request to a new request
// parsed from the provided byte slice
func (e *Event) SetRequest(rawRequest []byte) error {
	req, err := http.ReadRequest(bufio.NewReader(bytes.NewReader(rawRequest)))
	if err != nil {
		return fmt.Errorf("ReadRequest: %v", err)
	}
	e.Req = req
	return nil
}

// Response is a regular http.Response with the ability to
// receive the body a a byte slice via ReadBody.
type Response struct {
	*http.Response
}

// RawBody returns the response body as a byte slice leaving
// the original Body as an unread io.NopCloser over the same
// bytes.
func (r *Response) RawBody() ([]byte, error) {
	return readWithoutClose(&r.Body)
}

// Raw returns an approximation of the full response as byte
// slice.
func (r *Response) Raw() ([]byte, error) {
	// make sure that the body is a NopCloser
	_, err := readWithoutClose(&r.Body)
	if err != nil {
		return nil, fmt.Errorf("readWithoutClose: %v", err)
	}
	dump, err := httputil.DumpResponse(r.Response, true)
	if err != nil {
		return nil, fmt.Errorf("writing response: %v", err)
	}
	return dump, nil
}

// SetBody sets the Body of the response to a NopCloser over
// the given bytes.
func (r *Response) SetBody(body []byte) {
	r.Body = ioutil.NopCloser(bytes.NewBuffer(body))
}

// Set replaces the response a new Response parsed from the
// provided byte slice
func (r *Response) Set(rawResponse []byte) error {
	responseReader := bufio.NewReader(bytes.NewReader(rawResponse))
	res, err := http.ReadResponse(responseReader, r.Request)
	if err != nil {
		return err
	}
	*r = Response{Response: res}
	return nil
}

func (e *Event) prepareRequest() error {
	url := e.Req.URL
	if e.ForceHost != "" {
		url.Scheme = e.ForceScheme
		url.Host = e.ForceHost
	}

	// try to find out if the body is non-nil but won't yield any data
	var body = e.Req.Body
	if e.Req.Body != nil {
		rd := bufio.NewReader(e.Req.Body)
		buf, err := rd.Peek(1)
		if err == io.EOF || len(buf) == 0 {
			// if the body is non-nil but nothing can be read from it we set the body to http.NoBody
			// this happens for incoming http2 connections
			body = http.NoBody
		} else {
			body = bufferedReadCloser{
				Reader: rd,
				Closer: e.Req.Body,
			}
		}
	}

	req, err := http.NewRequestWithContext(e.Req.Context(), e.Req.Method, url.String(), body)
	if err != nil {
		return err
	}

	// use Host header from received request
	req.Host = e.Req.Host

	for name, values := range e.Req.Header {
		if _, ok := filterHeaders[strings.ToLower(name)]; ok {
			// header is filtered, do not send it to the upstream server
			continue
		}

		if newname, ok := renameHeaders[strings.ToLower(name)]; ok {
			name = newname
		}
		req.Header[name] = values
	}

	req.ContentLength = e.Req.ContentLength

	e.Req = req

	return nil
}

// Log logs a message through the embedded logger, prefixed with information
// about the request that spawned the Event
func (e *Event) Log(msg string, args ...interface{}) {
	args = append([]interface{}{e.ID, e.Req.RemoteAddr}, args...)
	e.Logger.Printf("[%4d %v] "+msg, args...)
}

// SendError responds with an error (which is also logged).
func (e *Event) SendError(msg string, args ...interface{}) {
	e.Log(msg, args...)
	e.ResponseWriter.Header().Set("Content-Type", "text/plain")
	e.ResponseWriter.WriteHeader(http.StatusInternalServerError)
	fmt.Fprintf(e.ResponseWriter, msg, args...)
}
