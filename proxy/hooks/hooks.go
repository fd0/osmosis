package hooks

import (
	"fmt"

	"github.com/fd0/osmosis/proxy"
)

// RemoveCompression sets Accept-Encoding to identity such that the
// response is uncompressed and easily editable.
func RemoveCompression(event *proxy.Event) (*proxy.Response, error) {
	event.Req.Header.Set("Accept-Encoding", "identity")
	return event.ForwardRequest()
}

// LogCompleteRequest waits for the server response and then logs the
// status code, request method, URL and protocol.
func LogCompleteRequest(event *proxy.Event) (*proxy.Response, error) {
	res, err := event.ForwardRequest()
	if err != nil {
		return nil, err
	}
	event.Log("%v %v %v %v\n", res.StatusCode, event.Req.Method, event.Req.URL, event.Req.Proto)
	return res, err
}

// DumpToLog returns a hook that dumps the request and/or the response to the event's logger.
func DumpToLog(dumpRequest, dumpResponse bool) func(*proxy.Event) (*proxy.Response, error) {
	return func(event *proxy.Event) (*proxy.Response, error) {
		if dumpRequest {
			dump, err := event.RawRequest()
			if err != nil {
				return nil, fmt.Errorf("dumping request: %v", err)
			}
			event.Log("Request dump:\n%s", dump)
		}

		res, err := event.ForwardRequest()
		if err != nil {
			return nil, err
		}

		if dumpResponse {
			dump, err := res.Raw()
			if err != nil {
				return nil, fmt.Errorf("dumping response: %v", err)
			}
			event.Log("Response dump:\n%s", dump)
		}
		return res, nil
	}
}
