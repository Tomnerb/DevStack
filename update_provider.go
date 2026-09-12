package main

import (
	"context"
	"errors"
	"io"

	"github.com/wailsapp/wails/v3/pkg/updater"
)

// requiredChecksumProvider prevents an update source from silently falling
// back to an unverified artifact when its checksum sidecar is missing or does
// not contain the selected platform archive.
type requiredChecksumProvider struct {
	provider updater.Provider
}

func (p requiredChecksumProvider) Name() string {
	return p.provider.Name()
}

func (p requiredChecksumProvider) Check(ctx context.Context, request updater.CheckRequest) (*updater.Release, error) {
	release, err := p.provider.Check(ctx, request)
	if err != nil || release == nil {
		return release, err
	}
	if release.Verification == nil || release.Verification.DigestAlgo != "sha256" || len(release.Verification.Digest) != 32 {
		return nil, errors.New("release is missing a valid SHA-256 entry in SHA256SUMS")
	}
	return release, nil
}

func (p requiredChecksumProvider) Download(
	ctx context.Context,
	release *updater.Release,
	destination io.Writer,
	onProgress func(written, total int64),
) error {
	return p.provider.Download(ctx, release, destination, onProgress)
}
