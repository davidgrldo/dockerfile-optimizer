package report

import (
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"path/filepath"

	"github.com/davidgrldo/dockerfile-optimizer/internal/analyzer"
)

type Output struct {
	SchemaVersion string          `json:"schema_version"`
	Source        string          `json:"source"`
	Stack         StackOutput     `json:"stack"`
	Findings      []FindingOutput `json:"findings"`
	Summary       Summary         `json:"summary"`
}

type StackOutput struct {
	Detected  analyzer.Stack `json:"detected"`
	Selected  analyzer.Stack `json:"selected"`
	Supported bool           `json:"supported"`
}

type FindingOutput struct {
	ID           string            `json:"id"`
	Severity     analyzer.Severity `json:"severity"`
	Message      string            `json:"message"`
	SuggestedFix string            `json:"suggested_fix"`
	Line         int               `json:"line"`
	EndLine      int               `json:"end_line"`
	Stage        *int              `json:"stage"`
}

type Summary struct {
	Info  int `json:"info"`
	Warn  int `json:"warn"`
	Error int `json:"error"`
}

type ErrorOutput struct {
	SchemaVersion string      `json:"schema_version"`
	Error         ErrorDetail `json:"error"`
}

type ErrorDetail struct {
	Kind    string `json:"kind"`
	Message string `json:"message"`
}

func NewOutput(result analyzer.Result) Output {
	output := Output{
		SchemaVersion: "2",
		Source:        result.Source,
		Stack: StackOutput{
			Detected:  result.DetectedStack,
			Selected:  result.SelectedStack,
			Supported: result.Supported,
		},
		Findings: []FindingOutput{},
	}
	for _, finding := range result.Findings {
		output.Findings = append(output.Findings, FindingOutput{
			ID:           finding.ID,
			Severity:     finding.Severity,
			Message:      finding.Message,
			SuggestedFix: finding.SuggestedFix,
			Line:         finding.Range.StartLine,
			EndLine:      finding.Range.EndLine,
			Stage:        finding.Stage,
		})
		switch finding.Severity {
		case analyzer.SeverityInfo:
			output.Summary.Info++
		case analyzer.SeverityWarn:
			output.Summary.Warn++
		case analyzer.SeverityError:
			output.Summary.Error++
		}
	}
	return output
}

func WriteJSON(w io.Writer, result analyzer.Result) error {
	return json.NewEncoder(w).Encode(NewOutput(result))
}

func WriteErrorJSON(w io.Writer, kind, message string) error {
	return json.NewEncoder(w).Encode(ErrorOutput{
		SchemaVersion: "2",
		Error:         ErrorDetail{Kind: kind, Message: message},
	})
}

const sarifSchema = "https://json.schemastore.org/sarif-2.1.0.json"

type SARIFLog struct {
	Version string     `json:"version"`
	Schema  string     `json:"$schema"`
	Runs    []SARIFRun `json:"runs"`
}

type SARIFRun struct {
	Tool    SARIFTool     `json:"tool"`
	Results []SARIFResult `json:"results"`
}

type SARIFTool struct {
	Driver SARIFDriver `json:"driver"`
}

type SARIFDriver struct {
	Name           string              `json:"name"`
	InformationURI string              `json:"informationUri"`
	Rules          []SARIFRuleMetadata `json:"rules"`
}

type SARIFRuleMetadata struct {
	ID               string       `json:"id"`
	ShortDescription SARIFMessage `json:"shortDescription"`
}

type SARIFResult struct {
	RuleID     string                `json:"ruleId"`
	Level      string                `json:"level"`
	Message    SARIFMessage          `json:"message"`
	Locations  []SARIFLocation       `json:"locations"`
	Properties SARIFResultProperties `json:"properties"`
}

type SARIFMessage struct {
	Text string `json:"text"`
}

type SARIFLocation struct {
	PhysicalLocation SARIFPhysicalLocation `json:"physicalLocation"`
}

type SARIFPhysicalLocation struct {
	ArtifactLocation SARIFArtifactLocation `json:"artifactLocation"`
	Region           SARIFRegion           `json:"region"`
}

type SARIFArtifactLocation struct {
	URI string `json:"uri"`
}

type SARIFRegion struct {
	StartLine int `json:"startLine"`
	EndLine   int `json:"endLine"`
}

type SARIFResultProperties struct {
	SuggestedFix string `json:"suggestedFix,omitempty"`
}

func WriteSARIF(w io.Writer, results []analyzer.Result) error {
	run := SARIFRun{
		Tool: SARIFTool{Driver: SARIFDriver{
			Name:           "dockopt",
			InformationURI: "https://github.com/davidgrldo/dockerfile-optimizer",
			Rules:          []SARIFRuleMetadata{},
		}},
		Results: []SARIFResult{},
	}
	seenRules := map[string]bool{}
	for _, analysis := range results {
		for _, finding := range analysis.Findings {
			if !seenRules[finding.ID] {
				run.Tool.Driver.Rules = append(run.Tool.Driver.Rules, SARIFRuleMetadata{
					ID:               finding.ID,
					ShortDescription: SARIFMessage{Text: finding.Message},
				})
				seenRules[finding.ID] = true
			}
			run.Results = append(run.Results, SARIFResult{
				RuleID:  finding.ID,
				Level:   sarifLevel(finding.Severity),
				Message: SARIFMessage{Text: finding.Message},
				Locations: []SARIFLocation{{PhysicalLocation: SARIFPhysicalLocation{
					ArtifactLocation: SARIFArtifactLocation{URI: sarifArtifactURI(analysis.Source)},
					Region: SARIFRegion{
						StartLine: finding.Range.StartLine,
						EndLine:   finding.Range.EndLine,
					},
				}}},
				Properties: SARIFResultProperties{SuggestedFix: finding.SuggestedFix},
			})
		}
	}
	return json.NewEncoder(w).Encode(SARIFLog{
		Version: "2.1.0",
		Schema:  sarifSchema,
		Runs:    []SARIFRun{run},
	})
}

func sarifArtifactURI(source string) string {
	if source == "-" {
		return "stdin:///Dockerfile"
	}
	if filepath.IsAbs(source) {
		return (&url.URL{Scheme: "file", Path: filepath.ToSlash(source)}).String()
	}
	return filepath.ToSlash(source)
}

func sarifLevel(severity analyzer.Severity) string {
	switch severity {
	case analyzer.SeverityError:
		return "error"
	case analyzer.SeverityWarn:
		return "warning"
	default:
		return "note"
	}
}

func WriteHuman(w io.Writer, result analyzer.Result) error {
	if _, err := fmt.Fprintf(w, "Detected stack: %s\n", result.DetectedStack); err != nil {
		return err
	}
	if result.SelectedStack != result.DetectedStack {
		if _, err := fmt.Fprintf(w, "Selected stack: %s\n", result.SelectedStack); err != nil {
			return err
		}
	}
	if result.Supported {
		if _, err := fmt.Fprintln(w, "Stack-specific checks enabled."); err != nil {
			return err
		}
	} else {
		if _, err := fmt.Fprintln(w, "Stack-specific checks unavailable; generic checks only."); err != nil {
			return err
		}
	}
	for _, finding := range result.Findings {
		line := fmt.Sprintf("line %d", finding.Range.StartLine)
		if finding.Range.EndLine != finding.Range.StartLine {
			line = fmt.Sprintf("lines %d-%d", finding.Range.StartLine, finding.Range.EndLine)
		}
		if _, err := fmt.Fprintf(w, "[%s] %s (%s): %s\n", finding.Severity, finding.ID, line, finding.Message); err != nil {
			return err
		}
	}
	if result.Supported && len(result.Findings) == 0 {
		_, err := fmt.Fprintln(w, "No issues found.")
		return err
	}
	return nil
}
