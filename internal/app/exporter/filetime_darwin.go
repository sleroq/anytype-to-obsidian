//go:build darwin

package exporter

import (
	"os/exec"
	"time"
)

func setFileCreationTime(path string, created time.Time) error {
	if created.IsZero() {
		return nil
	}
	setFilePath, err := exec.LookPath("SetFile")
	if err != nil {
		return nil
	}
	ts := created.Local().Format("01/02/2006 15:04:05")
	return exec.Command(setFilePath, "-d", ts, path).Run()
}
