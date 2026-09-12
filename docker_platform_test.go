package main

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestResolveExternalDockerEndpointReplacesMissingLocalSocket(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix socket endpoint test")
	}

	home := t.TempDir()
	preferred := "unix://" + filepath.Join(home, "current", "docker.sock")
	t.Setenv("DOCKER_HOST", preferred)

	missing := "unix://" + filepath.Join(home, "removed", "docker.sock")
	if got := resolveExternalDockerEndpoint(missing); got != preferred {
		t.Fatalf("resolved endpoint = %q, want %q", got, preferred)
	}
}

func TestResolveExternalDockerEndpointPreservesExistingSocket(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix socket endpoint test")
	}

	home := t.TempDir()
	existingPath := filepath.Join(home, "saved", "docker.sock")
	if err := os.MkdirAll(filepath.Dir(existingPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(existingPath, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	existing := "unix://" + existingPath
	t.Setenv("DOCKER_HOST", "unix://"+filepath.Join(home, "other", "docker.sock"))

	if got := resolveExternalDockerEndpoint(existing); got != existing {
		t.Fatalf("resolved endpoint = %q, want saved endpoint %q", got, existing)
	}
}

func TestResolveExternalDockerEndpointPreservesRemoteEndpoint(t *testing.T) {
	t.Setenv("DOCKER_HOST", "unix:///tmp/docker.sock")
	const remote = "tcp://docker.example:2376"
	if got := resolveExternalDockerEndpoint(remote); got != remote {
		t.Fatalf("resolved endpoint = %q, want remote endpoint %q", got, remote)
	}
}
