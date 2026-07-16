package main

import (
	flag "github.com/spf13/pflag"

	"log"
	"os"

	"github.com/davidgrldo/dockerfile-optimizer/internal/analyzer"
	"github.com/davidgrldo/dockerfile-optimizer/internal/dockerfile"
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

	file, err := os.Open(path)
	if os.IsNotExist(err) {
		log.Fatalf("❌ File not found: %s", path)
	}
	if err != nil {
		log.Fatalf("Failed to open Dockerfile: %v", err)
	}
	defer file.Close()

	doc, err := dockerfile.Parse(path, file)
	if err != nil {
		log.Fatalf("Failed to parse Dockerfile: %v", err)
	}

	var stack analyzer.Stack
	if *stackOverride != "" {
		stack, err = analyzer.ParseStack(*stackOverride)
		if err != nil {
			log.Fatal(err)
		}
	} else {
		stack = analyzer.DetectStack(doc)
	}
	result := analyzer.Analyze(doc, stack)
	var writeErr error
	if *jsonOutput {
		writeErr = report.WriteJSON(os.Stdout, result)
	} else {
		writeErr = report.WriteHuman(os.Stdout, result)
	}
	if writeErr != nil {
		log.Fatalf("Failed to write output: %v", writeErr)
	}
}
