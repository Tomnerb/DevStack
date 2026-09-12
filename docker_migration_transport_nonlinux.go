//go:build !linux

package main

import (
	"context"
	"errors"
)

func networkHelperImportDockerImages(
	context.Context,
	string,
) error {
	return errors.New(
		"Docker image migration into Dockiva Native is currently available only on Linux",
	)
}

func networkHelperUploadDockerVolume(
	context.Context,
	string,
	string,
) error {
	return errors.New(
		"Docker volume migration into Dockiva Native is currently available only on Linux",
	)
}
