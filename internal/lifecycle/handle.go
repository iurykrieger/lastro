package lifecycle

import "time"

// Handle is the public reference to a sensor run. It is reconstructable
// from running_sensors.json so a cross-process StopSensor can act
// without sharing in-memory state with StartSensor.
type Handle struct {
	SensorID             string    `json:"sensor_id"`
	RunID                string    `json:"run_id"`
	RunDir               string    `json:"run_dir"`
	PID                  int       `json:"pid"`
	PGID                 int       `json:"pgid"`
	StartedAt            time.Time `json:"started_at"`
	ExpectedObservations []string  `json:"expected_observations,omitempty"`
	HarnessPID           int       `json:"harness_pid"`
	HarnessVersion       string    `json:"harness_version"`
	GOOS                 string    `json:"goos"`
}
