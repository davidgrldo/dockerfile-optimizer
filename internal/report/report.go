package report

import (
	`encoding/json`
	`fmt`
	`io`
	`os`

	`github.com/davidgrldo/dockerfile-optimizer/internal/analyzer`
)

var out io.Writer = os.Stdout

func PrintJSON(results []analyzer.Suggestion, stack string) {
	output := map[string]interface{}{
		"stack":       stack,
		"suggestions": results,
	}

	data, err := json.MarshalIndent(output, "", "  ")
	if err != nil {
		_, _ = fmt.Fprintln(out, "Failed to format JSON:", err)
		return
	}
	_, _ = fmt.Fprintln(out, string(data))
}

func PrintHuman(results []analyzer.Suggestion) {
	if len(results) == 0 {
		_, _ = fmt.Fprintln(out, "✅ No issues found!")
		return
	}

	_, _ = fmt.Fprintln(out, "🚨 Optimization Suggestions:")
	for _, r := range results {
		_, _ = fmt.Fprintf(out, " - [%s] %s\n", r.Severity, r.Description)
	}
}
