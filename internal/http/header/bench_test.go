package header

import "testing"

// Realistic request: six fields, the shape a browser or curl sends.
func benchHeader() []byte {
	return []byte("GET /path HTTP/1.1\r\n" +
		"Host: test.example.com\r\n" +
		"User-Agent: bench/1.0\r\n" +
		"Accept: */*\r\n" +
		"Accept-Encoding: gzip, deflate\r\n" +
		"X-Forwarded-For: 1.2.3.4\r\n" +
		"Connection: keep-alive\r\n" +
		"\r\n")
}

func BenchmarkNewRequest(b *testing.B) {
	in := benchHeader()
	b.ReportAllocs()
	b.SetBytes(int64(len(in)))
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		if _, err := NewRequest(in); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkFinalize(b *testing.B) {
	h, err := NewRequest(benchHeader())
	if err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = h.Finalize()
	}
}
