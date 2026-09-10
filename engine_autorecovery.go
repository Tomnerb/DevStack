package main

import (
	"log"
	"strings"

	"github.com/wailsapp/wails/v3/pkg/application"
)

func (s *DockerService) AutoRecoverEngine(
	backend string,
	platformOption string,
	autoReconnect bool,
	startEngine bool,
) {
	backend = normalizeEngineBackend(backend)

	if backend == "external" && autoReconnect {
		if err := s.ReconnectExternalDocker(); err == nil {
			s.emitEngineRecovered(
				"docker-reconnected",
			)
			return
		}
	}

	if backend == "external" || !startEngine {
		return
	}

	if tryPlatformAutomaticEngineStart(
		s,
		backend,
		platformOption,
	) {
		s.emitEngineRecovered(
			"engine-started",
		)
		return
	}

	log.Printf(
		"DevStack engine auto-recovery could not start backend %q",
		strings.TrimSpace(backend),
	)
}

func (s *DockerService) emitEngineRecovered(
	action string,
) {
	app := application.Get()
	if app == nil {
		return
	}

	app.Event.Emit(
		dockerChangedEvent,
		DockerEventNotice{
			Type:   "engine",
			Action: action,
		},
	)
}
