package respond_test

// End-to-end containment, measured (ML n20_10). Real HTTP on loopback between three parts:
// attacker -> gateway /events; detection -> responder -> gateway admin /admin/block.
// Run: go test ./internal/respond -run Containment -v   (the -v output carries the measurements)

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Fhionnghaile/aegis-gateway/internal/contain"
	"github.com/Fhionnghaile/aegis-gateway/internal/event"
	"github.com/Fhionnghaile/aegis-gateway/internal/httpapi"
	"github.com/Fhionnghaile/aegis-gateway/internal/ingest"
	"github.com/Fhionnghaile/aegis-gateway/internal/metrics"
	"github.com/Fhionnghaile/aegis-gateway/internal/respond"
)

type stack struct {
	pub, admin, resp *httptest.Server
	m                *metrics.Metrics
}

func newStack(t *testing.T, guardThreshold int) *stack {
	t.Helper()
	v, _ := event.NewSchemaValidator("")
	bl := contain.NewBlocklist()
	m := &metrics.Metrics{}
	g := contain.NewGuard(bl, guardThreshold, 10*time.Second, 15*time.Minute)
	s := httpapi.New(v, ingest.NewQueue(100000), m).WithContainment(bl, g)
	st := &stack{pub: httptest.NewServer(s.Handler()), admin: httptest.NewServer(s.AdminHandler()), m: m}
	st.resp = httptest.NewServer(respond.New(st.admin.URL).Handler())
	t.Cleanup(func() { st.pub.Close(); st.admin.Close(); st.resp.Close() })
	return st
}

func post(t *testing.T, url, body string) int {
	t.Helper()
	r, err := http.Post(url, "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	r.Body.Close()
	return r.StatusCode
}

// A detection from aegis-detector (its exact JSON) blocks the hostile sensor at the gateway.
func TestContainmentDetectorToGateway(t *testing.T) {
	st := newStack(t, 20)
	hostile := `{"source":"edr-7","type":"process_start"}`
	healthy := `{"source":"edr-1","type":"process_start"}`
	if post(t, st.pub.URL+"/events", hostile) != http.StatusAccepted {
		t.Fatal("before containment the sensor's events must be accepted")
	}

	det := `{"rule_id":"sensor-flood","title":"Sensor event flood","severity":"high","detected_at":"2026-11-28T19:00:00Z","group_key":"edr-7","count":1000}`
	t0 := time.Now()
	resp, err := http.Post(st.resp.URL+"/detections", "application/json", bytes.NewBufferString(det))
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	_, _ = out.ReadFrom(resp.Body)
	resp.Body.Close()
	confirmed := time.Since(t0)
	if !strings.Contains(out.String(), `"outcome":"blocked"`) || strings.Contains(out.String(), `"took_ms":0}`) {
		t.Fatalf("responder did not block: %s", out.String())
	}
	// First request after the responder answered: must already be refused.
	code := post(t, st.pub.URL+"/events", hostile)
	contained := time.Since(t0)
	if code != http.StatusForbidden {
		t.Fatalf("hostile sensor not contained: got %d", code)
	}
	leaked := 0
	for i := 0; i < 200; i++ {
		if post(t, st.pub.URL+"/events", hostile) != http.StatusForbidden {
			leaked++
		}
	}
	if leaked != 0 || post(t, st.pub.URL+"/events", healthy) != http.StatusAccepted {
		t.Fatalf("leaked=%d, or the healthy sensor was caught too", leaked)
	}
	if st.m.Refused.Load() != 201 {
		t.Fatalf("refused counter %d, want 201", st.m.Refused.Load())
	}
	t.Logf("MEASURED detector->gateway: block confirmed %.2f ms after the detection was sent; first hostile request refused at %.2f ms; hostile requests accepted after containment: %d of 201; healthy sensor unaffected",
		ms(confirmed), ms(contained), leaked)
}

// The gateway contains abuse of itself with no external detector: a client flooding invalid
// events is blocked on the Threshold-th one.
func TestContainmentGuardInvalidFlood(t *testing.T) {
	st := newStack(t, 20)
	bad := `{"source":"x"}` // missing required "type": fails validation
	var first time.Time
	sent := 0
	for i := 1; i <= 100; i++ {
		if i == 1 {
			first = time.Now()
		}
		code := post(t, st.pub.URL+"/events", bad)
		sent++
		if code == http.StatusForbidden {
			break
		}
		if code != http.StatusBadRequest {
			t.Fatalf("request %d: got %d", i, code)
		}
	}
	took := time.Since(first)
	if sent != 21 || st.m.Contained.Load() != 1 {
		t.Fatalf("want the 21st request refused after 20 invalid ones; sent=%d contained=%d", sent, st.m.Contained.Load())
	}
	// Even a valid event from the blocked client is now refused.
	if post(t, st.pub.URL+"/events", `{"source":"x","type":"login"}`) != http.StatusForbidden {
		t.Fatal("blocked client's valid event was accepted")
	}
	t.Logf("MEASURED guard: blocked after 20 invalid events; flood start -> first refusal %.2f ms; valid events from the blocked client also refused", ms(took))
}

// Low-severity or unmapped detections never block anything.
func TestResponderPolicyIgnoresWhatItShould(t *testing.T) {
	st := newStack(t, 20)
	for _, det := range []string{
		`{"rule_id":"sensor-flood","severity":"medium","group_key":"edr-7"}`,
		`{"rule_id":"auth-bruteforce","severity":"high","group_key":"alice"}`,
		`{"rule_id":"sensor-flood","severity":"critical","group_key":""}`,
	} {
		r, _ := http.Post(st.resp.URL+"/detections", "application/json", strings.NewReader(det))
		var b bytes.Buffer
		_, _ = b.ReadFrom(r.Body)
		r.Body.Close()
		if !strings.Contains(b.String(), "ignored") {
			t.Fatalf("should be ignored: %s -> %s", det, b.String())
		}
	}
	if post(t, st.pub.URL+"/events", `{"source":"edr-7","type":"x"}`) != http.StatusAccepted {
		t.Fatal("an ignored detection blocked a sensor")
	}
}

func ms(d time.Duration) float64 { return float64(d.Microseconds()) / 1000 }
