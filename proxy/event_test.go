package proxy

import (
	"bytes"
	"io/ioutil"
	"log"
	"net/http"
	"testing"
)

var getRequest = []byte(`GET /hello.htm HTTP/1.1
Host: www.example.com
User-Agent: monsoon
Accept-Language: en-us
Accept-Encoding: gzip, deflate
Connection: Keep-Alive

`)

var postRequestHeaders = []byte(`POST /test HTTP/1.1
Host: www.example.com
User-Agent: monsoon
Content-Type: application/x-www-form-urlencoded
Content-Length: 27

`)

var postRequestBody = []byte(`field1=value1&field2=value2`)

var postRequest = append(postRequestHeaders, postRequestBody...)

var responseWithBodyHeaders = []byte(`HTTP/1.1 200 OK
Date: Sat, 12 Oct 2019 10:39:53 GMT
Server: Caddy
Last-Modified: Sat, 12 Oct 2019 10:00:03 GMT
Content-Length: 53
Content-Type: text/html
Connection: Closed

`)

var responseWithBodyBody = []byte(`<html>
<body>
<h1>Hello, World!</h1>
</body>
</html>
`)

var responseWithBody = append(responseWithBodyHeaders, responseWithBodyBody...)

var responseWithoutBody = []byte(`HTTP/1.1 404 Not Found
Date: Sun, 18 Oct 2012 10:36:20 GMT
Server: Apache/2.2.14 (Win32)
Content-Length: 0
Connection: Closed
Content-Type: text/html; charset=iso-8859-1

`)

func TestSetRequest(t *testing.T) {
	t.Run("POST", func(t *testing.T) {
		e := dummyEvent()

		err := e.SetRequest(postRequest)
		if err != nil {
			t.Fatalf("SetRequest with POST request failed: %v", err)
		}
		if e.Req.Method != http.MethodPost {
			t.Errorf("Method mismatch (got `%s`, want `%s`)", e.Req.Method, http.MethodPost)
		}
		if e.Req.RequestURI != "" {
			t.Errorf("client request cannot have RequestURI set")
		}
	})

	t.Run("GET", func(t *testing.T) {
		e := dummyEvent()

		err := e.SetRequest(getRequest)
		if err != nil {
			t.Fatalf("SetRequest with GET request failed: %v", err)
		}
		if e.Req.Method != http.MethodGet {
			t.Errorf("Method mismatch (got `%s`, want `%s`)", e.Req.Method, http.MethodGet)
		}
		if e.Req.RequestURI != "" {
			t.Errorf("client request cannot have RequestURI set")
		}
	})
}

func TestResponseSet(t *testing.T) {
	t.Run("with body", func(t *testing.T) {
		res := Response{&http.Response{}}

		err := res.Set(responseWithBody)
		if err != nil {
			t.Fatalf("setting response with body failed: %v", err)
		}
		if res.StatusCode != http.StatusOK {
			t.Errorf("StatusCode mismatch (got `%d`, want `%d`)", res.StatusCode, http.StatusOK)
		}
	})

	t.Run("without body", func(t *testing.T) {
		res := Response{&http.Response{}}

		err := res.Set(responseWithoutBody)
		if err != nil {
			t.Fatalf("setting response without body failed: %v", err)
		}
		if res.StatusCode != http.StatusNotFound {
			t.Errorf("StatusCode mismatch (got `%d`, want `%d`)", res.StatusCode, http.StatusNotFound)
		}
	})
}

func TestRawRequestBody(t *testing.T) {
	t.Run("read post data", func(t *testing.T) {
		e := dummyEvent()
		err := e.SetRequest(postRequest)
		if err != nil {
			t.Fatalf("setting up event: %v", err)
		}

		got, err := e.RawRequestBody()
		if err != nil {
			t.Fatalf("RawRequestBody failed: %v", err)
		}
		if !bytes.Equal(postRequestBody, got) {
			t.Errorf("body mismatch (got `%s`, want `%s`)", got, postRequestBody)
		}
	})
	t.Run("read multiple times", func(t *testing.T) {
		e := dummyEvent()
		err := e.SetRequest(postRequest)
		if err != nil {
			t.Fatalf("setting up event: %v", err)
		}
		want, err := e.RawRequestBody()
		if err != nil {
			t.Fatalf("RawRequestBody failed the first time: %v", err)
		}
		got, err := e.RawRequestBody()
		if err != nil {
			t.Fatalf("RawRequestBody failed the second time: %v", err)
		}
		if !bytes.Equal(want, got) {
			t.Errorf("body mismatch (got `%s`, want `%s`)", got, want)
		}
	})
	t.Run("read empty body", func(t *testing.T) {
		e := dummyEvent()
		err := e.SetRequest(getRequest)
		if err != nil {
			t.Fatalf("setting up event: %v", err)
		}

		got, err := e.RawRequestBody()
		if err != nil {
			t.Fatalf("RawRequestBody failed: %v", err)
		}
		if !bytes.Equal([]byte{}, got) {
			t.Errorf("body mismatch (got `%s`, want empty body)", got)
		}
	})
}

func TestRawRequest(t *testing.T) {
	e := dummyEvent()
	err := e.SetRequest(postRequest)
	if err != nil {
		t.Fatalf("setting up event: %v", err)
	}

	dump, err := e.RawRequest()
	if err != nil {
		t.Fatalf("dumping raw event: %v", err)
	}

	if !bytes.Contains(dump, postRequestBody) {
		t.Errorf("body `%s` was not found in request: %s", postRequestHeaders, dump)
	}

	wantHeaders := bytes.Split(postRequestHeaders, []byte("\n"))

	for _, wantHeader := range wantHeaders {
		if !bytes.Contains(dump, wantHeader) {
			t.Errorf("header `%s` was not found in request: %s", wantHeader, dump)
		}
	}

}

func TestResponseRawBody(t *testing.T) {
	t.Run("read full body", func(t *testing.T) {
		res := Response{&http.Response{}}
		err := res.Set(responseWithBody)
		if err != nil {
			t.Fatalf("setting up response: %v", err)
		}

		got, err := res.RawBody()
		if err != nil {
			t.Fatalf("RawBody failed: %v", err)
		}
		if !bytes.Equal(responseWithBodyBody, got) {
			t.Errorf("body mismatch (got `%s`, want `%s`)", got, responseWithBodyBody)
		}
	})
	t.Run("read multiple times", func(t *testing.T) {
		res := Response{&http.Response{}}
		err := res.Set(responseWithBody)
		if err != nil {
			t.Fatalf("setting up response: %v", err)
		}

		want, err := res.RawBody()
		if err != nil {
			t.Fatalf("RawBody failed the first time: %v", err)
		}
		got, err := res.RawBody()
		if err != nil {
			t.Fatalf("RawBody failed the second time: %v", err)
		}
		if !bytes.Equal(want, got) {
			t.Errorf("body mismatch (got `%s`, want `%s`)", got, want)
		}
	})
	t.Run("read empty body", func(t *testing.T) {
		res := Response{&http.Response{}}
		err := res.Set(responseWithoutBody)
		if err != nil {
			t.Fatalf("setting up response: %v", err)
		}

		got, err := res.RawBody()
		if err != nil {
			t.Fatalf("RawBody failed: %v", err)
		}
		if !bytes.Equal([]byte{}, got) {
			t.Errorf("body mismatch (got `%s`, want empty body)", got)
		}
	})
}

func TestSetRequestBody(t *testing.T) {
	t.Run("adding body to GET request", func(t *testing.T) {
		e := dummyEvent()
		err := e.SetRequest(getRequest)
		if err != nil {
			t.Fatalf("setting up event: %v", err)
		}
		want := []byte("new GET request body")
		e.SetRequestBody(want)
		got, err := e.RawRequestBody()
		if err != nil {
			t.Fatalf("retrieving new GET request body: %v", err)
		}
		if !bytes.Equal(got, want) {
			t.Fatalf("body mismatch (got `%s`, want `%s`)", got, want)
		}
	})
	t.Run("replacing body in POST request", func(t *testing.T) {
		e := dummyEvent()
		err := e.SetRequest(postRequest)
		if err != nil {
			t.Fatalf("setting up event: %v", err)
		}
		want := []byte("replaced POST request body")
		e.SetRequestBody(want)
		got, err := e.RawRequestBody()
		if err != nil {
			t.Fatalf("retrieving new POST request body: %v", err)
		}
		if !bytes.Equal(got, want) {
			t.Fatalf("body mismatch (got `%s`, want `%s`)", got, want)
		}
	})
}

func TestResponseRaw(t *testing.T) {
	r := &Response{&http.Response{}}
	err := r.Set(responseWithBody)
	if err != nil {
		t.Fatalf("setting up response: %v", err)
	}

	dump, err := r.Raw()
	if err != nil {
		t.Fatalf("dumping raw event: %v", err)
	}

	if !bytes.Contains(dump, responseWithBodyBody) {
		t.Errorf("body `%s` was not found in request: %s", responseWithBodyBody, dump)
	}

	wantHeaders := bytes.Split(responseWithBodyHeaders, []byte("\n"))

	for _, wantHeader := range wantHeaders {
		if !bytes.Contains(dump, wantHeader) {
			t.Errorf("header `%s` was not found in request: %s", wantHeader, dump)
		}
	}

}

func TestForwardRequestDefaultError(t *testing.T) {
	e := dummyEvent()
	_, err := e.ForwardRequest()
	if err != ErrNoForwardAction {
		t.Errorf("received error `%v` instead of ErrNoForwardAction", err)
	}
}

func mustReadFile(fileName string) []byte {
	content, err := ioutil.ReadFile(fileName)
	if err != nil {
		panic(err)
	}
	return content
}

func dummyEvent() *Event {
	return newEvent(dummyResponseWriter{}, &http.Request{}, dummyLogger, 0)
}

var dummyLogger = log.New(ioutil.Discard, "", 0)

type dummyResponseWriter struct{}

func (d dummyResponseWriter) Header() http.Header {
	return http.Header{}
}

func (d dummyResponseWriter) Write(input []byte) (int, error) {
	return len(input), nil
}

func (d dummyResponseWriter) WriteHeader(statusCode int) {}
