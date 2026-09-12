package main

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/updater"
)

//go:embed VERSION
var sourceVersion string

// These values are overridden with -ldflags for release builds. Keeping the
// version embedded means ordinary `go build` and developer builds still report
// the correct product version.
var (
	buildVersion = ""
	buildCommit  = "development"
	buildDate    = ""
)

type VersionInfo struct {
	Version   string `json:"version"`
	Commit    string `json:"commit"`
	BuildDate string `json:"buildDate"`
	Channel   string `json:"channel"`
}

type UpdateInfo struct {
	CurrentVersion string `json:"currentVersion"`
	LatestVersion  string `json:"latestVersion"`
	Available      bool   `json:"available"`
	ReleaseName    string `json:"releaseName"`
	ReleaseURL     string `json:"releaseUrl"`
	PublishedAt    string `json:"publishedAt"`
	Notes          string `json:"notes"`
	Message        string `json:"message"`
	ArtifactName   string `json:"artifactName"`
	ArtifactSize   int64  `json:"artifactSize"`
}

func currentVersion() string {
	if version := strings.TrimSpace(buildVersion); version != "" {
		return strings.TrimPrefix(version, "v")
	}
	return strings.TrimPrefix(strings.TrimSpace(sourceVersion), "v")
}

func (s *AppService) GetVersionInfo() VersionInfo {
	channel := "stable"
	if strings.Contains(currentVersion(), "-") {
		channel = "prerelease"
	}
	return VersionInfo{
		Version:   currentVersion(),
		Commit:    strings.TrimSpace(buildCommit),
		BuildDate: strings.TrimSpace(buildDate),
		Channel:   channel,
	}
}

func (s *AppService) CheckForUpdates() (UpdateInfo, error) {
	current := currentVersion()
	app := application.Get()
	if app == nil || app.Updater == nil {
		return UpdateInfo{}, errors.New("application updater is unavailable")
	}

	release, err := app.Updater.Check(context.Background())
	if err != nil {
		return UpdateInfo{}, fmt.Errorf("check for updates: %w", err)
	}
	if release == nil {
		return UpdateInfo{
			CurrentVersion: current,
			LatestVersion:  current,
			Message:        "DevStack is up to date.",
		}, nil
	}

	return updateInfoFromRelease(current, release)
}

// ApplyUpdate downloads the platform-specific release, verifies its SHA-256
// digest, stages it, and restarts DevStack through Wails' detached updater
// helper. The running application is only replaced after it has exited.
func (s *AppService) ApplyUpdate() error {
	app := application.Get()
	if app == nil || app.Updater == nil {
		return errors.New("application updater is unavailable")
	}
	if err := app.Updater.DownloadAndInstall(context.Background()); err != nil {
		return fmt.Errorf("download update: %w", err)
	}
	if err := app.Updater.Restart(context.Background()); err != nil {
		return fmt.Errorf("restart into update: %w", err)
	}
	return nil
}

func updateInfoFromRelease(current string, release *updater.Release) (UpdateInfo, error) {
	if release == nil {
		return UpdateInfo{}, errors.New("release is unavailable")
	}
	latest := strings.TrimPrefix(strings.TrimSpace(release.Version), "v")
	if latest == "" {
		return UpdateInfo{}, errors.New("latest GitHub release has no version tag")
	}

	releaseURL, _ := release.Metadata["github.release.htmlURL"].(string)
	if err := validateReleaseURL(releaseURL); err != nil {
		return UpdateInfo{}, err
	}

	return UpdateInfo{
		CurrentVersion: current,
		LatestVersion:  latest,
		Available:      compareVersions(latest, current) > 0,
		ReleaseName:    strings.TrimSpace(release.Name),
		ReleaseURL:     releaseURL,
		PublishedAt:    release.PublishedAt.UTC().Format("2006-01-02T15:04:05Z"),
		Notes:          strings.TrimSpace(release.Notes),
		Message:        fmt.Sprintf("DevStack %s is available.", latest),
		ArtifactName:   release.Artifact.Filename,
		ArtifactSize:   release.Artifact.Size,
	}, nil
}

func (s *AppService) OpenUpdatePage(releaseURL string) error {
	if err := validateReleaseURL(releaseURL); err != nil {
		return err
	}
	app := application.Get()
	if app == nil || app.Browser == nil {
		return errors.New("application browser is unavailable")
	}
	return app.Browser.OpenURL(releaseURL)
}

func validateReleaseURL(raw string) error {
	parsed, err := url.Parse(raw)
	if err != nil {
		return errors.New("invalid release URL")
	}
	if parsed.Scheme != "https" || !strings.EqualFold(parsed.Hostname(), "github.com") {
		return errors.New("release URL must use github.com over HTTPS")
	}
	if !strings.HasPrefix(strings.ToLower(parsed.EscapedPath()), "/tomnerb/devstack/releases/") {
		return errors.New("release URL does not belong to Tomnerb/DevStack")
	}
	return nil
}

func compareVersions(left, right string) int {
	leftParts, leftPrerelease := versionParts(left)
	rightParts, rightPrerelease := versionParts(right)
	for index := 0; index < 3; index++ {
		if leftParts[index] > rightParts[index] {
			return 1
		}
		if leftParts[index] < rightParts[index] {
			return -1
		}
	}
	if leftPrerelease != rightPrerelease {
		if leftPrerelease {
			return -1
		}
		return 1
	}
	return 0
}

func versionParts(version string) ([3]int, bool) {
	version = strings.TrimPrefix(strings.TrimSpace(version), "v")
	coreAndPrerelease := strings.SplitN(version, "-", 2)
	version = coreAndPrerelease[0]
	version = strings.SplitN(version, "+", 2)[0]
	parts := strings.Split(version, ".")
	result := [3]int{}
	for index := 0; index < len(parts) && index < len(result); index++ {
		result[index], _ = strconv.Atoi(parts[index])
	}
	return result, len(coreAndPrerelease) > 1
}
