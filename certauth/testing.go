package certauth

import (
	"testing"
)

const testCAKey = `
-----BEGIN RSA PRIVATE KEY-----
MIIEogIBAAKCAQEAvV2ShGwSyBQT9dvhflRBzkZIkXW1klIc9kRcHHPfd+M5o5zb
ORf+oRvaNHDfVApz7Rw94kKJZHU9ifbtsYhb6yye1ZE1Kx1Ag3Wq2MjsQ/6/VCOi
fF6wXJhkN1+jw0vYl6tHZiCzZtHw5EbrQyPIF/nt+lUQ4raiHdu1gKWcDwPATjtg
vd4EAkt4yXwlvAf51gOGDLffIyrFMPjbbFHwZ+l9zFman4w46vT7MEG/iA+HduUn
UeindNJPK1HQPJ3VWrr8fi5d4uUGvXc1NTkI540HFgGXk+kTP9UFWkE1ON9BJClC
dy+wVxiEM/lCHWTjEQ1bm1ppiJOkerXPv4M3iwIDAQABAoIBABCkqLv6w6vSF+8D
5E22GhKHGtmt/sddcC400+OsS3e+ANLgdmQON9LxH7x8LySzxnyQft/j+S9bHo2B
pmJ0GaARy+P5XzLq30CultV2000mlqpOd3l22QlKW4SCY2JGyEKMSzoupZXj3cgy
c3rzKiLxVHksAM/sPVNifBFSfOTUyNzbi1EnhontjH+HpFDPuE3GiIVjhwfsUS0y
aT2Rs9IS2tGPl+DKDezHuoT+YYm4p7LeCHTcFaAlfWf75P0HmAv0rFGgmLBeRUhz
LePMsLNAth4BzcthQXTp+vkZ1ePQQ600HDiAGuackbVu9G15wE3QHbZ31SDASy7N
TQ8MRAECgYEA2KJGIrtcpJpAw9LZBja7l46SfaEp/htdEllkG4Jb3P2JiB56gOdU
rholdsQ6br+ZCKjWdhpLdMUajEhFjwi6duwgWnJKSI670w+7++mlYMhBMsYP5vMu
X8gjAlBUKUUR98ikLBKR187CzfwwKqzmihJ91Uf5fAa/FR+H/GLQjccCgYEA38bI
yj7Szvx7fOpdQ2liu/2JpFWvddrazicQ6HK4JbyRKhrrex/0CzkXueanvIMINcJs
xEnmsAJsfgV6nIoVodRwT0cM3ofDQwY+f2KSvuFd5tuuUizcQ4yFN4YoHR+eBTQQ
C+CDYjCJQMuqyWA4C9uykWktNl6+sj2leeyjmB0CgYASrfNkPUmgu9hHyl+CRKfq
SpXhFUt8qLlewqx6HsRzCr2YKiCgCtJnbMO8OPFc6VJ1x7EuX9gPyosee4Db84G4
jWXAxsgW94/EhD/OWfgznzDYAvIOFPvzsFsscObA5D7HYdqeHj/LHv33Kv6wP1Zl
o3CMOneNtTs2xBBCt/aJswKBgBIX7ZZEvCDWU1nHTWEs/Tm8B0wNTZGW74gpqnlR
BUiv1YD1CkM7Uy0xIZT7bGaWpaxLGyZH32ot1/3cjYxosdUS6z3NveGkUopxz83W
94yNhl0rOA4W6HxhuUfDBi1MqCc9jWqYbacby408qoN7zyxOSELvoSM7R+n7iAyy
sIuVAoGAERMHTrG+xvkSM1OJ1RVPpK+6hGlkE0ATifovallZVwbe5QJ0aVcVRQN4
LLbu1w3V/P/D7I+7PqhLdSFda6GXWVSkwUOpENO/U5raY7xuGZYdkL604+bievXI
RLwyhGTrg4V+ptDTKnEoh6nJairyBY6cBFu52puAZjSR+1ttdiU=
-----END RSA PRIVATE KEY-----
`

const testCACert = `
-----BEGIN CERTIFICATE-----
MIIDCjCCAfKgAwIBAgIIFYPPBw4C3H0wDQYJKoZIhvcNAQELBQAwKDEmMCQGA1UE
ChMdT3Ntb3NpcyBJbnRlcmNlcHRpb24gUHJveHkgQ0EwHhcNMTkwMjE2MDk0NTI1
WhcNMjkwMjEzMDk0NTI1WjAoMSYwJAYDVQQKEx1Pc21vc2lzIEludGVyY2VwdGlv
biBQcm94eSBDQTCCASIwDQYJKoZIhvcNAQEBBQADggEPADCCAQoCggEBAL1dkoRs
EsgUE/Xb4X5UQc5GSJF1tZJSHPZEXBxz33fjOaOc2zkX/qEb2jRw31QKc+0cPeJC
iWR1PYn27bGIW+ssntWRNSsdQIN1qtjI7EP+v1QjonxesFyYZDdfo8NL2JerR2Yg
s2bR8ORG60MjyBf57fpVEOK2oh3btYClnA8DwE47YL3eBAJLeMl8JbwH+dYDhgy3
3yMqxTD422xR8GfpfcxZmp+MOOr0+zBBv4gPh3blJ1Hop3TSTytR0Dyd1Vq6/H4u
XeLlBr13NTU5COeNBxYBl5PpEz/VBVpBNTjfQSQpQncvsFcYhDP5Qh1k4xENW5ta
aYiTpHq1z7+DN4sCAwEAAaM4MDYwDgYDVR0PAQH/BAQDAgKkMBMGA1UdJQQMMAoG
CCsGAQUFBwMBMA8GA1UdEwEB/wQFMAMBAf8wDQYJKoZIhvcNAQELBQADggEBAIG0
WyzmkZBYTr57Ep891ZcryXswEQzallvuWyPQxCu3zfrijlO3bH93VWAmjqSjKYKy
okL0vPnhpCkDeOniGYICazqI4RC/6OZEQ8pcHBlDMPeeA6OukmF6HPE9ASjqLys/
fzvrPL4V/Orcz/nOkwqVu6x/CMuNGrB3CCsBcZy6aqRV1Nw+zLUrJwIisdL2tXaL
uboxXdDlxkB6XfzzcSm/p5qwxWJPBc6yqGKmclQeMmNCWUdlVhYOiVKpcz8Xmd39
I/6O9yIizxtTQtm6E+Gkk0odiHJ7ltsMi8mnrtiTNivR/dPpmbvTkaHr7m8Mscpa
p/RvrXheARy9//eADcw=
-----END CERTIFICATE-----
`

// TestCA returns a CA for use in testing read from testdata/
func TestCA(t testing.TB) *CertificateAuthority {
	cert, err := parseCertificate([]byte(testCACert))
	if err != nil {
		t.Fatal(err)
	}

	key, err := parsePrivateKey([]byte(testCAKey))
	if err != nil {
		t.Fatal(err)
	}

	return &CertificateAuthority{
		Certificate: cert,
		Key:         key,
	}
}

// TestNewCA returns a newly generated CA for use in testing.
func TestNewCA(t testing.TB) *CertificateAuthority {
	ca, err := NewCA()
	if err != nil {
		t.Fatal(err)
	}
	return ca
}
