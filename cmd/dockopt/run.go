package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/davidgrldo/dockerfile-optimizer/internal/analyzer"
	"github.com/davidgrldo/dockerfile-optimizer/internal/config"
	"github.com/davidgrldo/dockerfile-optimizer/internal/dockerfile"
	"github.com/davidgrldo/dockerfile-optimizer/internal/fix"
	"github.com/davidgrldo/dockerfile-optimizer/internal/report"
)

func run(args []string, stdout, stderr io.Writer) int {
	return runWithOpener(args, stdout, stderr, func(path string) (io.ReadCloser, error) {
		if path == "-" {
			return io.NopCloser(os.Stdin), nil
		}
		return os.Open(path)
	})
}

func runWithOpener(args []string, stdout, stderr io.Writer, open func(string) (io.ReadCloser, error)) int {
	jsonRequested := requestsJSON(args)
	flags := flag.NewFlagSet("dockopt", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	jsonMode := flags.Bool("json", false, "output results as JSON")
	sarifMode := flags.Bool("sarif", false, "output one aggregated SARIF 2.1.0 log")
	stackName := flags.String("stack", "", "override detected stack")
	failOn := flags.String("fail-on", "error", "failure threshold: none, warn, or error")
	ignoreRules := flags.String("ignore", "", "comma-separated rule IDs to suppress")
	configPath := flags.String("config", "", "path to .dockopt.yml")
	noConfig := flags.Bool("no-config", false, "ignore .dockopt.yml")
	applyFix := flags.Bool("fix", false, "rewrite GEN002, GEN003, and GEN006 in place")
	if err := flags.Parse(args); err != nil {
		return writeFailure(stdout, stderr, jsonRequested, "usage_error", err)
	}
	setFlags := map[string]bool{}
	flags.Visit(func(f *flag.Flag) { setFlags[f.Name] = true })
	if *jsonMode && *sarifMode {
		return writeFailure(stdout, stderr, false, "usage_error", errors.New("--json and --sarif are mutually exclusive"))
	}
	cfg, err := loadRuntimeConfig(*noConfig, *configPath)
	if err != nil {
		return writeFailure(stdout, stderr, *jsonMode, "usage_error", err)
	}
	if !setFlags["fail-on"] && cfg.FailOn != "" {
		*failOn = cfg.FailOn
	}
	if !setFlags["stack"] && cfg.Stack != "" {
		*stackName = cfg.Stack
	}
	paths, err := expandInputs(flags.Args())
	if err != nil {
		return writeFailure(stdout, stderr, *jsonMode, "input_error", err)
	}
	if len(paths) < 1 {
		return writeFailure(stdout, stderr, *jsonMode, "usage_error", errors.New("expected at least one Dockerfile path"))
	}
	if *applyFix {
		for _, path := range paths {
			if path == "-" {
				return writeFailure(stdout, stderr, *jsonMode, "usage_error", errors.New("--fix cannot read from stdin"))
			}
		}
	}
	for _, path := range paths {
		if path != "-" && strings.HasPrefix(path, "-") {
			return writeFailure(stdout, stderr, *jsonMode, "usage_error", errors.New("flags must appear before Dockerfile paths"))
		}
	}
	if *failOn != "none" && *failOn != "warn" && *failOn != "error" {
		return writeFailure(stdout, stderr, *jsonMode, "usage_error", fmt.Errorf("invalid fail-on threshold %q", *failOn))
	}

	var ignore []string
	if *ignoreRules != "" {
		parsed, err := parseIgnore(*ignoreRules)
		if err != nil {
			return writeFailure(stdout, stderr, *jsonMode, "usage_error", err)
		}
		ignore = append(ignore, parsed...)
	}
	ignore = append(ignore, cfg.Ignore...)

	var override analyzer.Stack
	if *stackName != "" {
		var err error
		override, err = analyzer.ParseStack(*stackName)
		if err != nil {
			return writeFailure(stdout, stderr, *jsonMode, "usage_error", fmt.Errorf("unknown stack %q", *stackName))
		}
	}

	multi := len(paths) > 1
	if *sarifMode {
		return analyzeSARIF(paths, stdout, stderr, override, *failOn, ignore, *applyFix, open)
	}
	exit := 0
	for _, path := range paths {
		if code := analyzePath(path, stdout, stderr, *jsonMode, multi, override, *failOn, ignore, *applyFix, open); code > exit {
			exit = code
		}
	}
	return exit
}

// analyzePath analyzes one Dockerfile and writes its result. It returns the
// per-file exit contribution: 0 clean, 1 threshold reached, 2 could not analyze.
// The caller keeps the maximum across all paths.
func analyzePath(path string, stdout, stderr io.Writer, jsonMode, multi bool, override analyzer.Stack, failOn string, ignore []string, applyFix bool, open func(string) (io.ReadCloser, error)) int {
	result, kind, err := analyzeInput(path, override, ignore, applyFix, open)
	if err != nil {
		return writeFailure(stdout, stderr, jsonMode, kind, err)
	}

	if jsonMode {
		err = report.WriteJSON(stdout, result)
	} else {
		if multi {
			if _, herr := fmt.Fprintf(stdout, "==> %s <==\n", path); herr != nil {
				_, _ = fmt.Fprintf(stderr, "output_error: %v\n", herr)
				return 2
			}
		}
		err = report.WriteHuman(stdout, result)
	}
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "output_error: %v\n", err)
		return 2
	}
	if meetsThreshold(result.Findings, failOn) {
		return 1
	}
	return 0
}

func analyzeInput(path string, override analyzer.Stack, ignore []string, applyFix bool, open func(string) (io.ReadCloser, error)) (analyzer.Result, string, error) {
	file, err := open(path)
	if err != nil {
		return analyzer.Result{}, "input_error", err
	}

	doc, err := dockerfile.Parse(path, file)
	closeErr := file.Close()
	if err != nil {
		kind := "input_error"
		var parseErr *dockerfile.ParseError
		if errors.As(err, &parseErr) {
			kind = "parse_error"
		}
		return analyzer.Result{}, kind, err
	}
	if closeErr != nil {
		return analyzer.Result{}, "input_error", fmt.Errorf("close %s: %w", path, closeErr)
	}
	result := analyzer.Analyze(doc, override, ignore...)
	if applyFix {
		fixed, err := rewriteFile(path, result, override, ignore)
		if err != nil {
			return analyzer.Result{}, "output_error", err
		}
		result = fixed
	}
	return result, "", nil
}

func rewriteFile(path string, result analyzer.Result, override analyzer.Stack, ignore []string) (analyzer.Result, error) {
	src, err := os.ReadFile(path)
	if err != nil {
		return analyzer.Result{}, err
	}
	out, err := fix.Apply(src, result.Findings)
	if err != nil {
		return analyzer.Result{}, err
	}
	if bytes.Equal(src, out) {
		return result, nil
	}
	if err := os.WriteFile(path, out, 0o644); err != nil {
		return analyzer.Result{}, err
	}
	doc, err := dockerfile.Parse(path, bytes.NewReader(out))
	if err != nil {
		return analyzer.Result{}, err
	}
	return analyzer.Analyze(doc, override, ignore...), nil
}

func analyzeSARIF(paths []string, stdout, stderr io.Writer, override analyzer.Stack, failOn string, ignore []string, applyFix bool, open func(string) (io.ReadCloser, error)) int {
	results := make([]analyzer.Result, 0, len(paths))
	exit := 0
	for _, path := range paths {
		result, kind, err := analyzeInput(path, override, ignore, applyFix, open)
		if err != nil {
			if code := writeFailure(stdout, stderr, false, kind, err); code > exit {
				exit = code
			}
			continue
		}
		results = append(results, result)
		if meetsThreshold(result.Findings, failOn) && exit < 1 {
			exit = 1
		}
	}
	if err := report.WriteSARIF(stdout, results); err != nil {
		_, _ = fmt.Fprintf(stderr, "output_error: %v\n", err)
		return 2
	}
	return exit
}

func requestsJSON(args []string) bool {
	jsonMode := false
	for index := 0; index < len(args); index++ {
		arg := args[index]
		if arg == "--" || !strings.HasPrefix(arg, "-") {
			break
		}
		flagText := strings.TrimPrefix(arg, "-")
		flagText = strings.TrimPrefix(flagText, "-")
		name, value, hasValue := strings.Cut(flagText, "=")
		switch name {
		case "json":
			if !hasValue {
				jsonMode = true
				continue
			}
			if parsed, err := strconv.ParseBool(value); err == nil {
				jsonMode = parsed
			}
		case "stack", "fail-on", "ignore", "config":
			if hasValue {
				continue
			}
			index++
		}
	}
	return jsonMode
}

func loadRuntimeConfig(noConfig bool, explicit string) (config.Config, error) {
	if noConfig {
		return config.Config{}, nil
	}
	path := explicit
	if path == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return config.Config{}, err
		}
		found, err := config.Find(cwd)
		if err != nil {
			return config.Config{}, err
		}
		if found == "" {
			return config.Config{}, nil
		}
		path = found
	}
	cfg, err := config.Load(path)
	if err != nil {
		return config.Config{}, fmt.Errorf("config %s: %w", path, err)
	}
	if cfg.FailOn != "" && cfg.FailOn != "none" && cfg.FailOn != "warn" && cfg.FailOn != "error" {
		return config.Config{}, fmt.Errorf("invalid fail-on threshold %q", cfg.FailOn)
	}
	if cfg.Stack != "" {
		if _, err := analyzer.ParseStack(cfg.Stack); err != nil {
			return config.Config{}, fmt.Errorf("unknown stack %q", cfg.Stack)
		}
	}
	if len(cfg.Ignore) > 0 {
		ids, err := parseIgnore(strings.Join(cfg.Ignore, ","))
		if err != nil {
			return config.Config{}, err
		}
		cfg.Ignore = ids
	}
	return cfg, nil
}

func writeFailure(stdout, stderr io.Writer, jsonMode bool, kind string, err error) int {
	if jsonMode {
		if writeErr := report.WriteErrorJSON(stdout, kind, err.Error()); writeErr != nil {
			_, _ = fmt.Fprintf(stderr, "output_error: %v\n", writeErr)
		}
	} else {
		_, _ = fmt.Fprintf(stderr, "%s: %v\n", kind, err)
	}
	return 2
}

func meetsThreshold(findings []analyzer.Finding, threshold string) bool {
	for _, finding := range findings {
		if threshold == "warn" && (finding.Severity == analyzer.SeverityWarn || finding.Severity == analyzer.SeverityError) ||
			threshold == "error" && finding.Severity == analyzer.SeverityError {
			return true
		}
	}
	return false
}

func parseIgnore(value string) ([]string, error) {
	var ids []string
	for _, part := range strings.Split(value, ",") {
		id := strings.ToUpper(strings.TrimSpace(part))
		if id == "" {
			continue
		}
		if !analyzer.KnownRuleID(id) {
			return nil, fmt.Errorf("unknown rule %q", id)
		}
		ids = append(ids, id)
	}
	return ids, nil
}

func expandInputs(paths []string) ([]string, error) {
	var out []string
	for _, path := range paths {
		if path == "-" {
			out = append(out, path)
			continue
		}
		info, err := os.Stat(path)
		if err != nil {
			out = append(out, path)
			continue
		}
		if !info.IsDir() {
			out = append(out, path)
			continue
		}
		found, err := findDockerfiles(path)
		if err != nil {
			return nil, err
		}
		if len(found) == 0 {
			return nil, fmt.Errorf("no Dockerfiles found in %s", path)
		}
		out = append(out, found...)
	}
	return out, nil
}

func findDockerfiles(root string) ([]string, error) {
	var found []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if path != root && skipDirectory(d.Name()) {
				return fs.SkipDir
			}
			return nil
		}
		if isDockerfileName(d.Name()) {
			found = append(found, path)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return found, nil
}

func skipDirectory(name string) bool {
	if strings.HasPrefix(name, ".") {
		return true
	}
	switch name {
	case "node_modules", "vendor":
		return true
	default:
		return false
	}
}

func isDockerfileName(name string) bool {
	return name == "Dockerfile" || name == "dockerfile" || strings.HasPrefix(name, "Dockerfile.") || strings.HasSuffix(name, ".Dockerfile")
}
