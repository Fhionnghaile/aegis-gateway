package event

import (
	"testing"
	"time"
)

func TestNormaliseDefaults(t *testing.T) {
	e := Event{Source: "edr", Type: "  LOGIN_FAIL "}
	now := time.Unix(1700000000, 0)
	e.Normalise(now)
	if e.Type != "login_fail" {
		t.Fatalf("type not normalised: %q", e.Type)
	}
	if e.Severity != "info" {
		t.Fatalf("severity default: %q", e.Severity)
	}
	if e.ID == "" || e.ReceivedAt != now || e.Timestamp != now {
		t.Fatalf("fields not filled: %+v", e)
	}
}

func TestPartitionKey(t *testing.T) {
	if string(Event{Source: "a", Type: "b"}.PartitionKey()) != "a|b" {
		t.Fatal("bad key")
	}
}
