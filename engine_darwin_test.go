//go:build darwin

package main

import (
	"compress/gzip"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestStageCompressedBundledGuestAssets(t *testing.T) {
	source := t.TempDir()
	destination := t.TempDir()
	t.Setenv("DOCKIVA_GUEST_ASSETS_PATH", source)
	t.Setenv("DOCKIVA_GUEST_DIR", destination)

	if err := os.WriteFile(filepath.Join(source, "vmlinux"), []byte("kernel"), 0o644); err != nil {
		t.Fatal(err)
	}
	disk, err := os.Create(filepath.Join(source, "rootfs.ext4.gz"))
	if err != nil {
		t.Fatal(err)
	}
	compressed := gzip.NewWriter(disk)
	if _, err := compressed.Write([]byte("compressed disk")); err != nil {
		t.Fatal(err)
	}
	if err := compressed.Close(); err != nil {
		t.Fatal(err)
	}
	if err := disk.Close(); err != nil {
		t.Fatal(err)
	}

	if err := stageBundledGuestAssets(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(destination, "rootfs.ext4"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "compressed disk" {
		t.Fatalf("rootfs.ext4 = %q, want decompressed disk", data)
	}
}

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

func TestDockivaVMMStateDirReusesRunningLegacyVM(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	legacy := filepath.Join(home, "Library", "Application Support", "DevStack", "run")
	if err := os.MkdirAll(legacy, 0o755); err != nil {
		t.Fatal(err)
	}
	pid := os.Getpid()
	if err := os.WriteFile(filepath.Join(legacy, "vmm.pid"), []byte(fmt.Sprint(pid)), 0o644); err != nil {
		t.Fatal(err)
	}
	status := fmt.Sprintf(`{"running":true,"pid":%d}`, pid)
	if err := os.WriteFile(filepath.Join(legacy, "status.json"), []byte(status), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := dockivaVMMStateDir()
	if err != nil {
		t.Fatal(err)
	}
	if got != legacy {
		t.Fatalf("state dir = %q, want running legacy state %q", got, legacy)
	}
}

func TestDockivaVMMStateDirIgnoresStaleLegacyVM(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	legacy := filepath.Join(home, "Library", "Application Support", "DevStack", "run")
	if err := os.MkdirAll(legacy, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(legacy, "vmm.pid"), []byte("99999999"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(legacy, "status.json"), []byte(`{"running":true,"pid":99999999}`), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := dockivaVMMStateDir()
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(home, "Library", "Application Support", "Dockiva", "run")
	if got != want {
		t.Fatalf("state dir = %q, want current state %q", got, want)
	}
}
