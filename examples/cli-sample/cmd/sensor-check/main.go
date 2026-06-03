package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"
)

type signalRec struct {
	SchemaVersion string         `json:"schema_version"`
	SensorID      string         `json:"sensor_id"`
	UseCaseID     string         `json:"use_case_id"`
	Angle         string         `json:"angle"`
	EmittedAt     string         `json:"emitted_at"`
	Verdict       string         `json:"verdict"`
	Confidence    float64        `json:"confidence"`
	Evidence      map[string]any `json:"evidence"`
}

func emit(rec signalRec) {
	rec.EmittedAt = time.Now().UTC().Format(time.RFC3339Nano)
	if rec.Confidence == 0 {
		rec.Confidence = 1.0
	}
	if rec.Evidence == nil {
		rec.Evidence = map[string]any{}
	}
	enc := json.NewEncoder(os.Stdout)
	_ = enc.Encode(rec)
}

func main() {
	var sensorID, useCaseID, angle, check string

	args := os.Args[1:]
	for i := 0; i < len(args); i++ {
		switch {
		case strings.HasPrefix(args[i], "--sensor="):
			sensorID = strings.TrimPrefix(args[i], "--sensor=")
		case args[i] == "--sensor" && i+1 < len(args):
			i++
			sensorID = args[i]
		case strings.HasPrefix(args[i], "--use-case="):
			useCaseID = strings.TrimPrefix(args[i], "--use-case=")
		case args[i] == "--use-case" && i+1 < len(args):
			i++
			useCaseID = args[i]
		case strings.HasPrefix(args[i], "--angle="):
			angle = strings.TrimPrefix(args[i], "--angle=")
		case args[i] == "--angle" && i+1 < len(args):
			i++
			angle = args[i]
		default:
			if !strings.HasPrefix(args[i], "-") {
				check = args[i]
			}
		}
	}

	if sensorID == "" || useCaseID == "" || angle == "" || check == "" {
		fmt.Fprintf(os.Stderr, "usage: sensor-check --sensor=<id> --use-case=<id> --angle=<angle> <pass>\n")
		os.Exit(2)
	}

	switch check {
	case "pass":
		emit(signalRec{
			SchemaVersion: "1.0.0",
			SensorID:      sensorID,
			UseCaseID:     useCaseID,
			Angle:         angle,
			Verdict:       "pass",
			Evidence:      map[string]any{"expected": "trivial", "actual": "trivial"},
		})

	default:
		fmt.Fprintf(os.Stderr, "unknown check: %s\n", check)
		os.Exit(2)
	}
}
