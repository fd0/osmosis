package proxy

import (
	"bufio"
	"crypto/tls"
	"io"
	"net"
	"net/http"

	"golang.org/x/sync/errgroup"
)

func copyUntilError(src, dst io.ReadWriteCloser) error {
	var g errgroup.Group
	g.Go(func() error {
		_, err := io.Copy(src, dst)
		src.Close()
		dst.Close()
		return err
	})

	g.Go(func() error {
		_, err := io.Copy(dst, src)
		src.Close()
		dst.Close()
		return err
	})

	return g.Wait()
}

// HandleUpgradeRequest handles an upgraded connection (e.g. websockets).
func HandleUpgradeRequest(req *Request) {
	reqUpgrade := req.Request.Header.Get("upgrade")
	req.Log("handle upgrade request to %v", reqUpgrade)

	host := req.URL.Host
	if req.ForceHost != "" {
		host = req.ForceHost
	}

	scheme := req.URL.Scheme
	if req.ForceHost != "" {
		scheme = req.ForceScheme
	}

	var outgoingConn net.Conn
	var err error

	if scheme == "https" {
		outgoingConn, err = tls.Dial("tcp", host, nil)
	} else {
		outgoingConn, err = net.Dial("tcp", host)
	}

	if err != nil {
		req.SendError("connecting to %v failed: %v", host, err)
		req.Body.Close()
		return
	}

	defer outgoingConn.Close()

	req.Log("connected to %v", host)

	outReq, err := prepareRequest(req.Request, host, scheme)
	if err != nil {
		req.SendError("preparing request to %v failed: %v", host, err)
		req.Body.Close()
		return
	}

	// put back the "Connection" header
	outReq.Header.Set("connection", req.Request.Header.Get("connection"))

	dumpRequest(req.Request)
	dumpRequest(outReq)

	err = outReq.Write(outgoingConn)
	if err != nil {
		req.SendError("unable to forward request to %v: %v", host, err)
		req.Body.Close()
		return
	}

	outgoingReader := bufio.NewReader(outgoingConn)
	outRes, err := http.ReadResponse(outgoingReader, outReq)
	if err != nil {
		req.SendError("unable to read response from %v: %v", host, err)
		req.Body.Close()
		return
	}

	dumpResponse(outRes)

	hj, ok := req.ResponseWriter.(http.Hijacker)
	if !ok {
		req.SendError("switching protocols failed, incoming connection is not bidirectional")
		req.Body.Close()
		return
	}

	clientConn, _, err := hj.Hijack()
	if !ok {
		req.SendError("switching protocols failed, hijacking incoming connection failed: %v", err)
		req.Body.Close()
		return
	}
	defer clientConn.Close()

	err = outRes.Write(clientConn)
	if err != nil {
		req.Log("writing response to client failed: %v", err)
		return
	}

	req.Log("start forwarding data")
	err = copyUntilError(outgoingConn, clientConn)
	if err != nil {
		req.Log("copying data for websocket returned error: %v", err)
	}
	req.Log("connection done")
}
