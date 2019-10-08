package proxy

import (
	"bytes"
	"io/ioutil"
	"log"
	"net/http"
	"testing"
)

func TestSetRequest(t *testing.T) {
	t.Run("POST", func(t *testing.T) {
		e := dummyEvent()

		err := e.SetRequest(mustReadFile("testdata/post_request"))
		if err != nil {
			t.Fatalf("SetRequest with POST request failed: %v", err)
		}
		if e.Req.Method != http.MethodPost {
			t.Errorf("Method mismatch (has `%s`, want `%s`)", e.Req.Method, http.MethodPost)
		}
	})

	t.Run("GET", func(t *testing.T) {
		e := dummyEvent()

		err := e.SetRequest(mustReadFile("testdata/get_request"))
		if err != nil {
			t.Fatalf("SetRequest with GET request failed: %v", err)
		}
		if e.Req.Method != http.MethodGet {
			t.Errorf("Method mismatch (has `%s`, want `%s`)", e.Req.Method, http.MethodGet)
		}
	})
}

func TestResponseSet(t *testing.T) {
	t.Run("with body", func(t *testing.T) {
		res := Response{&http.Response{}}

		err := res.Set(mustReadFile("testdata/response_with_body"))
		if err != nil {
			t.Fatalf("setting response with body failed: %v", err)
		}
		if res.StatusCode != http.StatusOK {
			t.Errorf("StatusCode mismatch (has `%d`, want `%d`)", res.StatusCode, http.StatusOK)
		}
	})

	t.Run("without body", func(t *testing.T) {
		res := Response{&http.Response{}}

		err := res.Set(mustReadFile("testdata/response_without_body"))
		if err != nil {
			t.Fatalf("setting response without body failed: %v", err)
		}
		if res.StatusCode != http.StatusNotFound {
			t.Errorf("StatusCode mismatch (has `%d`, want `%d`)", res.StatusCode, http.StatusNotFound)
		}
	})
}

func TestRawRequestBody(t *testing.T) {
	t.Run("read post data", func(t *testing.T) {
		e := dummyEvent()
		e.SetRequest(mustReadFile("testdata/post_request"))

		want := []byte("field1=value1&field2=value2")
		has, err := e.RawRequestBody()
		if err != nil {
			t.Fatalf("RawRequestBody failed: %v", err)
		}
		if !bytes.Equal(want, has) {
			t.Errorf("body mismatch (has `%s`, want `%s`)", has, want)
		}
	})
	t.Run("read multiple times", func(t *testing.T) {
		e := dummyEvent()
		e.SetRequest(mustReadFile("testdata/post_request"))

		want, err := e.RawRequestBody()
		if err != nil {
			t.Fatalf("RawRequestBody failed the first time: %v", err)
		}
		has, err := e.RawRequestBody()
		if err != nil {
			t.Fatalf("RawRequestBody failed the second time: %v", err)
		}
		if !bytes.Equal(want, has) {
			t.Errorf("body mismatch (has `%s`, want `%s`)", has, want)
		}
	})
	t.Run("read empty body", func(t *testing.T) {
		e := dummyEvent()
		e.SetRequest(mustReadFile("testdata/get_request"))

		want := []byte("")
		has, err := e.RawRequestBody()
		if err != nil {
			t.Fatalf("RawRequestBody failed: %v", err)
		}
		if !bytes.Equal(want, has) {
			t.Errorf("body mismatch (has `%s`, want `%s`)", has, want)
		}
	})
}

func TestResponseRawBody(t *testing.T) {
	t.Run("read full body", func(t *testing.T) {
		res := &Response{&http.Response{}}
		res.Set(mustReadFile("testdata/response_with_body"))

		want := []byte("<html>\n<body>\n<h1>Hello, World!</h1>\n</body>\n</html>\n\n")
		has, err := res.RawBody()
		if err != nil {
			t.Fatalf("RawBody failed: %v", err)
		}
		if !bytes.Equal(want, has) {
			t.Errorf("body mismatch (has `%s`, want `%s`)", has, want)
		}
	})
	t.Run("read multiple times", func(t *testing.T) {
		res := &Response{&http.Response{}}
		res.Set(mustReadFile("testdata/response_with_body"))

		want, err := res.RawBody()
		if err != nil {
			t.Fatalf("RawBody failed the first time: %v", err)
		}
		has, err := res.RawBody()
		if err != nil {
			t.Fatalf("RawBody failed the second time: %v", err)
		}
		if !bytes.Equal(want, has) {
			t.Errorf("body mismatch (has `%s`, want `%s`)", has, want)
		}
	})
	t.Run("read empty body", func(t *testing.T) {
		res := &Response{&http.Response{}}
		res.Set(mustReadFile("testdata/response_without_body"))

		want := []byte("")
		has, err := res.RawBody()
		if err != nil {
			t.Fatalf("RawBody failed: %v", err)
		}
		if !bytes.Equal(want, has) {
			t.Errorf("body mismatch (has `%s`, want `%s`)", has, want)
		}
	})
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
