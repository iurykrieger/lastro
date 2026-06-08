// Command harness-tools is the single pre-compiled binary backing every
// harness skill script. Each subcommand matches the name of a skill
// (detect-stack, detect-use-cases, etc.) and accepts the same flags and
// positional arguments as the corresponding go-run-able script under
// skills/<name>/scripts/.
package main

import (
	"fmt"
	"io"
	"os"
)

func main() {
	os.Exit(run(os.Args, os.Stdin, os.Stdout, os.Stderr))
}

// run is the testable entry point.
func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) < 2 {
		fmt.Fprintf(stderr, "usage: harness-tools <subcommand> [args...]\n")
		fmt.Fprintf(stderr, "subcommands:\n")
		fmt.Fprintf(stderr, "  detect-stack        validate and persist a stack-manifest YAML\n")
		fmt.Fprintf(stderr, "  detect-use-cases    validate and persist a use-case or fixture YAML\n")
		fmt.Fprintf(stderr, "  create-sensors      validate and persist a sensor YAML\n")
		fmt.Fprintf(stderr, "  create-core-sensors validate and persist a core sensor YAML\n")
		fmt.Fprintf(stderr, "  run-sensor          run an assertion sensor\n")
		fmt.Fprintf(stderr, "  start-sensor        start an observational sensor\n")
		fmt.Fprintf(stderr, "  stop-sensor         stop an in-flight observational sensor\n")
		fmt.Fprintf(stderr, "  tail-sensor-signals tail a sensor's signals.jsonl\n")
		fmt.Fprintf(stderr, "  validate-use-case   run all sensors for a use case\n")
		fmt.Fprintf(stderr, "  heal                apply an edit plan and re-validate\n")
		return 1
	}

	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintln(stderr, "cwd:", err)
		return 1
	}

	sub, rest := args[1], args[1:]
	switch sub {
	case "detect-stack":
		return persistDetectStack(rest, stdout, stderr)
	case "detect-use-cases":
		return persistDetectUseCases(rest, stdout, stderr)
	case "create-sensors":
		return persistCreateSensors(rest, stdout, stderr)
	case "create-core-sensors":
		return persistCreateCoreSensors(rest, stdout, stderr)
	case "run-sensor":
		return runRunSensor(rest, stdin, stdout, stderr, cwd)
	case "start-sensor":
		return runStartSensor(rest, stdin, stdout, stderr, cwd)
	case "stop-sensor":
		return runStopSensor(rest, stdin, stdout, stderr, cwd)
	case "tail-sensor-signals":
		return runTailSensorSignals(rest, stdin, stdout, stderr, cwd)
	case "validate-use-case":
		return runValidateUseCase(rest, stdin, stdout, stderr, cwd)
	case "heal":
		return runHeal(rest, stdin, stdout, stderr, cwd)
	default:
		fmt.Fprintf(stderr, "harness-tools: unknown subcommand %q\n", sub)
		return 1
	}
}
