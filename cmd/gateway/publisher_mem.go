//go:build !kafka

package main

import (
	"github.com/Fhionnghaile/aegis-gateway/internal/config"
	"github.com/Fhionnghaile/aegis-gateway/internal/publish"
)

func newPublisher(cfg config.Config) publish.Publisher { return &publish.MemoryPublisher{} }
