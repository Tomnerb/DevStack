//go:build !linux

package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
)

func (s *DockerService) importDockerImagesToNative(ctx context.Context, archive string) error {
	file, err := os.Open(archive)
	if err != nil {
		return fmt.Errorf("open image archive: %w", err)
	}
	defer file.Close()

	result, err := s.client.Load().ImageLoad(ctx, file)
	if err != nil {
		return err
	}
	defer result.Close()
	if _, err := io.Copy(io.Discard, result); err != nil {
		return fmt.Errorf("read native image import response: %w", err)
	}
	return nil
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
