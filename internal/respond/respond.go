// Package respond closes the Aegis loop (ML n20_10): a detection from aegis-detector becomes a
// block at the gateway, automatically and within a measured time.
//
// It accepts the detector's own output format, one JSON object per detection:
//
//	{"rule_id":"sensor-flood","title":"...","severity":"high","detected_at":"...","group_key":"edr-7","count":1000}
//
// A Policy maps rule_id to a containment action; the detection's group_key names what to block.
// Only detections at or above MinSeverity are acted on; everything else is logged and ignored, so
// a noisy low-severity rule can never lock out a sensor.
package respond

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// Detection is the JSON aegis-detector publishes to its `detections` topic.
type Detection struct {
	RuleID     string `json:"rule_id"`
	Title      string `json:"title"`
	Severity   string `json:"severity"`
	DetectedAt string `json:"detected_at"`
	GroupKey   string `json:"group_key"`
	Count      int    `json:"count"`
}

// Action says what to block for a rule: Kind "source" (the event source, e.g. a sensor) or "ip".
type Action struct {
	Kind       string `json:"kind"`
	TTLSeconds int    `json:"ttl_seconds"`
}

// DefaultPolicy: the rules whose detections mean "this sender is hostile or broken".
func DefaultPolicy() map[string]Action {
	return map[string]Action{
		"sensor-flood":     {Kind: "source", TTLSeconds: 900},  // a sensor suddenly floods: compromised or broken
		"sensor-forgery":   {Kind: "source", TTLSeconds: 3600}, // events claiming a source they can't be from
		"gateway-abuse-ip": {Kind: "ip", TTLSeconds: 900},
	}
}

var rank = map[string]int{"info": 0, "low": 1, "medium": 2, "high": 3, "critical": 4}

// Result is what happened to one detection, with the time it took to put the block in place.
type Result struct {
	Detection Detection
	Key       string        // block key sent to the gateway ("" if none)
	Outcome   string        // "blocked", "ignored: <why>", "error: <why>"
	Took      time.Duration // detection received -> gateway confirmed the block
}

// Responder turns detections into gateway blocks.
type Responder struct {
	AdminURL    string // gateway admin listener, e.g. http://127.0.0.1:9091
	Client      *http.Client
	Policy      map[string]Action
	MinSeverity string
	OnResult    func(Result)
}

func New(adminURL string) *Responder {
	return &Responder{AdminURL: strings.TrimRight(adminURL, "/"), Client: &http.Client{Timeout: 5 * time.Second},
		Policy: DefaultPolicy(), MinSeverity: "high"}
}

// Handle acts on one detection. It returns only after the gateway has confirmed the block, so
// once Handle returns "blocked", the next request from that sender is refused.
func (r *Responder) Handle(ctx context.Context, d Detection) (res Result) {
	start := time.Now()
	res.Detection = d
	defer func() {
		res.Took = time.Since(start)
		if r.OnResult != nil {
			r.OnResult(res)
		}
	}()
	act, ok := r.Policy[d.RuleID]
	switch {
	case !ok:
		res.Outcome = "ignored: no containment policy for rule " + d.RuleID
		return res
	case rank[strings.ToLower(d.Severity)] < rank[r.MinSeverity]:
		res.Outcome = "ignored: severity " + d.Severity + " below " + r.MinSeverity
		return res
	case strings.TrimSpace(d.GroupKey) == "":
		res.Outcome = "ignored: detection names nothing to block (no group_key)"
		return res
	}
	res.Key = act.Kind + ":" + d.GroupKey
	body, _ := json.Marshal(map[string]any{"key": res.Key, "ttl_seconds": act.TTLSeconds,
		"reason": fmt.Sprintf("responder: %s (%s) at %s", d.RuleID, d.Severity, d.DetectedAt)})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.AdminURL+"/admin/block", bytes.NewReader(body))
	if err != nil {
		res.Outcome = "error: " + err.Error()
		return res
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := r.Client.Do(req)
	if err != nil {
		res.Outcome = "error: " + err.Error()
		return res
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		res.Outcome = fmt.Sprintf("error: gateway answered %d", resp.StatusCode)
		return res
	}
	res.Outcome = "blocked"
	return res
}

// Handler serves POST /detections with one detection (or a JSON array of them).
func (r *Responder) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /detections", func(wr http.ResponseWriter, req *http.Request) {
		raw := bytes.TrimSpace(readAll(req))
		var ds []Detection
		if len(raw) > 0 && raw[0] == '[' {
			if json.Unmarshal(raw, &ds) != nil {
				http.Error(wr, "bad json", http.StatusBadRequest)
				return
			}
		} else {
			var d Detection
			if json.Unmarshal(raw, &d) != nil {
				http.Error(wr, "bad json", http.StatusBadRequest)
				return
			}
			ds = []Detection{d}
		}
		out := make([]map[string]any, 0, len(ds))
		for _, d := range ds {
			res := r.Handle(req.Context(), d)
			out = append(out, map[string]any{"rule_id": d.RuleID, "key": res.Key, "outcome": res.Outcome, "took_ms": float64(res.Took.Microseconds()) / 1000})
		}
		wr.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(wr).Encode(out)
	})
	return mux
}

func readAll(req *http.Request) []byte {
	var b bytes.Buffer
	_, _ = b.ReadFrom(http.MaxBytesReader(nil, req.Body, 1<<20))
	return b.Bytes()
}
