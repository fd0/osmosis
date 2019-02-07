package proxy

import "net/http"

// ServeStatic returns the PEM encoded CA certificate.
func ServeStatic(rw http.ResponseWriter, req *http.Request, cert []byte) {
	switch req.URL.Path {
	case "/ca":
		rw.Header().Set("Content-Type", "application/x-x509-ca-cert")
		rw.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
		rw.Header().Set("Pragma", "no-cache")
		rw.Header().Set("Expires", "0")
		rw.WriteHeader(http.StatusOK)
		rw.Write(cert)
	default:
		http.Error(rw, "not found", http.StatusNotFound)
	}
}
