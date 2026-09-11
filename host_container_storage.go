package main

import "fmt"

type HostContainerStorageItem struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Description   string `json:"description"`
	Path          string `json:"path"`
	SizeBytes     int64  `json:"sizeBytes"`
	Size          string `json:"size"`
	CanReveal     bool   `json:"canReveal"`
	CanClean      bool   `json:"canClean"`
	EngineRunning bool   `json:"engineRunning"`
	CleanupLabel  string `json:"cleanupLabel"`
}

func formatStorageBytes(size int64) string {
	const unit = int64(1000)
	if size < unit {
		return fmt.Sprintf("%d B", size)
	}

	divisor := unit
	exponent := 0
	for value := size / unit; value >= unit && exponent < 4; value /= unit {
		divisor *= unit
		exponent++
	}

	units := [...]string{"KB", "MB", "GB", "TB", "PB"}
	return fmt.Sprintf("%.2f %s", float64(size)/float64(divisor), units[exponent])
}
