//go:build !darwin

package main

import "errors"

func (s *DockerService) GetHostContainerStorage() ([]HostContainerStorageItem, error) {
	return []HostContainerStorageItem{}, nil
}

func (s *DockerService) RevealHostContainerStorage(string) error {
	return errors.New("revealing host container storage is currently available on macOS")
}

func (s *DockerService) CleanHostContainerStorage(string, string) (DockerCLIResult, error) {
	return DockerCLIResult{}, errors.New("host container storage cleanup is currently available on macOS")
}
