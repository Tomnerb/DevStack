//go:build darwin

package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStageBundledGuestAssets(t *testing.T) {
	source := t.TempDir()
	destination := t.TempDir()
	t.Setenv("DOCKIVA_GUEST_ASSETS_PATH", source)
	t.Setenv("DOCKIVA_GUEST_DIR", destination)

	for name, content := range map[string]string{
		"vmlinux":      "kernel",
		"rootfs.ext4":  "disk",
		"manifest.txt": "manifest",
	} {
		if err := os.WriteFile(filepath.Join(source, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	if err := stageBundledGuestAssets(); err != nil {
		t.Fatal(err)
	}

	for name, want := range map[string]string{
		"vmlinux":      "kernel",
		"rootfs.ext4":  "disk",
		"manifest.txt": "manifest",
	} {
		data, err := os.ReadFile(filepath.Join(destination, name))
		if err != nil {
			t.Fatal(err)
		}
		if string(data) != want {
			t.Fatalf("%s = %q, want %q", name, data, want)
		}
	}

	if err := os.WriteFile(filepath.Join(destination, "rootfs.ext4"), []byte("persistent"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := stageBundledGuestAssets(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(destination, "rootfs.ext4"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "persistent" {
		t.Fatalf("existing guest disk was overwritten: %q", data)
	}
}
