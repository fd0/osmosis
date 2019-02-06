package main

import (
	"crypto/tls"
	"fmt"
	"net/http"
	"net/http/httputil"
	"os"
	"time"

	"github.com/gorilla/websocket"
)

func dumpResponse(res *http.Response) {
	buf, err := httputil.DumpResponse(res, true)
	if err == nil {
		fmt.Printf("--------------\n%s\n--------------\n", buf)
	}
}

func dumpRequest(req *http.Request) {
	buf, err := httputil.DumpRequest(req, true)
	if err == nil {
		fmt.Printf("--------------\n%s\n--------------\n", buf)
	}
}

func main() {
	url := "ws://echo.websocket.org"
	if len(os.Args) >= 2 {
		url = os.Args[1]
	}
	fmt.Printf("connecting to %v\n", url)

	var dialer = *websocket.DefaultDialer
	dialer.TLSClientConfig = &tls.Config{
		InsecureSkipVerify: true,
	}

	c, res, err := dialer.Dial(url, nil)
	if err != nil {
		panic(err)
	}

	dumpResponse(res)

	go func() {
		for i := 0; i < 10; i++ {
			<-time.After(time.Second)
			err = c.WriteMessage(websocket.TextMessage, []byte("foobar: "+time.Now().String()))
			if err != nil {
				panic(err)
			}
		}

		err = c.Close()
		if err != nil {
			panic(err)
		}
	}()

	for {
		t, buf, err := c.ReadMessage()
		if err != nil {
			fmt.Fprintf(os.Stderr, "error ReadMessage: %v\n", err)
			break
		}

		fmt.Printf("received new message, type %v: %q\n", t, buf)
	}
}
