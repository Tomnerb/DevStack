package main

import "testing"

func TestFormatStorageBytes(t *testing.T) {
	tests := map[int64]string{
		0:              "0 B",
		1000:           "1.00 KB",
		5_000_000:      "5.00 MB",
		55_450_000_000: "55.45 GB",
	}

	for size, expected := range tests {
		if actual := formatStorageBytes(size); actual != expected {
			t.Fatalf("formatStorageBytes(%d) = %q, want %q", size, actual, expected)
		}
	}
}
