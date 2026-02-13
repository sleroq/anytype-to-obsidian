//go:build !darwin

package exporter

import "time"

func setFileCreationTime(_ string, _ time.Time) error {
	return nil
}
