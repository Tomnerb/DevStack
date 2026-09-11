package main

import "testing"

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
