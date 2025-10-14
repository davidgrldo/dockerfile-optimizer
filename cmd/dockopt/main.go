package main

import (
	flag "github.com/spf13/pflag"

	`fmt`
	"log"
	`os`

	"github.com/davidgrldo/dockerfile-optimizer/internal/analyzer"
	"github.com/davidgrldo/dockerfile-optimizer/internal/parser"
	"github.com/davidgrldo/dockerfile-optimizer/internal/report"
)

var (
	jsonOutput    = flag.Bool("json", false, "Output results as JSON")
	stackOverride = flag.String("stack", "", "Override detected stack (e.g. go, rust)")
	message       = `
Usage:
1. Run from binary:
	dockopt <Dockerfile path> [--json] [--stack=name]
			
2. Run from source:
	go run ./cmd/dockopt/main.go <Dockerfile path> [--json] [--stack=name]
`
)

func main() {
	flag.Parse()
	args := flag.Args()

	if len(args) < 1 {
		log.Fatal(message)
	}
	path := args[0]

	lines, err := parser.ParseDockerfile(path)
	if os.IsNotExist(err) {
		log.Fatalf("❌ File not found: %s", path)
	}
	if err != nil {
		log.Fatalf("Failed to parse Dockerfile: %v", err)
	}

	var stack string
	if *stackOverride != "" {
		stack = *stackOverride
	} else {
		stack = analyzer.DetectStack(lines)
	}
	results := analyzer.RunChecksDetailed(lines, stack)

	if *jsonOutput {
		report.PrintJSON(results, stack)
	} else {
		fmt.Printf("🔍 Detected stack: %s\n", stack)
		report.PrintHuman(results)
	}
}
