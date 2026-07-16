package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/davidgrldo/dockerfile-optimizer/internal/analyzer"
	"github.com/davidgrldo/dockerfile-optimizer/internal/dockerfile"
	"github.com/davidgrldo/dockerfile-optimizer/internal/report"
)

func run(args []string, stdout, stderr io.Writer) int {
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
	file, err := os.Open(path)
	if err != nil {
		return writeFailure(stdout, stderr, *jsonMode, "input_error", err)
	}
	defer file.Close()

	doc, err := dockerfile.Parse(path, file)
	if err != nil {
		kind := "input_error"
		var parseErr *dockerfile.ParseError
		if errors.As(err, &parseErr) {
			kind = "parse_error"
		}
		return writeFailure(stdout, stderr, *jsonMode, kind, err)
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
		switch arg {
		case "--json", "--json=true":
			jsonMode = true
		case "--json=false":
			jsonMode = false
		case "--stack", "--fail-on":
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
