//go:build ignore
// +build ignore

// fakesensor is a test helper compiled by executor/lifecycle test
// packages via TestMain. The build tag prevents it from being picked up
// during normal `go build ./...`.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"syscall"
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
	HealHint      *healHint      `json:"heal_hint,omitempty"`
}

type healHint struct {
	Summary   string `json:"summary"`
	Rationale string `json:"rationale"`
}

func emit(rec signalRec) {
	if rec.SchemaVersion == "" {
		rec.SchemaVersion = "1.0.0"
	}
	if rec.SensorID == "" {
		rec.SensorID = os.Getenv("HARNESS_SENSOR_ID")
		if rec.SensorID == "" {
			rec.SensorID = "fake"
		}
	}
	if rec.UseCaseID == "" {
		rec.UseCaseID = os.Getenv("HARNESS_USE_CASE_ID")
		if rec.UseCaseID == "" {
			rec.UseCaseID = "fake-uc"
		}
	}
	if rec.Angle == "" {
		rec.Angle = "build"
	}
	rec.EmittedAt = time.Now().UTC().Format(time.RFC3339Nano)
	if rec.Confidence == 0 {
		rec.Confidence = 1.0
	}
	if rec.Evidence == nil {
		rec.Evidence = map[string]any{}
	}
	enc := json.NewEncoder(os.Stdout)
	_ = enc.Encode(rec)
	// Give the executor's pumpStdout goroutine time to drain the pipe
	// before cmd.Wait() closes the read end. Without this, fast-exiting
	// sensors race the scanner ("file already closed" → 0 signals →
	// spurious inconclusive verdict). Same workaround the dogfood
	// sensors apply via `sleep 0.1`. Keep brief to avoid slowing the
	// dozens of executor/lifecycle tests that use this helper.
	time.Sleep(20 * time.Millisecond)
}

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: fakesensor <subcommand> [args...]")
		os.Exit(2)
	}
	args := os.Args[1:]
	switch args[0] {
	case "signal":
		cmdSignal(args[1:])
	case "stream":
		cmdStream(args[1:])
	case "crash":
		cmdCrash(args[1:])
	case "watch":
		cmdWatch(args[1:])
	case "sleep":
		cmdSleep(args[1:])
	default:
		fmt.Fprintln(os.Stderr, "fakesensor: unknown subcommand:", args[0])
		os.Exit(2)
	}
}

func cmdSignal(args []string) {
	if len(args) == 0 {
		os.Exit(2)
	}
	rec := signalRec{Verdict: args[0]}
	var summary, obsKey, angle string
	for i := 1; i < len(args); i++ {
		switch args[i] {
		case "--observation-key":
			i++
			if i < len(args) {
				obsKey = args[i]
			}
		case "--summary":
			i++
			if i < len(args) {
				summary = args[i]
			}
		case "--angle":
			i++
			if i < len(args) {
				angle = args[i]
			}
		}
	}
	if angle != "" {
		rec.Angle = angle
	}
	if obsKey != "" {
		rec.Evidence = map[string]any{"observation_key": obsKey}
	}
	if rec.Verdict == "fail" || rec.Verdict == "warn" {
		s := summary
		if s == "" {
			s = "fake failure"
		}
		rec.HealHint = &healHint{Summary: s, Rationale: "fakesensor-generated"}
	}
	emit(rec)
}

func cmdStream(args []string) {
	if len(args) == 0 {
		os.Exit(2)
	}
	n, err := strconv.Atoi(args[0])
	if err != nil {
		os.Exit(2)
	}
	interval := time.Duration(0)
	for i := 1; i < len(args); i++ {
		if args[i] == "--interval" {
			i++
			if d, err := time.ParseDuration(args[i]); err == nil {
				interval = d
			}
		}
	}
	for i := 0; i < n; i++ {
		emit(signalRec{Verdict: "pass"})
		if interval > 0 {
			time.Sleep(interval)
		}
	}
}

func cmdCrash(args []string) {
	exitCode := 1
	msg := "fakesensor crashed"
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--exit-code":
			i++
			if i < len(args) {
				if c, err := strconv.Atoi(args[i]); err == nil {
					exitCode = c
				}
			}
		case "--stderr":
			i++
			if i < len(args) {
				msg = args[i]
			}
		}
	}
	fmt.Fprintln(os.Stderr, msg)
	os.Exit(exitCode)
}

func cmdWatch(args []string) {
	var emits []string
	interval := 50 * time.Millisecond
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--emit":
			i++
			if i < len(args) {
				emits = append(emits, args[i])
			}
		case "--interval":
			i++
			if i < len(args) {
				if d, err := time.ParseDuration(args[i]); err == nil {
					interval = d
				}
			}
		}
	}
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)
	for _, k := range emits {
		select {
		case <-sigCh:
			return
		default:
		}
		emit(signalRec{Angle: "logs", Verdict: "pass", Evidence: map[string]any{"observation_key": k}})
		time.Sleep(interval)
	}
	// Then idle until signaled.
	<-sigCh
}

func cmdSleep(args []string) {
	if len(args) == 0 {
		os.Exit(2)
	}
	d, err := time.ParseDuration(args[0])
	if err != nil {
		os.Exit(2)
	}
	time.Sleep(d)
}
