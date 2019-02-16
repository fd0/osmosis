package certauth

import (
	"crypto/x509"
	"sync"
	"time"
)

// cacheEntry bundles a certificate and a timestamp.
type cacheEntry struct {
	T time.Time
	C *x509.Certificate
}

// Cache contains a list of certificates.
type Cache struct {
	certs           map[string]cacheEntry
	lastCleanup     time.Time
	cleanupInterval time.Duration
	cacheDuration   time.Duration
	m               sync.Mutex
}

// NewCache initializes a new cache.
func NewCache(cleanupInterval, cacheDuration time.Duration) *Cache {
	return &Cache{
		certs:           make(map[string]cacheEntry),
		cleanupInterval: cleanupInterval,
		cacheDuration:   cacheDuration,
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

// GetOrCreate returns a certificate from the cache, or calls f to create a
// certificate. The cache is locked while f runs.
func (c *Cache) GetOrCreate(name string, f func() (*x509.Certificate, error)) (*x509.Certificate, error) {
	c.m.Lock()
	defer c.m.Unlock()

	// do cleanup now
	if time.Since(c.lastCleanup) > c.cleanupInterval {
		c.cleanup()
	}

	entry, ok := c.certs[name]
	if ok {
		return entry.C, nil
	}

	// create new cert using f
	cert, err := f()
	if err != nil {
		return nil, err
	}

	// cache it
	c.certs[name] = cacheEntry{
		C: cert,
		T: time.Now(),
	}

	return cert, nil
}
