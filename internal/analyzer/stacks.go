package analyzer

import (
	"strings"

	`github.com/davidgrldo/dockerfile-optimizer/internal/parser`
)

type StackPattern struct {
	Name     string
	Keywords []string
}

var stackPatterns = []StackPattern{
	{Name: "go", Keywords: []string{"golang", "go build"}},
	{Name: "java", Keywords: []string{"openjdk", "java", "maven", "gradle"}},
	{Name: "python", Keywords: []string{"python", "pip install"}},
	{Name: "node", Keywords: []string{"npm install", "node", "yarn"}},
	{Name: "rust", Keywords: []string{"rust", "cargo"}},
	{Name: "dotnet", Keywords: []string{"dotnet", "csproj"}},
	{Name: "php", Keywords: []string{"php", "composer"}},
	{Name: "ruby", Keywords: []string{"ruby", "bundle install"}},
	{Name: "c_cpp", Keywords: []string{"gcc", "g++", "make", "cmake"}},
}

func DetectStack(lines []string) string {
	for _, line := range lines {
		lower := strings.ToLower(line)
		for _, pattern := range stackPatterns {
			for _, keyword := range pattern.Keywords {
				if strings.Contains(lower, keyword) {
					return pattern.Name
				}
			}
		}
	}
	return "generic"
}

func DetectStackFromFile(path string) string {
	lines, err := parser.ParseDockerfile(path)
	if err != nil {
		return "generic"
	}
	return DetectStack(lines)
}
