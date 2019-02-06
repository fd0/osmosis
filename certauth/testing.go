package certauth

import "testing"

// TestCA returns a CA for use in testing.
func TestCA(t testing.TB) *CertificateAuthority {
	ca, err := NewCA()
	if err != nil {
		t.Fatal(err)
	}
	return ca
}
