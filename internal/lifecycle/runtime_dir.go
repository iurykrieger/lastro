package lifecycle

import "path/filepath"

func runDirPath(runtimeRoot, sensorID, runID string) string {
	return filepath.Join(runtimeRoot, sensorID, runID)
}

func registryPath(runtimeRoot string) string {
	return filepath.Join(runtimeRoot, "running_sensors.json")
}
