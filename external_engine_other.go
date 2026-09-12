//go:build !darwin

package main

func startExternalDockerOnRequest(_ string) (bool, error) {
	return false, nil
}
