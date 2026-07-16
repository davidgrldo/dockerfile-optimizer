package report

import (
	"encoding/json"
	"fmt"
	"io"

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
	ID       string            `json:"id"`
	Severity analyzer.Severity `json:"severity"`
	Message  string            `json:"message"`
	Line     int               `json:"line"`
	EndLine  int               `json:"end_line"`
	Stage    *int              `json:"stage"`
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
		SchemaVersion: "1",
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
			ID:       finding.ID,
			Severity: finding.Severity,
			Message:  finding.Message,
			Line:     finding.Range.StartLine,
			EndLine:  finding.Range.EndLine,
			Stage:    finding.Stage,
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
		SchemaVersion: "1",
		Error:         ErrorDetail{Kind: kind, Message: message},
	})
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
