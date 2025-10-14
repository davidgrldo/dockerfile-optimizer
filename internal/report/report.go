package report

import "fmt"

func Print(findings []string) {
	if len(findings) == 0 {
		fmt.Println("✅ No issues found!")
		return
	}
	fmt.Println("🚨 Optimization Suggestions:")
	for _, f := range findings {
		fmt.Printf(" - %s\n", f)
	}
}
