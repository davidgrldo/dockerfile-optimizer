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
	stack             Stack
	imageRepositories []string
	commandSequences  []string
}

var stackRegistry = []stackDefinition{
	{stack: StackGeneric},
	{stack: StackGo, imageRepositories: []string{"golang"}, commandSequences: []string{"go build"}},
	{stack: StackJava, imageRepositories: []string{"openjdk", "java", "maven", "gradle"}, commandSequences: []string{"java", "maven", "gradle"}},
	{stack: StackPython, imageRepositories: []string{"python"}, commandSequences: []string{"pip install"}},
	{stack: StackNode, imageRepositories: []string{"node"}, commandSequences: []string{"npm install", "npm ci", "yarn"}},
	{stack: StackRust, imageRepositories: []string{"rust"}, commandSequences: []string{"rustc", "cargo"}},
	{stack: StackDotNet, imageRepositories: []string{"dotnet"}, commandSequences: []string{"dotnet", "csproj"}},
	{stack: StackPHP, imageRepositories: []string{"php"}, commandSequences: []string{"composer"}},
	{stack: StackRuby, imageRepositories: []string{"ruby"}, commandSequences: []string{"bundle install"}},
	{stack: StackCCPP, imageRepositories: []string{"gcc", "g++"}, commandSequences: []string{"gcc", "g++", "make", "cmake"}},
}

func ParseStack(value string) (Stack, error) {
	stack := Stack(value)
	if _, ok := lookupStack(stack); !ok {
		return "", fmt.Errorf("invalid stack %q", value)
	}
	return stack, nil
}

func IsSupported(stack Stack) bool {
	_, valid := lookupStack(stack)
	return valid && stack != StackGeneric && len(stackRuleIDs(stack)) > 0
}

func DetectStack(doc *dockerfile.Document) Stack {
	for i := len(doc.Stages) - 1; i >= 0; i-- {
		if stack := DetectStageStack(doc.Stages[i]); stack != StackGeneric {
			return stack
		}
	}
	return StackGeneric
}

// DetectStageStack returns the stack evidenced by a stage's base image, or by
// RUN commands in that stage if the image is unrecognized.
func DetectStageStack(stage dockerfile.Stage) Stack {
	if stack, ok := detectStackFromImage(stage.BaseImage); ok {
		return stack
	}
	for _, instruction := range stage.Instructions {
		if instruction.Opcode != "RUN" {
			continue
		}
		if stack, ok := detectStackFromCommand(instruction.Value); ok {
			return stack
		}
	}
	return StackGeneric
}

func detectStackFromImage(value string) (Stack, bool) {
	components := imageRepositoryComponents(value)
	for _, definition := range stackRegistry[1:] {
		if containsAny(components, definition.imageRepositories) {
			return definition.stack, true
		}
	}
	return "", false
}

func detectStackFromCommand(value string) (Stack, bool) {
	for _, definition := range stackRegistry[1:] {
		for _, sequence := range definition.commandSequences {
			if containsCommandSequence(value, sequence) {
				return definition.stack, true
			}
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

func stackRuleIDs(stack Stack) []string {
	ids := []string{}
	for _, r := range registeredRules {
		for _, assigned := range r.stacks {
			if assigned == stack {
				ids = append(ids, r.id)
				break
			}
		}
	}
	return ids
}

func imageRepositoryComponents(image string) []string {
	name, _, _ := strings.Cut(strings.ToLower(image), "@")
	components := strings.Split(name, "/")
	last := len(components) - 1
	if last >= 0 {
		components[last], _, _ = strings.Cut(components[last], ":")
	}
	return components
}

func containsAny(values, candidates []string) bool {
	for _, value := range values {
		for _, candidate := range candidates {
			if value == candidate {
				return true
			}
		}
	}
	return false
}

func containsCommandSequence(value, sequence string) bool {
	fields := strings.Fields(strings.ToLower(value))
	want := strings.Fields(sequence)
	if len(want) == 0 {
		return false
	}
	for start := 0; start < len(fields); start++ {
		if commandToken(fields[start]) != want[0] {
			continue
		}
		matched := 1
		for i := start + 1; i < len(fields) && matched < len(want); i++ {
			token := commandToken(fields[i])
			if token == want[matched] {
				matched++
				continue
			}
			if strings.HasPrefix(token, "-") {
				continue
			}
			break
		}
		if matched == len(want) {
			return true
		}
	}
	return false
}

func commandToken(field string) string {
	return strings.Trim(field, ";&|(){}'\"")
}
