package proxy

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"log"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/happal/osmosis/certauth"
)

// cacheEntry bundles a certificate and a timestamp.
type cacheEntry struct {
	T time.Time
	C *x509.Certificate
}

// cacheKey bundles a target address with a server name (sent in SNI).
type cacheKey struct {
	Addr, ServerName string
}

// Cache contains a list of certificates.
type Cache struct {
	certs           map[cacheKey]cacheEntry
	lastCleanup     time.Time
	cleanupInterval time.Duration
	cacheDuration   time.Duration
	m               sync.Mutex

	ca           *certauth.CertificateAuthority
	clientConfig *tls.Config
	log          *log.Logger
}

const (
	cleanupInterval = 30 * time.Second
	cacheDuration   = 10 * time.Minute
)

// NewCache returns a new Cache.
func NewCache(ca *certauth.CertificateAuthority, clientConfig *tls.Config, log *log.Logger) *Cache {
	return &Cache{
		certs:           make(map[cacheKey]cacheEntry),
		cleanupInterval: cleanupInterval,
		cacheDuration:   cacheDuration,

		ca:           ca,
		clientConfig: clientConfig,
		log:          log,
	}
}

// cleanup removes old certificates.
func (c *Cache) cleanup() {
	for name, entry := range c.certs {
		if time.Since(entry.T) > c.cacheDuration {
			delete(c.certs, name)
		}
	}
}

// getOrCreate returns a certificate from the cache, or calls f to create a
// certificate. The cache is locked while f runs.
func (c *Cache) getOrCreate(addr, serverName string, f func() (*x509.Certificate, error)) (*x509.Certificate, error) {
	c.m.Lock()
	defer c.m.Unlock()

	// do cleanup now
	if time.Since(c.lastCleanup) > c.cleanupInterval {
		c.cleanup()
	}

	key := cacheKey{Addr: addr, ServerName: serverName}

	entry, ok := c.certs[key]
	if ok {
		return entry.C, nil
	}

	// create new cert using f
	cert, err := f()
	if err != nil {
		return nil, err
	}

	// cache it
	c.certs[key] = cacheEntry{
		C: cert,
		T: time.Now(),
	}

	return cert, nil
}

// getCertificate connects to the host, attempts a TLS handshake, and then
// disconnects. It returns the first leaf (=non-CA) certificate.
func getCertificate(ctx context.Context, target, serverName string, clientConfig *tls.Config) (*x509.Certificate, error) {
	// create new dialer so that we can use DialContext
	dialer := &net.Dialer{}

	// connect with timeout context
	conn, err := dialer.DialContext(ctx, "tcp", target)
	if err != nil {
		return nil, err
	}

	var cfg = &tls.Config{}
	if clientConfig != nil {
		// use settings from the tlsClient
		cfg = clientConfig.Clone()
	}

	cfg.ServerName = serverName

	// set server name to host name without port
	cfg.ServerName = strings.Split(target, ":")[0]

	// try a TLS client handshake
	client := tls.Client(conn, cfg)
	err = client.Handshake()
	if err != nil {
		_ = conn.Close()
		return nil, err
	}

	// close the TLS client (which also closes the underlying connection)
	err = client.Close()
	if err != nil {
		_ = conn.Close()
		return nil, err
	}

	for _, cert := range client.ConnectionState().PeerCertificates {
		if !cert.IsCA {
			return cert, nil
		}
	}

	return nil, errors.New("no certificate could be found")
}

// Get returns a certificate from the cache, which is generated on demand.
func (c *Cache) Get(ctx context.Context, addr, serverName string) (*tls.Certificate, error) {
	c.log.Printf("Get cert for %v", addr)
	name := strings.Split(addr, ":")[0]

	crt, err := c.getOrCreate(addr, serverName, func() (*x509.Certificate, error) {
		// try to get the host's cert and clone it
		cert, err := getCertificate(ctx, addr, serverName, c.clientConfig)
		if err == nil {
			clonedCert, err := c.ca.Clone(cert)
			if err == nil {
				return clonedCert, nil
			}
			c.log.Printf("error cloning cert for %v (%v): %v", addr, serverName, err)
		} else {
			c.log.Printf("error getting cert for %v (%v): %v", addr, serverName, err)
		}

		crt, err := c.ca.NewCertificate(name, []string{name})
		if err != nil {
			return nil, err
		}

		return crt, nil
	})

	if err != nil {
		return nil, err
	}

	return c.ca.TLSCert(crt), nil
}
