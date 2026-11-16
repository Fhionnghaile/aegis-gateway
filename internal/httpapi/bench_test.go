package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func BenchmarkIngestHTTP(b *testing.B) {
	h := newTestServer(1 << 20).Handler()
	body := `{"source":"edr","type":"login_fail","severity":"high"}`
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			r := httptest.NewRequest("POST", "/events", strings.NewReader(body))
			wr := httptest.NewRecorder()
			h.ServeHTTP(wr, r)
			if wr.Code != http.StatusAccepted {
				b.Fatalf("code %d", wr.Code)
			}
		}
	})
}
