package analyzer

import (
	"strings"

	"github.com/davidgrldo/dockerfile-optimizer/internal/dockerfile"
)

type Severity string

const (
	SeverityInfo  Severity = "info"
	SeverityWarn  Severity = "warn"
	SeverityError Severity = "error"
)

type Finding struct {
	ID           string
	Severity     Severity
	Message      string
	SuggestedFix string
	Range        dockerfile.Range
	Stage        *int
}

type Result struct {
	Source        string
	DetectedStack Stack
	SelectedStack Stack
	Supported     bool
	Findings      []Finding
}

// Analyze runs the applicable rules over doc. override selects a stack
// explicitly; an empty override uses the detected stack. ignore suppresses
// findings whose IDs match (case-insensitive), in addition to per-instruction
// # dockopt:disable comments.
func Analyze(doc *dockerfile.Document, override Stack, ignore ...string) Result {
	detected := DetectStack(doc)
	selected := detected
	if override != "" {
		selected = override
	}
	result := Result{
		Source:        doc.Name,
		DetectedStack: detected,
		SelectedStack: selected,
		Supported:     IsSupported(selected),
		Findings:      []Finding{},
	}
	ignored := ignoreSet(ignore)
	for _, r := range registeredRules {
		if !r.appliesToDocument(doc, override) {
			continue
		}
		for _, f := range r.check(doc) {
			f.ID = r.id
			f.Severity = r.severity
			f.SuggestedFix = suggestedFix(r.id)
			if ignored[strings.ToUpper(r.id)] || instructionDisables(doc, f, r.id) {
				continue
			}
			if override == "" && !r.keepFinding(doc, f) {
				continue
			}
			result.Findings = append(result.Findings, f)
		}
	}
	return result
}

func suggestedFix(id string) string {
	fixes := map[string]string{
		"GEN001":    "Pin the base image to an explicit immutable tag or digest.",
		"GEN002":    "Add --no-install-recommends to the apt-get install command.",
		"GEN003":    "Remove /var/lib/apt/lists/* in the same RUN instruction.",
		"GEN004":    "Download with curl or wget, verify a checksum, then extract explicitly.",
		"GEN005":    "Create or select a non-root account and set USER in the final stage.",
		"GEN006":    "Add --no-cache to the apk add command.",
		"GEN007":    "Run the package manager's clean command in the same RUN instruction.",
		"GEN008":    "Pass the value through RUN --mount=type=secret instead of ARG or ENV.",
		"GEN009":    "Add RUN --mount=type=cache with a manager-specific target so downloads are reused.",
		"GEN010":    "Pin the image with an explicit tag and digest, for example alpine:3.20@sha256:....",
		"GO001":     "Build in a golang stage and copy the binary into a minimal runtime stage.",
		"GO002":     "Set CGO_ENABLED=0 for the Go build copied into scratch.",
		"GO003":     "Copy the compiled binary into scratch, distroless, or another minimal runtime image.",
		"GO004":     "Copy go.mod and go.sum and download modules before copying the full source tree.",
		"JAVA001":   "Use a JRE, slim, jlink, or distroless image for the final stage.",
		"RUST001":   "Build in a rust stage and copy the binary into a minimal runtime stage.",
		"RUST002":   "Copy Cargo.toml and Cargo.lock and fetch crates before copying the full source tree.",
		"RUST003":   "Add --release to cargo build.",
		"RUST004":   "Copy the compiled binary into a slim, alpine, or distroless runtime image.",
		"DOTNET001": "Pin the .NET base image to an explicit tag.",
		"DOTNET002": "Publish in an SDK stage and copy the output into aspnet or runtime.",
		"PHP001":    "Add --no-dev to composer install.",
		"PHP002":    "Add --optimize-autoloader to composer install.",
		"RUBY001":   "Add --deployment to bundle install.",
		"PY001":     "Add --no-cache-dir to pip install.",
		"PY002":     "Copy requirements files and install dependencies before copying the full source tree.",
		"NODE001":   "Use npm ci with a committed lockfile.",
		"NODE002":   "Copy package.json and the lockfile and install dependencies before copying the full source tree.",
		"NODE003":   "Install with yarn --immutable/--frozen-lockfile or pnpm --frozen-lockfile.",
		"NODE004":   "Set ENV NODE_ENV=production before installing production dependencies.",
		"CCPP001":   "Compile in a builder stage and copy the binary into a minimal runtime stage.",
	}
	return fixes[id]
}

func (r rule) appliesToDocument(doc *dockerfile.Document, override Stack) bool {
	if override != "" {
		return r.appliesTo(override)
	}
	if r.appliesTo(StackGeneric) {
		return true
	}
	for _, stage := range doc.Stages {
		if r.appliesTo(DetectStageStack(stage)) {
			return true
		}
	}
	return false
}

func (r rule) keepFinding(doc *dockerfile.Document, f Finding) bool {
	if r.appliesTo(StackGeneric) || f.Stage == nil || *f.Stage < 0 || *f.Stage >= len(doc.Stages) {
		return true
	}
	return r.appliesTo(DetectStageStack(doc.Stages[*f.Stage]))
}

func ignoreSet(ids []string) map[string]bool {
	set := make(map[string]bool, len(ids))
	for _, id := range ids {
		for _, part := range strings.Split(id, ",") {
			if normalized := strings.ToUpper(strings.TrimSpace(part)); normalized != "" {
				set[normalized] = true
			}
		}
	}
	return set
}

func instructionDisables(doc *dockerfile.Document, f Finding, ruleID string) bool {
	for _, instruction := range doc.Instructions {
		if instruction.Range != f.Range {
			continue
		}
		for _, disabled := range instruction.Disabled {
			if strings.EqualFold(disabled, ruleID) {
				return true
			}
		}
	}
	return false
}

func KnownRuleID(id string) bool {
	want := strings.ToUpper(strings.TrimSpace(id))
	for _, r := range registeredRules {
		if r.id == want {
			return true
		}
	}
	return false
}
