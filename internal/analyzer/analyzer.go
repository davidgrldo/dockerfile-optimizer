package analyzer

func RunChecks(lines []string, stack string) []string {
	var findings []string

	for _, rule := range rules {
		if rule.Stack == "generic" || rule.Stack == stack {
			f := rule.Check(lines)
			if f != "" {
				findings = append(findings, f)
			}
		}
	}

	return findings
}
