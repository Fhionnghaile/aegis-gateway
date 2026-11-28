package contain

import (
	"testing"
	"time"
)

type clock struct{ t time.Time }

func (c *clock) now() time.Time      { return c.t }
func (c *clock) add(d time.Duration) { c.t = c.t.Add(d) }
func newClock() *clock               { return &clock{t: time.Date(2026, 11, 28, 12, 0, 0, 0, time.UTC)} }

func TestBlockExpiresAndUnblocks(t *testing.T) {
	c := newClock()
	b := NewBlocklist()
	b.Now = c.now
	b.Block("ip:10.0.0.9", "test", time.Minute)
	if !b.Blocked("ip:10.0.0.9") || b.Blocked("ip:10.0.0.8") {
		t.Fatal("block not applied to exactly the key")
	}
	c.add(61 * time.Second)
	if b.Blocked("ip:10.0.0.9") || len(b.List()) != 0 {
		t.Fatal("block did not expire")
	}
	b.Block("source:s1", "test", time.Hour)
	if !b.Unblock("source:s1") || b.Blocked("source:s1") || b.Unblock("source:s1") {
		t.Fatal("unblock failed")
	}
}

func TestValidKey(t *testing.T) {
	for k, want := range map[string]bool{"ip:1.2.3.4": true, "source:edr-1": true, "ip:": false, "user:bob": false, "": false} {
		if ValidKey(k) != want {
			t.Errorf("ValidKey(%q) = %v", k, !want)
		}
	}
}

// The guard blocks on exactly the Threshold-th bad request inside the window, not before,
// and bad requests spread wider than the window never trigger it.
func TestGuardThresholdAndWindow(t *testing.T) {
	c := newClock()
	b := NewBlocklist()
	b.Now = c.now
	var got []Containment
	g := NewGuard(b, 5, 10*time.Second, time.Minute)
	g.OnContain = func(x Containment) { got = append(got, x) }

	for i := 0; i < 6; i++ { // one every 3 s: never 5 inside 10 s
		if g.Bad("10.0.0.7") {
			t.Fatal("slow trickle should not trigger")
		}
		c.add(3 * time.Second)
	}
	for i := 1; i <= 5; i++ {
		blocked := g.Bad("10.0.0.9")
		if blocked != (i == 5) {
			t.Fatalf("request %d: blocked=%v", i, blocked)
		}
		c.add(100 * time.Millisecond)
	}
	if !b.Blocked("ip:10.0.0.9") || b.Blocked("ip:10.0.0.7") || len(got) != 1 {
		t.Fatalf("wrong containment: %+v", got)
	}
	if got[0].BadRequests != 5 || got[0].TimeToBlock != 400*time.Millisecond {
		t.Fatalf("measurement wrong: %+v", got[0])
	}
}
