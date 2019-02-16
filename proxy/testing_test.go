package proxy

import "testing"

func BenchmarkTestProxy(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_, _, _ = TestProxy(b, nil)
	}
}
