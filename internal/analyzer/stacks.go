package analyzer

import (
	"fmt"
	"strings"

	"github.com/davidgrldo/dockerfile-optimizer/internal/dockerfile"
)

type Stack string

const (
	StackGeneric Stack = "generic"
	StackGo      Stack = "go"
	StackJava    Stack = "java"
	StackPython  Stack = "python"
	StackNode    Stack = "node"
	StackRust    Stack = "rust"
	StackDotNet  Stack = "dotnet"
	StackPHP     Stack = "php"
	StackRuby    Stack = "ruby"
	StackCCPP    Stack = "c_cpp"
)

type stackDefinition struct {
	stack     Stack
	keywords  []string
	supported bool
}

var stackRegistry = []stackDefinition{
	{stack: StackGeneric, supported: true},
	{stack: StackGo, keywords: []string{"golang", "go build"}, supported: true},
	{stack: StackJava, keywords: []string{"openjdk", "java", "maven", "gradle"}, supported: true},
	{stack: StackPython, keywords: []string{"python", "pip install"}},
	{stack: StackNode, keywords: []string{"npm install", "node", "yarn"}},
	{stack: StackRust, keywords: []string{"rust", "cargo"}, supported: true},
	{stack: StackDotNet, keywords: []string{"dotnet", "csproj"}, supported: true},
	{stack: StackPHP, keywords: []string{"php", "composer"}, supported: true},
	{stack: StackRuby, keywords: []string{"ruby", "bundle install"}, supported: true},
	{stack: StackCCPP, keywords: []string{"gcc", "g++", "make", "cmake"}},
}

func ParseStack(value string) (Stack, error) {
	stack := Stack(value)
	if _, ok := lookupStack(stack); !ok {
		return "", fmt.Errorf("invalid stack %q", value)
	}
	return stack, nil
}

func IsSupported(stack Stack) bool {
	definition, ok := lookupStack(stack)
	return ok && definition.supported
}

func DetectStack(doc *dockerfile.Document) Stack {
	for _, stage := range doc.Stages {
		if stack, ok := detectStack(stage.BaseImage); ok {
			return stack
		}
	}
	for _, instruction := range doc.Instructions {
		if instruction.Opcode != "RUN" {
			continue
		}
		if stack, ok := detectStack(instruction.Value); ok {
			return stack
		}
	}
	return StackGeneric
}

func detectStack(value string) (Stack, bool) {
	for _, definition := range stackRegistry[1:] {
		if containsKeyword(value, definition.keywords) {
			return definition.stack, true
		}
	}
	return "", false
}

func lookupStack(stack Stack) (stackDefinition, bool) {
	for _, definition := range stackRegistry {
		if definition.stack == stack {
			return definition, true
		}
	}
	return stackDefinition{}, false
}

func containsKeyword(value string, keywords []string) bool {
	value = strings.ToLower(value)
	for _, keyword := range keywords {
		if strings.Contains(value, keyword) {
			return true
		}
	}
	return false
}
