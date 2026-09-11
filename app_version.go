package main

import (
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
)

const latestReleaseAPI = "https://api.github.com/repos/Tomnerb/DevStack/releases/latest"

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
}

type githubLatestRelease struct {
	TagName     string `json:"tag_name"`
	Name        string `json:"name"`
	HTMLURL     string `json:"html_url"`
	Body        string `json:"body"`
	PublishedAt string `json:"published_at"`
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
	request, err := http.NewRequest(http.MethodGet, latestReleaseAPI, nil)
	if err != nil {
		return UpdateInfo{}, err
	}
	request.Header.Set("Accept", "application/vnd.github+json")
	request.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	request.Header.Set("User-Agent", "DevStack/"+current)

	client := &http.Client{Timeout: 12 * time.Second}
	response, err := client.Do(request)
	if err != nil {
		return UpdateInfo{}, fmt.Errorf("check for updates: %w", err)
	}
	defer response.Body.Close()

	if response.StatusCode == http.StatusNotFound {
		return UpdateInfo{
			CurrentVersion: current,
			Message:        "No published DevStack releases are available yet.",
		}, nil
	}
	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
		detail := strings.TrimSpace(string(body))
		if detail == "" {
			detail = response.Status
		}
		return UpdateInfo{}, fmt.Errorf("GitHub update check failed: %s", detail)
	}

	var release githubLatestRelease
	if err := json.NewDecoder(io.LimitReader(response.Body, 2<<20)).Decode(&release); err != nil {
		return UpdateInfo{}, fmt.Errorf("read GitHub release: %w", err)
	}

	latest := strings.TrimPrefix(strings.TrimSpace(release.TagName), "v")
	if latest == "" {
		return UpdateInfo{}, errors.New("latest GitHub release has no version tag")
	}
	if err := validateReleaseURL(release.HTMLURL); err != nil {
		return UpdateInfo{}, err
	}

	available := compareVersions(latest, current) > 0
	message := "DevStack is up to date."
	if available {
		message = fmt.Sprintf("DevStack %s is available.", latest)
	}

	return UpdateInfo{
		CurrentVersion: current,
		LatestVersion:  latest,
		Available:      available,
		ReleaseName:    strings.TrimSpace(release.Name),
		ReleaseURL:     release.HTMLURL,
		PublishedAt:    release.PublishedAt,
		Notes:          strings.TrimSpace(release.Body),
		Message:        message,
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
