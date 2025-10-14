package analyzer

import `strings`

type Rule struct {
	Description string
	Stack       string
	Severity    string // "info", "warn", "error"
	Check       func([]string) string
}

var rules = []Rule{
	{
		Description: "Avoid using 'latest' tag in base images",
		Stack:       "generic",
		Severity:    "warn",
		Check: func(lines []string) string {
			for _, l := range lines {
				if strings.Contains(l, "FROM") && strings.Contains(l, ":latest") {
					return "Avoid using 'latest' tag in base images"
				}
			}
			return ""
		},
	},
	{
		Description: "Use slim Java images for smaller and faster containers",
		Stack:       "java",
		Severity:    "info",
		Check: func(lines []string) string {
			for _, l := range lines {
				if l == "FROM openjdk:17" {
					return "Consider using 'openjdk:17-slim' instead"
				}
			}
			return ""
		},
	},
	{
		Description: "Use multi-stage builds for Go to reduce image size",
		Stack:       "go",
		Severity:    "warn",
		Check: func(lines []string) string {
			stageCount := 0
			for _, l := range lines {
				if strings.HasPrefix(strings.ToUpper(l), "FROM") {
					stageCount++
				}
			}
			if stageCount < 2 {
				return "Consider using multi-stage builds in Go to reduce final image size"
			}
			return ""
		},
	},
	{
		Description: "Ensure CGO is disabled when building static Go binaries",
		Stack:       "go",
		Severity:    "error",
		Check: func(lines []string) string {
			var runBuffer string
			for _, line := range lines {
				line = strings.TrimSpace(line)
				if strings.HasPrefix(line, "RUN") {
					runBuffer += " " + line
					if !strings.HasSuffix(line, "\\") {
						lower := strings.ToLower(runBuffer)
						if strings.Contains(lower, "go build") && !strings.Contains(lower, "cgo_enabled=0") {
							return "Set CGO_ENABLED=0 when building static Go binaries (e.g., for Alpine or scratch)"
						}
						runBuffer = ""
					}
				}
			}
			return ""
		},
	},
	{
		Description: "Avoid shipping full Golang build image as final container",
		Stack:       "go",
		Severity:    "warn",
		Check: func(lines []string) string {
			fromCount := 0
			for _, l := range lines {
				if strings.HasPrefix(strings.ToUpper(l), "FROM") {
					fromCount++
					if fromCount == 2 && strings.Contains(l, "golang") {
						return "Avoid using golang image in final stage; copy binary to scratch/distroless/alpine"
					}
				}
			}
			return ""
		},
	},
	{
		Description: "Consider using multi-stage builds for Rust to separate build and runtime",
		Stack:       "rust",
		Severity:    "warn",
		Check: func(lines []string) string {
			stageCount := 0
			for _, l := range lines {
				if strings.HasPrefix(strings.ToUpper(l), "FROM") {
					stageCount++
				}
			}
			if stageCount < 2 {
				return "Consider using multi-stage builds in Rust to reduce final image size"
			}
			return ""
		},
	},
	{
		Description: "Use specific SDK/runtime tags (not 'latest') in .NET base images",
		Stack:       "dotnet",
		Severity:    "warn",
		Check: func(lines []string) string {
			for _, l := range lines {
				if strings.Contains(l, "mcr.microsoft.com/dotnet") && strings.Contains(l, ":latest") {
					return "Avoid using 'latest' in .NET base images. Pin to a version like ':7.0-sdk'"
				}
			}
			return ""
		},
	},
	{
		Description: "Install PHP dependencies using composer with --no-dev and --optimize-autoloader flags",
		Stack:       "php",
		Severity:    "warn",
		Check: func(lines []string) string {
			for _, l := range lines {
				if strings.Contains(l, "composer install") && !strings.Contains(l, "--no-dev") {
					return "Use 'composer install --no-dev --optimize-autoloader' for production PHP builds"
				}
			}
			return ""
		},
	},
	{
		Description: "Use '--deployment' flag with 'bundle install' for production",
		Stack:       "ruby",
		Severity:    "info",
		Check: func(lines []string) string {
			for _, l := range lines {
				if strings.Contains(l, "bundle install") && !strings.Contains(l, "--deployment") {
					return "Use 'bundle install --deployment' for better performance in Ruby production builds"
				}
			}
			return ""
		},
	},
}
