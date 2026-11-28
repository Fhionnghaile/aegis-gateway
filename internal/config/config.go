package config

import (
	"os"
	"strconv"
)

type Config struct {
	Addr      string
	AdminAddr string // internal-only listener for /metrics
	QueueCap  int
	Workers   int
	Brokers   string
	Topic     string
	// Containment (ML n20_10): block a client after GuardThreshold invalid events within
	// GuardWindowSec; blocks last BlockTTLSec. GuardThreshold 0 turns the guard off.
	GuardThreshold int
	GuardWindowSec int
	BlockTTLSec    int
}

func FromEnv() Config {
	return Config{
		Addr:      env("AEGIS_ADDR", ":8080"),
		AdminAddr: env("AEGIS_ADMIN_ADDR", "127.0.0.1:9091"),
		QueueCap:  envInt("AEGIS_QUEUE_CAP", 100_000),
		Workers:   envInt("AEGIS_WORKERS", 8),
		Brokers:   env("AEGIS_KAFKA_BROKERS", "localhost:9092"),
		Topic:     env("AEGIS_TOPIC", "events"),

		GuardThreshold: envInt("AEGIS_GUARD_THRESHOLD", 20),
		GuardWindowSec: envInt("AEGIS_GUARD_WINDOW_SEC", 10),
		BlockTTLSec:    envInt("AEGIS_BLOCK_TTL_SEC", 900),
	}
}

func env(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}
func envInt(k string, d int) int {
	if v := os.Getenv(k); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return d
}
