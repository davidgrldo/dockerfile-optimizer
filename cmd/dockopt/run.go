package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/davidgrldo/dockerfile-optimizer/internal/analyzer"
	"github.com/davidgrldo/dockerfile-optimizer/internal/dockerfile"
	"github.com/davidgrldo/dockerfile-optimizer/internal/report"
)

func run(args []string, stdout, stderr io.Writer) int {
	return runWithOpener(args, stdout, stderr, func(path string) (io.ReadCloser, error) {
		return os.Open(path)
	})
}

func runWithOpener(args []string, stdout, stderr io.Writer, open func(string) (io.ReadCloser, error)) int {
	jsonRequested := requestsJSON(args)
	flags := flag.NewFlagSet("dockopt", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	jsonMode := flags.Bool("json", false, "output results as JSON")
	stackName := flags.String("stack", "", "override detected stack")
	failOn := flags.String("fail-on", "error", "failure threshold: none, warn, or error")
	if err := flags.Parse(args); err != nil {
		return writeFailure(stdout, stderr, jsonRequested, "usage_error", err)
	}
	if flags.NArg() != 1 {
		return writeFailure(stdout, stderr, *jsonMode, "usage_error", errors.New("expected exactly one Dockerfile path"))
	}
	if *failOn != "none" && *failOn != "warn" && *failOn != "error" {
		return writeFailure(stdout, stderr, *jsonMode, "usage_error", fmt.Errorf("invalid fail-on threshold %q", *failOn))
	}

	var override analyzer.Stack
	if *stackName != "" {
		var err error
		override, err = analyzer.ParseStack(*stackName)
		if err != nil {
			return writeFailure(stdout, stderr, *jsonMode, "usage_error", fmt.Errorf("unknown stack %q", *stackName))
		}
	}

	path := flags.Arg(0)
	file, err := open(path)
	if err != nil {
		return writeFailure(stdout, stderr, *jsonMode, "input_error", err)
	}

	doc, err := dockerfile.Parse(path, file)
	closeErr := file.Close()
	if err != nil {
		kind := "input_error"
		var parseErr *dockerfile.ParseError
		if errors.As(err, &parseErr) {
			kind = "parse_error"
		}
		return writeFailure(stdout, stderr, *jsonMode, kind, err)
	}
	if closeErr != nil {
		return writeFailure(stdout, stderr, *jsonMode, "input_error", fmt.Errorf("close %s: %w", path, closeErr))
	}
	selected := analyzer.DetectStack(doc)
	if *stackName != "" {
		selected = override
	}
	result := analyzer.Analyze(doc, selected)

	if *jsonMode {
		err = report.WriteJSON(stdout, result)
	} else {
		err = report.WriteHuman(stdout, result)
	}
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "output_error: %v\n", err)
		return 2
	}
	if meetsThreshold(result.Findings, *failOn) {
		return 1
	}
	return 0
}

func requestsJSON(args []string) bool {
	jsonMode := false
	for index := 0; index < len(args); index++ {
		arg := args[index]
		if arg == "--" || !strings.HasPrefix(arg, "-") {
			break
		}
		flagText := strings.TrimPrefix(arg, "-")
		flagText = strings.TrimPrefix(flagText, "-")
		name, value, hasValue := strings.Cut(flagText, "=")
		switch name {
		case "json":
			if !hasValue {
				jsonMode = true
				continue
			}
			if parsed, err := strconv.ParseBool(value); err == nil {
				jsonMode = parsed
			}
		case "stack", "fail-on":
			if hasValue {
				continue
			}
			index++
		}
	}
	return jsonMode
}

func writeFailure(stdout, stderr io.Writer, jsonMode bool, kind string, err error) int {
	if jsonMode {
		if writeErr := report.WriteErrorJSON(stdout, kind, err.Error()); writeErr != nil {
			_, _ = fmt.Fprintf(stderr, "output_error: %v\n", writeErr)
		}
	} else {
		_, _ = fmt.Fprintf(stderr, "%s: %v\n", kind, err)
	}
	return 2
}

func meetsThreshold(findings []analyzer.Finding, threshold string) bool {
	for _, finding := range findings {
		if threshold == "warn" && (finding.Severity == analyzer.SeverityWarn || finding.Severity == analyzer.SeverityError) ||
			threshold == "error" && finding.Severity == analyzer.SeverityError {
			return true
		}
	}
	return false
}
