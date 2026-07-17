package analyzer

import (
	"regexp"
	"slices"
	"strings"

	"github.com/davidgrldo/dockerfile-optimizer/internal/dockerfile"
)

type ruleCheck func(*dockerfile.Document) []Finding

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

func (r rule) appliesTo(selected Stack) bool {
	for _, stack := range r.stacks {
		if stack == StackGeneric || stack == selected {
			return true
		}
	}
	return false
}

var registeredRules = []rule{
	{"GEN001", SeverityWarn, []Stack{StackGeneric}, checkLatestBase},
	{"GEN002", SeverityWarn, []Stack{StackGeneric}, checkAptNoRecommends},
	{"GEN003", SeverityWarn, []Stack{StackGeneric}, checkAptCacheCleanup},
	{"GEN004", SeverityWarn, []Stack{StackGeneric}, checkAddRemoteURL},
	{"GEN005", SeverityWarn, []Stack{StackGeneric}, checkFinalUserRoot},
	{"GO001", SeverityWarn, []Stack{StackGo}, checkGoMultistage},
	{"GO002", SeverityError, []Stack{StackGo}, checkGoStaticBuild},
	{"GO003", SeverityWarn, []Stack{StackGo}, checkGoFinalImage},
	{"JAVA001", SeverityInfo, []Stack{StackJava}, checkJavaRuntime},
	{"RUST001", SeverityWarn, []Stack{StackRust}, checkRustMultistage},
	{"DOTNET001", SeverityWarn, []Stack{StackDotNet}, checkDotNetTag},
	{"PHP001", SeverityWarn, []Stack{StackPHP}, checkComposerFlag("--no-dev", "Use 'composer install --no-dev' for production PHP builds")},
	{"PHP002", SeverityWarn, []Stack{StackPHP}, checkComposerFlag("--optimize-autoloader", "Use 'composer install --optimize-autoloader' for production PHP builds")},
	{"RUBY001", SeverityInfo, []Stack{StackRuby}, checkRubyDeployment},
}

func checkLatestBase(doc *dockerfile.Document) []Finding {
	stageNames := map[string]bool{}
	var findings []Finding
	for _, stage := range doc.Stages {
		if message, ok := latestBaseFinding(strings.ToLower(stage.BaseImage), stageNames); ok {
			findings = append(findings, finding(message, stage.From, stage.Index))
		}
		if stage.Name != "" {
			stageNames[strings.ToLower(stage.Name)] = true
		}
	}
	return findings
}

// latestBaseFinding reports whether a (lowercased) base image resolves to the
// mutable 'latest' tag, either explicitly or by omitting a tag. Prior build
// stages, scratch, and digest-pinned images are never flagged.
func latestBaseFinding(image string, stageNames map[string]bool) (string, bool) {
	if strings.HasSuffix(image, ":latest") {
		return "Avoid using 'latest' tag in base images", true
	}
	if image == "" || image == "scratch" || stageNames[image] || strings.Contains(image, "@") {
		return "", false
	}
	if !hasImageTag(image) {
		return "Pin an explicit tag; untagged base images default to 'latest'", true
	}
	return "", false
}

func checkAptNoRecommends(doc *dockerfile.Document) []Finding {
	var findings []Finding
	for _, stage := range doc.Stages {
		for _, instruction := range stage.Instructions {
			// ponytail: matches the common `apt-get install ...` form; the rarer
			// `apt-get -y install` (flag before the subcommand) is not detected.
			if instruction.Opcode == "RUN" && containsCommandSequence(instruction.Value, "apt-get install") && !hasToken(instruction.Value, "--no-install-recommends") {
				findings = append(findings, finding("Add '--no-install-recommends' to 'apt-get install' to avoid pulling optional packages", instruction, stage.Index))
			}
		}
	}
	return findings
}

func checkAptCacheCleanup(doc *dockerfile.Document) []Finding {
	var findings []Finding
	for _, stage := range doc.Stages {
		for _, instruction := range stage.Instructions {
			if instruction.Opcode == "RUN" && containsCommandSequence(instruction.Value, "apt-get install") && !strings.Contains(instruction.Value, "/var/lib/apt/lists") {
				findings = append(findings, finding("Remove the apt cache in the same RUN (rm -rf /var/lib/apt/lists/*) to keep the layer small", instruction, stage.Index))
			}
		}
	}
	return findings
}

func checkAddRemoteURL(doc *dockerfile.Document) []Finding {
	var findings []Finding
	for _, stage := range doc.Stages {
		for _, instruction := range stage.Instructions {
			if instruction.Opcode != "ADD" {
				continue
			}
			for _, field := range strings.Fields(instruction.Value) {
				token := strings.ToLower(strings.Trim(field, `[]"',`))
				if strings.HasPrefix(token, "http://") || strings.HasPrefix(token, "https://") {
					findings = append(findings, finding("Avoid 'ADD <url>'; use 'RUN curl/wget' with a checksum or COPY instead", instruction, stage.Index))
					break
				}
			}
		}
	}
	return findings
}

func checkFinalUserRoot(doc *dockerfile.Document) []Finding {
	if len(doc.Stages) == 0 {
		return nil
	}
	stage := doc.Stages[len(doc.Stages)-1]
	var lastUser *dockerfile.Instruction
	for i := range stage.Instructions {
		if stage.Instructions[i].Opcode == "USER" {
			lastUser = &stage.Instructions[i]
		}
	}
	if lastUser == nil {
		return nil
	}
	fields := strings.Fields(lastUser.Value)
	if len(fields) == 0 {
		return nil
	}
	name, _, _ := strings.Cut(fields[0], ":")
	if name == "root" || name == "0" {
		return []Finding{finding("Final stage runs as root; set a non-root USER for the runtime", *lastUser, stage.Index)}
	}
	return nil
}

func checkGoMultistage(doc *dockerfile.Document) []Finding {
	if len(doc.Stages) != 1 {
		return nil
	}
	stage := doc.Stages[0]
	return []Finding{finding("Consider using multi-stage builds in Go to reduce final image size", stage.From, stage.Index)}
}

func checkGoStaticBuild(doc *dockerfile.Document) []Finding {
	if len(doc.Stages) < 2 || !strings.EqualFold(doc.Stages[len(doc.Stages)-1].BaseImage, "scratch") {
		return nil
	}

	var findings []Finding
	for _, stage := range doc.Stages[:len(doc.Stages)-1] {
		// ponytail: treats CGO_ENABLED=0 from a stage-level ENV/ARG (the common
		// form) or inline in the RUN as static; does not track a later re-enable.
		cgoDisabled := stageDisablesCGO(stage)
		for _, instruction := range stage.Instructions {
			if instruction.Opcode != "RUN" || !goBuildPattern.MatchString(instruction.Value) {
				continue
			}
			if cgoDisabled || cgoDisabledPattern.MatchString(instruction.Value) {
				continue
			}
			findings = append(findings, finding("Set CGO_ENABLED=0 when building static Go binaries for scratch", instruction, stage.Index))
		}
	}
	return findings
}

func stageDisablesCGO(stage dockerfile.Stage) bool {
	for _, instruction := range stage.Instructions {
		if (instruction.Opcode == "ENV" || instruction.Opcode == "ARG") && cgoDisabledPattern.MatchString(instruction.Value) {
			return true
		}
	}
	return false
}

func checkGoFinalImage(doc *dockerfile.Document) []Finding {
	if len(doc.Stages) == 0 {
		return nil
	}
	stage := doc.Stages[len(doc.Stages)-1]
	if !strings.EqualFold(imageRepository(stage.BaseImage), "golang") {
		return nil
	}
	return []Finding{finding("Avoid using golang image in final stage; copy binary to scratch/distroless/alpine", stage.From, stage.Index)}
}

var (
	javaJDKRepos   = []string{"openjdk", "eclipse-temurin", "amazoncorretto"}
	javaSlimMarker = []string{"slim", "jre", "alpine", "jlink", "distroless"}
)

func checkJavaRuntime(doc *dockerfile.Document) []Finding {
	var findings []Finding
	for _, stage := range doc.Stages {
		if isFatJavaImage(strings.ToLower(stage.BaseImage)) {
			findings = append(findings, finding("Use a slim or JRE Java base image (e.g. '-slim' or '-jre') to reduce image size", stage.From, stage.Index))
		}
	}
	return findings
}

// isFatJavaImage reports whether a (lowercased) base image is a full JDK image
// whose tag does not already select a slim/JRE variant. Untagged images are
// left to GEN001.
func isFatJavaImage(image string) bool {
	if !slices.Contains(javaJDKRepos, imageRepository(image)) {
		return false
	}
	tag := imageTag(image)
	if tag == "" {
		return false
	}
	for _, marker := range javaSlimMarker {
		if strings.Contains(tag, marker) {
			return false
		}
	}
	return true
}

func checkRustMultistage(doc *dockerfile.Document) []Finding {
	if len(doc.Stages) != 1 {
		return nil
	}
	stage := doc.Stages[0]
	return []Finding{finding("Consider using multi-stage builds in Rust to reduce final image size", stage.From, stage.Index)}
}

func checkDotNetTag(doc *dockerfile.Document) []Finding {
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
	return func(doc *dockerfile.Document) []Finding {
		var findings []Finding
		for _, stage := range doc.Stages {
			for _, instruction := range stage.Instructions {
				if instruction.Opcode == "RUN" && containsCommandSequence(instruction.Value, "composer install") && !hasToken(instruction.Value, flag) {
					findings = append(findings, finding(message, instruction, stage.Index))
				}
			}
		}
		return findings
	}
}

func checkRubyDeployment(doc *dockerfile.Document) []Finding {
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

func imageTag(image string) string {
	name, _, _ := strings.Cut(image, "@")
	lastComponent := name[strings.LastIndex(name, "/")+1:]
	_, tag, _ := strings.Cut(lastComponent, ":")
	return tag
}

func hasToken(value, token string) bool {
	for _, field := range strings.Fields(value) {
		if field == token {
			return true
		}
	}
	return false
}
