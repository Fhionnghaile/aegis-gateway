//go:build !kafka

package main

import (
	"github.com/padraigf/aegis-gateway/internal/config"
	"github.com/padraigf/aegis-gateway/internal/publish"
)

func newPublisher(cfg config.Config) publish.Publisher { return &publish.MemoryPublisher{} }
