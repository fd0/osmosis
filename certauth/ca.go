package certauth

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"io/ioutil"
	"math/big"
	"net"
	"time"
)

// CertificateAuthority manages a certificate authority which allows creating
// new certificates and signing them.
type CertificateAuthority struct {
	Key         *rsa.PrivateKey
	Certificate *x509.Certificate
}

// NewCA creates a new certificate authority.
func NewCA() (*CertificateAuthority, error) {
	// adapter from https://golang.org/src/crypto/tls/generate_cert.go
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, err
	}

	template := &x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()),
		Subject: pkix.Name{
			Organization: []string{"Osmosis Interception Proxy CA"},
		},
		NotBefore: time.Now(),
		NotAfter:  time.Now().Add(3650 * 24 * time.Hour), // 10 years

		IsCA:                  true,
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}

	// create self-signed certificate
	derCert, err := x509.CreateCertificate(rand.Reader, template, template, key.Public(), key)
	if err != nil {
		return nil, err
	}

	cert, err := x509.ParseCertificate(derCert)
	if err != nil {
		return nil, err
	}

	ca := &CertificateAuthority{
		Key:         key,
		Certificate: cert,
	}

	return ca, nil
}

// Load loads a certificate authority from files.
func Load(certfile, keyfile string) (*CertificateAuthority, error) {
	key, err := LoadPrivateKey(keyfile)
	if err != nil {
		return nil, err
	}

	cert, err := LoadCertificate(certfile)
	if err != nil {
		return nil, err
	}

	ca := &CertificateAuthority{
		Key:         key,
		Certificate: cert,
	}

	return ca, nil
}

// WriteCertificate creates filename and writes the certificate c to it,
// encoded in PEM.
func WriteCertificate(filename string, c *x509.Certificate) error {
	crt := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: c.Raw})
	return ioutil.WriteFile(filename, crt, 0644)
}

// LoadCertificate loads a PEM-encoded certificate from filename.
func LoadCertificate(filename string) (*x509.Certificate, error) {
	buf, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	return parseCertificate(buf)
}

func parseCertificate(buf []byte) (*x509.Certificate, error) {
	block, _ := pem.Decode(buf)
	if block.Type != "CERTIFICATE" {
		return nil, fmt.Errorf("key not found: wanted type %q, got %q",
			"CERTIFICATE", block.Type)
	}

	return x509.ParseCertificate(block.Bytes)
}

// WritePrivateKey creates filename and writes the private key p to it, encoded
// in PEM.
func WritePrivateKey(filename string, k *rsa.PrivateKey) error {
	key := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(k)})
	return ioutil.WriteFile(filename, key, 0600)
}

// LoadPrivateKey loads a PEM-encoded private key from filename.
func LoadPrivateKey(filename string) (*rsa.PrivateKey, error) {
	buf, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	return parsePrivateKey(buf)
}

func parsePrivateKey(buf []byte) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode(buf)
	if block.Type != "RSA PRIVATE KEY" {
		return nil, fmt.Errorf("key not found: wanted type %q, got %q",
			"RSA PRIVATE KEY", block.Type)
	}

	key, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		return nil, err
	}

	return key, nil
}

// Save saves a certificate authority to files.
func (ca *CertificateAuthority) Save(certfile, keyfile string) error {
	err := WriteCertificate(certfile, ca.Certificate)
	if err != nil {
		return err
	}

	err = WritePrivateKey(keyfile, ca.Key)
	if err != nil {
		return err
	}

	return nil
}

// CertificateAsPEM returns the CA certificate encoded as PEM.
func (ca *CertificateAuthority) CertificateAsPEM() []byte {
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: ca.Certificate.Raw})
}

// NewCertificate creates a new certificate for the given host name or IP address.
func (ca *CertificateAuthority) NewCertificate(commonName string, names []string) (*x509.Certificate, error) {
	// generate random 64 bit serial
	serial := make([]byte, 8)
	_, err := rand.Read(serial)
	if err != nil {
		panic(err)
	}

	template := &x509.Certificate{
		SerialNumber: big.NewInt(0).SetBytes(serial),
		Subject: pkix.Name{
			CommonName: commonName,
		},
		NotBefore: time.Now(),
		NotAfter:  time.Now().Add(3650 * 24 * time.Hour), // 10 years

		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}

	for _, name := range names {
		// try to parse an IP address to find out if we should insert a DNS name or an IP address
		ip := net.ParseIP(name)
		if ip != nil {
			template.IPAddresses = append(template.IPAddresses, ip)
		} else {
			template.DNSNames = append(template.DNSNames, name)
		}
	}

	derCert, err := x509.CreateCertificate(rand.Reader, template, ca.Certificate, ca.Key.Public(), ca.Key)
	if err != nil {
		return nil, err
	}

	return x509.ParseCertificate(derCert)
}
