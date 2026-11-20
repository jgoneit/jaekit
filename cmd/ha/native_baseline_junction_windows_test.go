package main

import (
	"errors"
	"testing"
)

func TestNativeWindowsBaselineJunctionScenarios(t *testing.T) {
	runBaselineJunctionScenarios(t)
}

func makeEvidenceFIFO(string) error { return errors.New("no FIFO on Windows") }
