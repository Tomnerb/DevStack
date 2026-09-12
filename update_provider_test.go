package main

import (
	"context"
	"io"
	"testing"

	"github.com/wailsapp/wails/v3/pkg/updater"
)

type updateProviderStub struct {
	release *updater.Release
}

func (updateProviderStub) Name() string { return "stub" }

func (p updateProviderStub) Check(context.Context, updater.CheckRequest) (*updater.Release, error) {
	return p.release, nil
}

func (updateProviderStub) Download(context.Context, *updater.Release, io.Writer, func(int64, int64)) error {
	return nil
}

func TestRequiredChecksumProviderRejectsMissingDigest(t *testing.T) {
	provider := requiredChecksumProvider{provider: updateProviderStub{release: &updater.Release{Version: "0.2.0"}}}
	if _, err := provider.Check(context.Background(), updater.CheckRequest{}); err == nil {
		t.Fatal("expected a release without SHA-256 verification to be rejected")
	}
}

func TestRequiredChecksumProviderAcceptsSHA256(t *testing.T) {
	release := &updater.Release{
		Version: "0.2.0",
		Verification: &updater.Verification{
			DigestAlgo: "sha256",
			Digest:     make([]byte, 32),
		},
	}
	provider := requiredChecksumProvider{provider: updateProviderStub{release: release}}
	got, err := provider.Check(context.Background(), updater.CheckRequest{})
	if err != nil {
		t.Fatalf("valid SHA-256 release was rejected: %v", err)
	}
	if got != release {
		t.Fatal("provider did not return the verified release")
	}
}
