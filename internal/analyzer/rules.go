package analyzer

import (
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/davidgrldo/dockerfile-optimizer/internal/dockerfile"
)

type ruleCheck func(*dockerfile.Document) []Finding

var (
	goBuildPattern     = regexp.MustCompile(`(?:^|[^A-Za-z0-9_])go[[:space:]]+build(?:$|[^A-Za-z0-9_])`)
	cgoDisabledPattern = regexp.MustCompile(`(?:^|[^A-Za-z0-9_])CGO_ENABLED[[:space:]]*=[[:space:]]*0(?:$|[^A-Za-z0-9_])`)
	secretKeyPattern   = regexp.MustCompile(`(?i)(^|[_-])(password|secret|token|api[_-]?key|private[_-]?key)([_-]|$)`)
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
	{"GEN006", SeverityWarn, []Stack{StackGeneric}, checkApkNoCache},
	{"GEN007", SeverityWarn, []Stack{StackGeneric}, checkRpmCacheCleanup},
	{"GEN008", SeverityWarn, []Stack{StackGeneric}, checkSecretBuildArgs},
	{"GEN009", SeverityWarn, []Stack{StackGeneric}, checkCacheMount},
	{"GEN010", SeverityInfo, []Stack{StackGeneric}, checkDigestPin},
	{"GO001", SeverityWarn, []Stack{StackGo}, checkGoMultistage},
	{"GO002", SeverityError, []Stack{StackGo}, checkGoStaticBuild},
	{"GO003", SeverityWarn, []Stack{StackGo}, checkGoFinalImage},
	{"GO004", SeverityWarn, []Stack{StackGo}, checkGoCopyOrder},
	{"JAVA001", SeverityInfo, []Stack{StackJava}, checkJavaRuntime},
	{"RUST001", SeverityWarn, []Stack{StackRust}, checkRustMultistage},
	{"RUST002", SeverityWarn, []Stack{StackRust}, checkRustCopyOrder},
	{"RUST003", SeverityWarn, []Stack{StackRust}, checkRustRelease},
	{"RUST004", SeverityWarn, []Stack{StackRust}, checkRustFinalImage},
	{"DOTNET001", SeverityWarn, []Stack{StackDotNet}, checkDotNetTag},
	{"DOTNET002", SeverityWarn, []Stack{StackDotNet}, checkDotNetSDKFinal},
	{"PHP001", SeverityWarn, []Stack{StackPHP}, checkComposerFlag("--no-dev", "Use 'composer install --no-dev' for production PHP builds")},
	{"PHP002", SeverityWarn, []Stack{StackPHP}, checkComposerFlag("--optimize-autoloader", "Use 'composer install --optimize-autoloader' for production PHP builds")},
	{"RUBY001", SeverityInfo, []Stack{StackRuby}, checkRubyDeployment},
	{"PY001", SeverityWarn, []Stack{StackPython}, checkPipNoCache},
	{"PY002", SeverityWarn, []Stack{StackPython}, checkPipCopyOrder},
	{"NODE001", SeverityWarn, []Stack{StackNode}, checkNpmCi},
	{"NODE002", SeverityWarn, []Stack{StackNode}, checkNpmCopyOrder},
	{"NODE003", SeverityWarn, []Stack{StackNode}, checkYarnPnpmFrozen},
	{"NODE004", SeverityWarn, []Stack{StackNode}, checkNodeEnvProduction},
	{"CCPP001", SeverityWarn, []Stack{StackCCPP}, checkCCPPFinalImage},
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
		if isNonRootBaseImage(stage.BaseImage) {
			return nil
		}
		return []Finding{finding("Final stage has no USER; the image will run as root", stage.From, stage.Index)}
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
	if !stageRunsSequence(stage, "go build", "go test", "go install") {
		return nil
	}
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
	if len(doc.Stages) == 0 {
		return nil
	}
	stage := doc.Stages[len(doc.Stages)-1]
	if isFatJavaImage(strings.ToLower(stage.BaseImage)) {
		return []Finding{finding("Use a slim or JRE Java base image (e.g. '-slim' or '-jre') to reduce image size", stage.From, stage.Index)}
	}
	return nil
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
	if !stageRunsSequence(stage, "cargo build", "cargo test", "rustc") {
		return nil
	}
	return []Finding{finding("Consider using multi-stage builds in Rust to reduce final image size", stage.From, stage.Index)}
}

func checkRustCopyOrder(doc *dockerfile.Document) []Finding {
	return checkInstallCopyOrder(doc, []string{"cargo.toml", "cargo.lock"}, func(value string) bool {
		return containsCommandSequence(value, "cargo fetch") || containsCommandSequence(value, "cargo build") || containsCommandSequence(value, "cargo test")
	}, "Copy Cargo.toml and Cargo.lock before COPY . so dependency layers stay cached")
}

func checkRustRelease(doc *dockerfile.Document) []Finding {
	var findings []Finding
	for _, stage := range doc.Stages {
		for _, instruction := range stage.Instructions {
			if instruction.Opcode != "RUN" || !containsCommandSequence(instruction.Value, "cargo build") {
				continue
			}
			if hasToken(instruction.Value, "--release") {
				continue
			}
			findings = append(findings, finding("Add '--release' to 'cargo build' for production binaries", instruction, stage.Index))
		}
	}
	return findings
}

func checkRustFinalImage(doc *dockerfile.Document) []Finding {
	if len(doc.Stages) == 0 {
		return nil
	}
	stage := doc.Stages[len(doc.Stages)-1]
	if !strings.EqualFold(imageRepository(stage.BaseImage), "rust") {
		return nil
	}
	tag := strings.ToLower(imageTag(stage.BaseImage))
	if tag != "" && (strings.Contains(tag, "slim") || strings.Contains(tag, "alpine") || strings.Contains(tag, "distroless")) {
		return nil
	}
	return []Finding{finding("Avoid using a full rust image in the final stage; copy the binary to a slim/alpine runtime", stage.From, stage.Index)}
}

func checkDotNetSDKFinal(doc *dockerfile.Document) []Finding {
	if len(doc.Stages) == 0 {
		return nil
	}
	stage := doc.Stages[len(doc.Stages)-1]
	if !slices.Contains(imageRepositoryComponents(stage.BaseImage), "sdk") {
		return nil
	}
	return []Finding{finding("Avoid using the .NET SDK image in the final stage; copy the app into aspnet or runtime", stage.From, stage.Index)}
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

func checkApkNoCache(doc *dockerfile.Document) []Finding {
	var findings []Finding
	for _, stage := range doc.Stages {
		for _, instruction := range stage.Instructions {
			if instruction.Opcode == "RUN" && containsCommandSequence(instruction.Value, "apk add") && !hasToken(instruction.Value, "--no-cache") {
				findings = append(findings, finding("Add '--no-cache' to 'apk add' to avoid storing the apk index in the layer", instruction, stage.Index))
			}
		}
	}
	return findings
}

func checkRpmCacheCleanup(doc *dockerfile.Document) []Finding {
	var findings []Finding
	managers := []struct {
		install string
		clean   string
		cache   string
		name    string
	}{
		{"yum install", "yum clean", "/var/cache/yum", "yum"},
		{"dnf install", "dnf clean", "/var/cache/dnf", "dnf"},
		{"microdnf install", "microdnf clean", "/var/cache/dnf", "microdnf"},
	}
	for _, stage := range doc.Stages {
		for _, instruction := range stage.Instructions {
			if instruction.Opcode != "RUN" {
				continue
			}
			for _, manager := range managers {
				if !containsCommandSequence(instruction.Value, manager.install) {
					continue
				}
				if containsCommandSequence(instruction.Value, manager.clean) || strings.Contains(instruction.Value, manager.cache) {
					continue
				}
				findings = append(findings, finding("Clean the "+manager.name+" cache in the same RUN (e.g. '"+manager.name+" clean all') to keep the layer small", instruction, stage.Index))
			}
		}
	}
	return findings
}

func checkPipNoCache(doc *dockerfile.Document) []Finding {
	var findings []Finding
	for _, stage := range doc.Stages {
		for _, instruction := range stage.Instructions {
			if instruction.Opcode != "RUN" {
				continue
			}
			if !containsCommandSequence(instruction.Value, "pip install") && !containsCommandSequence(instruction.Value, "pip3 install") {
				continue
			}
			if hasToken(instruction.Value, "--no-cache-dir") {
				continue
			}
			findings = append(findings, finding("Add '--no-cache-dir' to 'pip install' to keep pip caches out of the image", instruction, stage.Index))
		}
	}
	return findings
}

func checkNpmCi(doc *dockerfile.Document) []Finding {
	var findings []Finding
	for _, stage := range doc.Stages {
		for _, instruction := range stage.Instructions {
			if instruction.Opcode == "RUN" && containsCommandSequence(instruction.Value, "npm install") && !containsCommandSequence(instruction.Value, "npm ci") {
				findings = append(findings, finding("Prefer 'npm ci' over 'npm install' for reproducible production installs", instruction, stage.Index))
			}
		}
	}
	return findings
}

func checkPipCopyOrder(doc *dockerfile.Document) []Finding {
	return checkInstallCopyOrder(doc, []string{"requirements.txt"}, func(value string) bool {
		return containsCommandSequence(value, "pip install") || containsCommandSequence(value, "pip3 install")
	}, "Copy the requirements file before COPY . so dependency layers stay cached")
}

func checkNpmCopyOrder(doc *dockerfile.Document) []Finding {
	return checkInstallCopyOrder(doc, []string{"package.json", "package-lock.json", "yarn.lock", "pnpm-lock.yaml", "npm-shrinkwrap.json"}, func(value string) bool {
		return containsCommandSequence(value, "npm install") || containsCommandSequence(value, "npm ci") || isYarnInstall(value) || containsCommandSequence(value, "pnpm install")
	}, "Copy package.json and the lockfile before COPY . so dependency layers stay cached")
}

func checkGoCopyOrder(doc *dockerfile.Document) []Finding {
	return checkInstallCopyOrder(doc, []string{"go.mod", "go.sum", "go.work"}, func(value string) bool {
		return containsCommandSequence(value, "go mod download") || containsCommandSequence(value, "go mod tidy") || containsCommandSequence(value, "go build") || containsCommandSequence(value, "go install") || containsCommandSequence(value, "go test")
	}, "Copy go.mod and go.sum before COPY . so module layers stay cached")
}

func checkYarnPnpmFrozen(doc *dockerfile.Document) []Finding {
	var findings []Finding
	for _, stage := range doc.Stages {
		for _, instruction := range stage.Instructions {
			if instruction.Opcode != "RUN" {
				continue
			}
			switch {
			case isYarnInstall(instruction.Value):
				if hasToken(instruction.Value, "--frozen-lockfile") || hasToken(instruction.Value, "--immutable") {
					continue
				}
				findings = append(findings, finding("Use 'yarn install --frozen-lockfile' or '--immutable' for reproducible installs", instruction, stage.Index))
			case containsCommandSequence(instruction.Value, "pnpm install"):
				if hasToken(instruction.Value, "--frozen-lockfile") {
					continue
				}
				findings = append(findings, finding("Add '--frozen-lockfile' to 'pnpm install' for reproducible installs", instruction, stage.Index))
			}
		}
	}
	return findings
}

func checkNodeEnvProduction(doc *dockerfile.Document) []Finding {
	var findings []Finding
	for _, stage := range doc.Stages {
		if stageSetsNodeEnvProduction(stage) {
			continue
		}
		for _, instruction := range stage.Instructions {
			if instruction.Opcode != "RUN" {
				continue
			}
			if !isNodePackageInstall(instruction.Value) {
				continue
			}
			if strings.Contains(instruction.Value, "NODE_ENV=production") {
				continue
			}
			findings = append(findings, finding("Set NODE_ENV=production before installing Node.js dependencies", instruction, stage.Index))
		}
	}
	return findings
}

func checkCacheMount(doc *dockerfile.Document) []Finding {
	var findings []Finding
	for _, stage := range doc.Stages {
		for _, instruction := range stage.Instructions {
			if instruction.Opcode != "RUN" || instruction.JSON || hasCacheMount(instruction) {
				continue
			}
			if message, ok := cacheMountMessage(instruction.Value); ok {
				findings = append(findings, finding(message, instruction, stage.Index))
			}
		}
	}
	return findings
}

func checkDigestPin(doc *dockerfile.Document) []Finding {
	stageNames := map[string]bool{}
	var findings []Finding
	for _, stage := range doc.Stages {
		image := stage.BaseImage
		lower := strings.ToLower(image)
		skip := strings.Contains(image, "${") || strings.Contains(image, "$") || lower == "scratch" || stageNames[lower] || strings.Contains(image, "@") || !hasImageTag(image)
		if !skip {
			findings = append(findings, finding("Pin the base image digest (@sha256:...) in addition to the tag", stage.From, stage.Index))
		}
		if stage.Name != "" {
			stageNames[strings.ToLower(stage.Name)] = true
		}
	}
	return findings
}

func checkInstallCopyOrder(doc *dockerfile.Document, lockFiles []string, isInstall func(string) bool, message string) []Finding {
	var findings []Finding
	for _, stage := range doc.Stages {
		seenLock := false
		var broad *dockerfile.Instruction
		for _, instruction := range stage.Instructions {
			switch instruction.Opcode {
			case "COPY":
				if copyUsesStage(instruction.Value) {
					continue
				}
				sources := copySources(instruction.Value)
				if copyIncludes(sources, lockFiles) {
					seenLock = true
				}
				if isBroadContextCopy(sources) && !seenLock {
					inst := instruction
					broad = &inst
				}
			case "RUN":
				if isInstall(instruction.Value) && broad != nil {
					findings = append(findings, finding(message, *broad, stage.Index))
					broad = nil
				}
			}
		}
	}
	return findings
}

func copyUsesStage(value string) bool {
	for _, field := range strings.Fields(value) {
		if strings.HasPrefix(strings.ToLower(field), "--from=") {
			return true
		}
	}
	return false
}

func checkSecretBuildArgs(doc *dockerfile.Document) []Finding {
	var findings []Finding
	for _, stage := range doc.Stages {
		for _, instruction := range stage.Instructions {
			if instruction.Opcode != "ARG" && instruction.Opcode != "ENV" {
				continue
			}
			for _, key := range assignmentKeys(instruction.Value) {
				if secretKeyPattern.MatchString(key) {
					findings = append(findings, finding("Do not pass secrets via ARG/ENV; use a secret mount or build-time secret", instruction, stage.Index))
					break
				}
			}
		}
	}
	return findings
}

func copySources(value string) []string {
	fields := strings.Fields(value)
	var parts []string
	for _, field := range fields {
		if strings.HasPrefix(field, "--") {
			continue
		}
		parts = append(parts, strings.Trim(field, `"'`))
	}
	if len(parts) < 2 {
		return nil
	}
	return parts[:len(parts)-1]
}

func isBroadContextCopy(sources []string) bool {
	for _, source := range sources {
		if source == "." || source == "./" {
			return true
		}
	}
	return false
}

func copyIncludes(sources, names []string) bool {
	for _, source := range sources {
		base := strings.ToLower(filepath.Base(source))
		for _, name := range names {
			if base == name {
				return true
			}
			if name == "requirements.txt" && strings.HasPrefix(base, "requirements") && strings.HasSuffix(base, ".txt") {
				return true
			}
		}
	}
	return false
}

func assignmentKeys(value string) []string {
	var keys []string
	for _, field := range strings.Fields(value) {
		key, _, _ := strings.Cut(field, "=")
		if key != "" {
			keys = append(keys, key)
		}
	}
	return keys
}

func checkCCPPFinalImage(doc *dockerfile.Document) []Finding {
	if len(doc.Stages) == 0 {
		return nil
	}
	stage := doc.Stages[len(doc.Stages)-1]
	repo := imageRepository(stage.BaseImage)
	if repo != "gcc" && repo != "g++" {
		return nil
	}
	return []Finding{finding("Avoid using a compiler image in the final stage; copy the binary to a slim runtime", stage.From, stage.Index)}
}

func isNonRootBaseImage(image string) bool {
	return strings.Contains(strings.ToLower(image), "nonroot")
}

func stageRunsSequence(stage dockerfile.Stage, sequences ...string) bool {
	for _, instruction := range stage.Instructions {
		if instruction.Opcode != "RUN" {
			continue
		}
		for _, sequence := range sequences {
			if containsCommandSequence(instruction.Value, sequence) {
				return true
			}
		}
	}
	return false
}

func hasCacheMount(instruction dockerfile.Instruction) bool {
	for _, flag := range instruction.Flags {
		if strings.Contains(strings.ToLower(flag), "type=cache") {
			return true
		}
	}
	return false
}

func cacheMountMessage(value string) (string, bool) {
	switch {
	case containsCommandSequence(value, "apt-get install"):
		return "Use RUN --mount=type=cache,target=/var/cache/apt,sharing=locked for apt-get installs", true
	case containsCommandSequence(value, "go build"), containsCommandSequence(value, "go test"), containsCommandSequence(value, "go install"), containsCommandSequence(value, "go mod download"):
		return "Use RUN --mount=type=cache,target=/go/pkg/mod and target=/root/.cache/go-build for Go commands", true
	case containsCommandSequence(value, "npm ci"), containsCommandSequence(value, "npm install"), isYarnInstall(value), containsCommandSequence(value, "pnpm install"):
		return "Use RUN --mount=type=cache,target=/root/.npm (or the yarn/pnpm store) for Node installs", true
	case containsCommandSequence(value, "pip install"), containsCommandSequence(value, "pip3 install"):
		return "Use RUN --mount=type=cache,target=/root/.cache/pip for pip installs", true
	case containsCommandSequence(value, "cargo build"), containsCommandSequence(value, "cargo fetch"):
		return "Use RUN --mount=type=cache,target=/usr/local/cargo/registry for Cargo commands", true
	default:
		return "", false
	}
}

func isYarnInstall(value string) bool {
	if containsCommandSequence(value, "yarn install") {
		return true
	}
	fields := strings.Fields(strings.ToLower(value))
	for i, field := range fields {
		if commandToken(field) != "yarn" {
			continue
		}
		next := nextNonFlagToken(fields[i+1:])
		if next == "" || next == "install" {
			return true
		}
	}
	return false
}

func nextNonFlagToken(fields []string) string {
	for _, field := range fields {
		token := commandToken(field)
		if token == "" || strings.HasPrefix(token, "-") {
			continue
		}
		return token
	}
	return ""
}

func isNodePackageInstall(value string) bool {
	return containsCommandSequence(value, "npm ci") || containsCommandSequence(value, "npm install") || isYarnInstall(value) || containsCommandSequence(value, "pnpm install")
}

func stageSetsNodeEnvProduction(stage dockerfile.Stage) bool {
	for _, instruction := range stage.Instructions {
		if instruction.Opcode != "ENV" && instruction.Opcode != "ARG" {
			continue
		}
		if strings.Contains(instruction.Value, "NODE_ENV=production") {
			return true
		}
	}
	return false
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
