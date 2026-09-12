package main

import (
	"testing"
	"time"

	"github.com/wailsapp/wails/v3/pkg/updater"
)

func TestCompareVersions(t *testing.T) {
	tests := []struct {
		left  string
		right string
		want  int
	}{
		{"0.2.0", "0.1.0", 1},
		{"v1.0.0", "1.0.0", 0},
		{"1.2.3", "1.3.0", -1},
		{"2.0.0-beta.1", "1.9.9", 1},
		{"1.0.0", "1.0.0-beta.2", 1},
	}
	for _, test := range tests {
		if got := compareVersions(test.left, test.right); got != test.want {
			t.Fatalf("compareVersions(%q, %q) = %d, want %d", test.left, test.right, got, test.want)
		}
	}
}

func TestValidateReleaseURL(t *testing.T) {
	if err := validateReleaseURL("https://github.com/Tomnerb/DevStack/releases/tag/v0.2.0"); err != nil {
		t.Fatalf("valid release URL rejected: %v", err)
	}
	for _, raw := range []string{
		"http://github.com/Tomnerb/DevStack/releases/tag/v0.2.0",
		"https://example.com/Tomnerb/DevStack/releases/tag/v0.2.0",
		"https://github.com/another/project/releases/tag/v0.2.0",
	} {
		if err := validateReleaseURL(raw); err == nil {
			t.Fatalf("unsafe release URL accepted: %s", raw)
		}
	}
}

func TestUpdateInfoFromRelease(t *testing.T) {
	published := time.Date(2026, time.September, 12, 3, 4, 5, 0, time.UTC)
	info, err := updateInfoFromRelease("0.1.0", &updater.Release{
		Version:     "0.2.0",
		Name:        "DevStack v0.2.0",
		Notes:       "Faster updates.",
		PublishedAt: published,
		Artifact: updater.Artifact{
			Filename: "DevStack-0.2.0-darwin-arm64.zip",
			Size:     2048,
		},
		Metadata: map[string]any{
			"github.release.htmlURL": "https://github.com/Tomnerb/DevStack/releases/tag/v0.2.0",
		},
	})
	if err != nil {
		t.Fatalf("updateInfoFromRelease returned an error: %v", err)
	}
	if !info.Available || info.LatestVersion != "0.2.0" {
		t.Fatalf("unexpected update info: %+v", info)
	}
	if info.ArtifactName != "DevStack-0.2.0-darwin-arm64.zip" || info.ArtifactSize != 2048 {
		t.Fatalf("artifact metadata was not preserved: %+v", info)
	}
}
