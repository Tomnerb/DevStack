package main

import (
	"context"
	"strings"
	"time"
)

type runtimeStatsProvider interface {
	RuntimeStats(ctx context.Context, containerIDs []string) []ContainerResourceStats
}

type runtimeLogProvider interface {
	RuntimeReadLogs(ctx context.Context, containerID string, tail int, offset int64) (RuntimeLogChunk, error)
}

type runtimeTerminalProvider interface {
	RuntimeOpenTerminal(ctx context.Context, containerID string, width uint, height uint) (string, error)
	RuntimeTerminalInput(ctx context.Context, sessionID string, input string) error
	RuntimeTerminalResize(ctx context.Context, sessionID string, width uint, height uint) error
	RuntimeTerminalOutput(ctx context.Context, sessionID string, offset int64) (RuntimeTerminalChunk, error)
	RuntimeTerminalClose(ctx context.Context, sessionID string) error
}

type RuntimeLogChunk struct {
	Data       string `json:"data"`
	NextOffset int64  `json:"nextOffset"`
	Closed     bool   `json:"closed"`
}

type RuntimeTerminalChunk struct {
	Data       string `json:"data"`
	NextOffset int64  `json:"nextOffset"`
	Closed     bool   `json:"closed"`
	Error      string `json:"error,omitempty"`
}

func (s *DockerService) activeRuntimeProvider() string {
	ctx, cancel := context.WithTimeout(context.Background(), 1200*time.Millisecond)
	defer cancel()

	return strings.ToLower(s.currentRuntime().Info(ctx).Provider)
}
