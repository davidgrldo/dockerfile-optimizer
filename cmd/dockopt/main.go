package main

import (
	`fmt`
	"log"
	"os"

	"github.com/davidgrldo/dockerfile-optimizer/internal/analyzer"
	"github.com/davidgrldo/dockerfile-optimizer/internal/parser"
	"github.com/davidgrldo/dockerfile-optimizer/internal/report"
)

func main() {
	if len(os.Args) < 2 {
		log.Fatal("Usage: go run ./cmd/dockopt/main.go <Dockerfile path>")
	}
	path := os.Args[1]

	var stack string
	detected := analyzer.DetectStackFromFile(path)
	fmt.Printf("🔍 Detected stack: %s\n", detected)
	stack = detected

	lines, err := parser.ParseDockerfile(path)
	if err != nil {
		log.Fatalf("Failed to parse Dockerfile: %v", err)
	}

	results := analyzer.RunChecks(lines, stack)
	report.Print(results)
}
