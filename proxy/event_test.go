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
			t.Errorf("Method mismatch (got `%s`, want `%s`)", e.Req.Method, http.MethodPost)
		}
	})

	t.Run("GET", func(t *testing.T) {
		e := dummyEvent()

		err := e.SetRequest(mustReadFile("testdata/get_request"))
		if err != nil {
			t.Fatalf("SetRequest with GET request failed: %v", err)
		}
		if e.Req.Method != http.MethodGet {
			t.Errorf("Method mismatch (got `%s`, want `%s`)", e.Req.Method, http.MethodGet)
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
			t.Errorf("StatusCode mismatch (got `%d`, want `%d`)", res.StatusCode, http.StatusOK)
		}
	})

	t.Run("without body", func(t *testing.T) {
		res := Response{&http.Response{}}

		err := res.Set(mustReadFile("testdata/response_without_body"))
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
		err := e.SetRequest(mustReadFile("testdata/post_request"))
		if err != nil {
			t.Fatalf("setting up event: %v", err)
		}

		want := []byte("field1=value1&field2=value2")
		got, err := e.RawRequestBody()
		if err != nil {
			t.Fatalf("RawRequestBody failed: %v", err)
		}
		if !bytes.Equal(want, got) {
			t.Errorf("body mismatch (got `%s`, want `%s`)", got, want)
		}
	})
	t.Run("read multiple times", func(t *testing.T) {
		e := dummyEvent()
		err := e.SetRequest(mustReadFile("testdata/post_request"))
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
		err := e.SetRequest(mustReadFile("testdata/get_request"))
		if err != nil {
			t.Fatalf("setting up event: %v", err)
		}
		want := []byte("")
		got, err := e.RawRequestBody()
		if err != nil {
			t.Fatalf("RawRequestBody failed: %v", err)
		}
		if !bytes.Equal(want, got) {
			t.Errorf("body mismatch (got `%s`, want `%s`)", got, want)
		}
	})
}

func TestResponseRawBody(t *testing.T) {
	t.Run("read full body", func(t *testing.T) {
		res := Response{&http.Response{}}
		err := res.Set(mustReadFile("testdata/response_with_body"))
		if err != nil {
			t.Fatalf("setting up response: %v", err)
		}

		want := []byte("<html>\r\n<body>\r\n<h1>Hello, World!</h1>\r\n</body>\r\n</html>\r\n")
		got, err := res.RawBody()
		if err != nil {
			t.Fatalf("RawBody failed: %v", err)
		}
		if !bytes.Equal(want, got) {
			t.Errorf("body mismatch (got `%s`, want `%s`)", got, want)
		}
	})
	t.Run("read multiple times", func(t *testing.T) {
		res := Response{&http.Response{}}
		err := res.Set(mustReadFile("testdata/response_with_body"))
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
		err := res.Set(mustReadFile("testdata/response_without_body"))
		if err != nil {
			t.Fatalf("setting up response: %v", err)
		}

		want := []byte("")
		got, err := res.RawBody()
		if err != nil {
			t.Fatalf("RawBody failed: %v", err)
		}
		if !bytes.Equal(want, got) {
			t.Errorf("body mismatch (got `%s`, want `%s`)", got, want)
		}
	})
}

func TestSetRequestBody(t *testing.T) {
	t.Run("adding body to GET request", func(t *testing.T) {
		e := dummyEvent()
		err := e.SetRequest(mustReadFile("testdata/get_request"))
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
		err := e.SetRequest(mustReadFile("testdata/post_request"))
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
