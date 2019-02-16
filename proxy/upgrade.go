package proxy

import (
	"crypto/tls"
	"net/http"
	"net/url"
	"strings"

	"github.com/gorilla/websocket"

	"golang.org/x/sync/errgroup"
)

func copyWSMessages(src, dst *websocket.Conn) error {
	for {
		msgType, buf, err := src.ReadMessage()
		if websocket.IsCloseError(err, websocket.CloseNormalClosure) {
			return nil
		}
		if err != nil {
			return err
		}

		err = dst.WriteMessage(msgType, buf)
		if err != nil {
			return err
		}
	}
}

func copyWSUntilError(c1, c2 *websocket.Conn) error {
	var g errgroup.Group
	g.Go(func() error {
		defer c2.Close()
		return copyWSMessages(c1, c2)
	})
	g.Go(func() error {
		defer c1.Close()
		return copyWSMessages(c2, c1)
	})

	return g.Wait()
}

// filterWSHeaders contains headers which should not be used for establishing
// an outgoing websocket connection.
var filterWSHeaders = map[string]struct{}{
	"connection":               struct{}{},
	"upgrade":                  struct{}{},
	"sec-websocket-key":        struct{}{},
	"sec-websocket-version":    struct{}{},
	"sec-websocket-protocol":   struct{}{},
	"sec-websocket-extensions": struct{}{},
}

// prepareWSHeader copies all values from src to a new http.Header, except for
// the fields that are used to establish the websocket connection.
func prepareWSHeader(src http.Header) http.Header {
	hdr := make(http.Header, len(src))

	for name, values := range src {
		if _, ok := filterWSHeaders[strings.ToLower(name)]; ok {
			// header is filtered, do not send it to the upstream server
			continue
		}

		if newname, ok := renameHeaders[strings.ToLower(name)]; ok {
			name = newname
		}
		hdr[name] = values
	}

	return hdr
}

// HandleUpgradeRequest handles an upgraded connection (e.g. websockets).
func HandleUpgradeRequest(req *Request, clientConfig *tls.Config) {
	reqUpgrade := req.Request.Header.Get("upgrade")
	req.Log("handle upgrade request to %v", reqUpgrade)
	defer req.Log("done")

	dumpRequest(req.Request)

	// try to negotiate a websocket connection with the incoming request
	var upgrader = websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,

		// allow all origins, we are a proxy
		CheckOrigin: func(*http.Request) bool { return true },
	}

	inConn, err := upgrader.Upgrade(req.ResponseWriter, req.Request, nil)
	if err != nil {
		req.SendError("unable to negotiate a websocket upgrade: %v", err)
		req.Body.Close()
		return
	}
	defer inConn.Close()

	req.Log("negotiated websocket upgrade, establishing outgoing connection")

	wsURL := new(url.URL)
	// copy all values from the request URL
	*wsURL = *req.URL

	// apply forced host and scheme
	if req.ForceHost != "" {
		wsURL.Host = req.ForceHost
	}

	if req.ForceScheme != "" {
		wsURL.Scheme = req.ForceScheme
	}

	// set websocket scheme
	switch wsURL.Scheme {
	case "http":
		wsURL.Scheme = "ws"
	case "https":
		wsURL.Scheme = "wss"
	}

	hdr := prepareWSHeader(req.Request.Header)

	req.Log("connect to %v", wsURL)

	// remove the upgrade header field, it's re-added by the websocket library later
	hdr.Del("upgrade")

	var dialer = *websocket.DefaultDialer
	dialer.TLSClientConfig = clientConfig

	outConn, res, err := dialer.DialContext(req.Context(), wsURL.String(), hdr)
	if err != nil {
		req.Log("connecting to %v failed: %v", wsURL, err)
		dumpResponse(res)
		return
	}

	defer outConn.Close()

	req.Log("established outogoing connection to %v", wsURL)

	err = copyWSUntilError(inConn, outConn)
	if err != nil {
		req.Log("error copying messages: %v", err)
		return
	}
}
