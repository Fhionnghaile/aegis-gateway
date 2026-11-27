package config

import "testing"

func TestDefaults(t *testing.T) {
	for _, k := range []string{"AEGIS_ADDR", "AEGIS_QUEUE_CAP", "AEGIS_WORKERS", "AEGIS_KAFKA_BROKERS", "AEGIS_TOPIC"} {
		t.Setenv(k, "")
	}
	c := FromEnv()
	want := Config{Addr: ":8080", QueueCap: 100_000, Workers: 8, Brokers: "localhost:9092", Topic: "events"}
	if c != want {
		t.Fatalf("defaults: got %+v, want %+v", c, want)
	}
}

func TestEnvOverrides(t *testing.T) {
	t.Setenv("AEGIS_ADDR", ":9090")
	t.Setenv("AEGIS_QUEUE_CAP", "500")
	t.Setenv("AEGIS_WORKERS", "2")
	t.Setenv("AEGIS_KAFKA_BROKERS", "k1:9092,k2:9092")
	t.Setenv("AEGIS_TOPIC", "audit")
	c := FromEnv()
	want := Config{Addr: ":9090", QueueCap: 500, Workers: 2, Brokers: "k1:9092,k2:9092", Topic: "audit"}
	if c != want {
		t.Fatalf("overrides: got %+v, want %+v", c, want)
	}
}

// A malformed number must fall back to the safe default, never to 0 (a zero-capacity queue
// would reject every event).
func TestInvalidIntFallsBackToDefault(t *testing.T) {
	t.Setenv("AEGIS_QUEUE_CAP", "lots")
	t.Setenv("AEGIS_WORKERS", "8x")
	c := FromEnv()
	if c.QueueCap != 100_000 || c.Workers != 8 {
		t.Fatalf("got QueueCap=%d Workers=%d, want defaults 100000 and 8", c.QueueCap, c.Workers)
	}
}
