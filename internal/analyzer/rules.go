package analyzer

import (
	"regexp"
	"strings"

	"github.com/davidgrldo/dockerfile-optimizer/internal/dockerfile"
)

type ruleCheck func(*dockerfile.Document, Context) []Finding

var (
	goBuildPattern     = regexp.MustCompile(`(?:^|[^A-Za-z0-9_])go[[:space:]]+build(?:$|[^A-Za-z0-9_])`)
	cgoDisabledPattern = regexp.MustCompile(`(?:^|[^A-Za-z0-9_])CGO_ENABLED[[:space:]]*=[[:space:]]*0(?:$|[^A-Za-z0-9_])`)
)

type rule struct {
	id       string
	severity Severity
	stacks   []Stack
	check    ruleCheck
}

func (r rule) ID() string         { return r.id }
func (r rule) Severity() Severity { return r.severity }
func (r rule) Stacks() []Stack    { return append([]Stack(nil), r.stacks...) }

func (r rule) Evaluate(doc *dockerfile.Document, context Context) []Finding {
	findings := r.check(doc, context)
	for i := range findings {
		findings[i].ID = r.id
		findings[i].Severity = r.severity
	}
	return findings
}

func newRule(id string, severity Severity, stacks []Stack, check ruleCheck) Rule {
	return rule{id: id, severity: severity, stacks: append([]Stack(nil), stacks...), check: check}
}

var registeredRules = []Rule{
	newRule("GEN001", SeverityWarn, []Stack{StackGeneric}, checkLatestBase),
	newRule("GO001", SeverityWarn, []Stack{StackGo}, checkGoMultistage),
	newRule("GO002", SeverityError, []Stack{StackGo}, checkGoStaticBuild),
	newRule("GO003", SeverityWarn, []Stack{StackGo}, checkGoFinalImage),
	newRule("JAVA001", SeverityInfo, []Stack{StackJava}, checkJavaRuntime),
	newRule("RUST001", SeverityWarn, []Stack{StackRust}, checkRustMultistage),
	newRule("DOTNET001", SeverityWarn, []Stack{StackDotNet}, checkDotNetTag),
	newRule("PHP001", SeverityWarn, []Stack{StackPHP}, checkComposerFlag("--no-dev", "Use 'composer install --no-dev' for production PHP builds")),
	newRule("PHP002", SeverityWarn, []Stack{StackPHP}, checkComposerFlag("--optimize-autoloader", "Use 'composer install --optimize-autoloader' for production PHP builds")),
	newRule("RUBY001", SeverityInfo, []Stack{StackRuby}, checkRubyDeployment),
}

func Rules() []Rule { return append([]Rule(nil), registeredRules...) }

func checkLatestBase(doc *dockerfile.Document, _ Context) []Finding {
	var findings []Finding
	for _, stage := range doc.Stages {
		if strings.HasSuffix(strings.ToLower(stage.BaseImage), ":latest") {
			findings = append(findings, finding("Avoid using 'latest' tag in base images", stage.From, stage.Index))
		}
	}
	return findings
}

func checkGoMultistage(doc *dockerfile.Document, _ Context) []Finding {
	if len(doc.Stages) != 1 {
		return nil
	}
	stage := doc.Stages[0]
	return []Finding{finding("Consider using multi-stage builds in Go to reduce final image size", stage.From, stage.Index)}
}

func checkGoStaticBuild(doc *dockerfile.Document, _ Context) []Finding {
	if len(doc.Stages) < 2 || !strings.EqualFold(doc.Stages[len(doc.Stages)-1].BaseImage, "scratch") {
		return nil
	}

	var findings []Finding
	for _, stage := range doc.Stages[:len(doc.Stages)-1] {
		for _, instruction := range stage.Instructions {
			if instruction.Opcode == "RUN" && goBuildPattern.MatchString(instruction.Value) && !cgoDisabledPattern.MatchString(instruction.Value) {
				findings = append(findings, finding("Set CGO_ENABLED=0 when building static Go binaries for scratch", instruction, stage.Index))
			}
		}
	}
	return findings
}

func checkGoFinalImage(doc *dockerfile.Document, _ Context) []Finding {
	if len(doc.Stages) == 0 {
		return nil
	}
	stage := doc.Stages[len(doc.Stages)-1]
	if !strings.EqualFold(imageRepository(stage.BaseImage), "golang") {
		return nil
	}
	return []Finding{finding("Avoid using golang image in final stage; copy binary to scratch/distroless/alpine", stage.From, stage.Index)}
}

func checkJavaRuntime(doc *dockerfile.Document, _ Context) []Finding {
	var findings []Finding
	for _, stage := range doc.Stages {
		if strings.EqualFold(stage.BaseImage, "openjdk:17") {
			findings = append(findings, finding("Consider using 'openjdk:17-slim' instead", stage.From, stage.Index))
		}
	}
	return findings
}

func checkRustMultistage(doc *dockerfile.Document, _ Context) []Finding {
	if len(doc.Stages) != 1 {
		return nil
	}
	stage := doc.Stages[0]
	return []Finding{finding("Consider using multi-stage builds in Rust to reduce final image size", stage.From, stage.Index)}
}

func checkDotNetTag(doc *dockerfile.Document, _ Context) []Finding {
	var findings []Finding
	for _, stage := range doc.Stages {
		base := strings.ToLower(stage.BaseImage)
		if strings.HasPrefix(base, "mcr.microsoft.com/dotnet/") && !hasImageTag(base) {
			findings = append(findings, finding("Use a specific tag for .NET base images", stage.From, stage.Index))
		}
	}
	return findings
}

func checkComposerFlag(flag, message string) ruleCheck {
	return func(doc *dockerfile.Document, _ Context) []Finding {
		var findings []Finding
		for _, stage := range doc.Stages {
			for _, instruction := range stage.Instructions {
				if instruction.Opcode == "RUN" && strings.Contains(strings.ToLower(instruction.Value), "composer install") && !hasToken(instruction.Value, flag) {
					findings = append(findings, finding(message, instruction, stage.Index))
				}
			}
		}
		return findings
	}
}

func checkRubyDeployment(doc *dockerfile.Document, _ Context) []Finding {
	var findings []Finding
	for _, stage := range doc.Stages {
		for _, instruction := range stage.Instructions {
			value := strings.ToLower(instruction.Value)
			if instruction.Opcode == "RUN" && strings.Contains(value, "bundle install") && !strings.Contains(value, "--deployment") {
				findings = append(findings, finding("Use 'bundle install --deployment' for better performance in Ruby production builds", instruction, stage.Index))
			}
		}
	}
	return findings
}

func finding(message string, instruction dockerfile.Instruction, stage int) Finding {
	return Finding{Message: message, Range: instruction.Range, Stage: &stage}
}

func hasImageTag(image string) bool {
	name, _, _ := strings.Cut(image, "@")
	lastComponent := name[strings.LastIndex(name, "/")+1:]
	return strings.Contains(lastComponent, ":")
}

func imageRepository(image string) string {
	name, _, _ := strings.Cut(image, "@")
	lastComponent := name[strings.LastIndex(name, "/")+1:]
	repository, _, _ := strings.Cut(lastComponent, ":")
	return repository
}

func hasToken(value, token string) bool {
	for _, field := range strings.Fields(value) {
		if field == token {
			return true
		}
	}
	return false
}
