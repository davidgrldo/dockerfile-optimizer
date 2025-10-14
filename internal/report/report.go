package report

import (
	`encoding/json`
	`fmt`

	`github.com/davidgrldo/dockerfile-optimizer/internal/analyzer`
)

func PrintJSON(results []analyzer.Suggestion, stack string) {
	output := map[string]interface{}{
		"stack":       stack,
		"suggestions": results,
	}

	data, err := json.MarshalIndent(output, "", "  ")
	if err != nil {
		fmt.Println("Failed to format JSON:", err)
		return
	}
	fmt.Println(string(data))
}

func PrintHuman(results []analyzer.Suggestion) {
	if len(results) == 0 {
		fmt.Println("✅ No issues found!")
		return
	}

	fmt.Println("🚨 Optimization Suggestions:")
	for _, r := range results {
		fmt.Printf(" - [%s] %s\n", r.Severity, r.Description)
	}
}
