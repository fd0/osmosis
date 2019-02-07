package proxy

import (
	"fmt"
	"net/http"
	"net/http/httputil"
)

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
