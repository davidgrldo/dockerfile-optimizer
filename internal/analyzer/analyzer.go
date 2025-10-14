package analyzer

type Suggestion struct {
	Description string
	Severity    string
}

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

func RunChecksDetailed(lines []string, stack string) []Suggestion {
	var findings []Suggestion

	for _, rule := range rules {
		if rule.Stack == "generic" || rule.Stack == stack {
			if msg := rule.Check(lines); msg != "" {
				findings = append(findings, Suggestion{
					Description: msg,
					Severity:    rule.Severity,
				})
			}
		}
	}
	return findings
}
