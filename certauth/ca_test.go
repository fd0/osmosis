package certauth

import (
	"flag"
	"path/filepath"
	"testing"
)

var updateGoldenFiles bool

func init() {
	flag.BoolVar(&updateGoldenFiles, "update", false, "update golden files in testdata/")
}

func TestNew(t *testing.T) {
	var testCACert = filepath.Join("testdata", "test_ca_cert.pem")
	var testCAKey = filepath.Join("testdata", "test_ca_key.pem")

	ca, err := NewCA()
	if err != nil {
		t.Fatal(err)
	}

	if updateGoldenFiles {
		t.Logf("updating test CA in testdata/")

		err := ca.Save(testCACert, testCAKey)
		if err != nil {
			t.Fatal(err)
		}
	}
}

func BenchmarkNew(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_, err := NewCA()
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkNewCertificate(b *testing.B) {
	ca := TestCA(b)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, err := ca.NewCertificate("foo", []string{"foo"})
		if err != nil {
			b.Fatal(err)
		}
	}
}
