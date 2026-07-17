package dockerfile

import (
	"errors"
	"strings"
	"testing"
)

func TestParseCoreSyntax(t *testing.T) {
	input := "  # FROM rust:latest\n  from --platform=linux/amd64 golang:1.24 AS build\n\trun\tgo build ./...\n"
	doc, err := Parse("Dockerfile", strings.NewReader(input))
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Instructions) != 2 || doc.Instructions[0].Opcode != "FROM" {
		t.Fatalf("instructions=%#v", doc.Instructions)
	}
	if doc.Instructions[1].Opcode != "RUN" || doc.Instructions[1].Value != "go build ./..." {
		t.Fatalf("instruction=%#v", doc.Instructions[1])
	}
	stage := doc.Stages[0]
	if stage.BaseImage != "golang:1.24" || stage.Name != "build" || stage.Platform != "linux/amd64" {
		t.Fatalf("stage=%#v", stage)
	}
	if stage.From.Range != (Range{StartLine: 2, EndLine: 2}) {
		t.Fatalf("range=%#v", stage.From.Range)
	}
}

func TestParseActualFinalStage(t *testing.T) {
	doc, err := Parse("Dockerfile", strings.NewReader("FROM golang AS build\nRUN go build\nFROM alpine AS prep\nRUN true\nFROM scratch\n"))
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Stages) != 3 || doc.Stages[2].BaseImage != "scratch" || doc.Stages[2].Index != 2 {
		t.Fatalf("stages=%#v", doc.Stages)
	}
}

func TestParseMalformedFrom(t *testing.T) {
	_, err := Parse("Dockerfile", strings.NewReader("FROM --platform=linux/amd64\n"))
	var parseErr *ParseError
	if !errors.As(err, &parseErr) || parseErr.Line != 1 {
		t.Fatalf("error=%#v", err)
	}
}

func TestParsePreservesReadErrorIdentity(t *testing.T) {
	want := errors.New("read failed")
	_, err := Parse("Dockerfile", failingReader{err: want})
	if !errors.Is(err, want) {
		t.Fatalf("error=%v, want wrapped %v", err, want)
	}
	var parseErr *ParseError
	if errors.As(err, &parseErr) {
		t.Fatalf("read error must not be ParseError: %#v", err)
	}
}

func TestParseContinuationIgnoresBlankLine(t *testing.T) {
	doc, err := Parse("Dockerfile", strings.NewReader("FROM alpine\nRUN echo one \\\n\n && echo two\n"))
	if err != nil {
		t.Fatal(err)
	}
	run := doc.Stages[0].Instructions[0]
	if run.Value != "echo one && echo two" || run.Range != (Range{StartLine: 2, EndLine: 4}) {
		t.Fatalf("run=%#v", run)
	}
}

func TestParseContinuationIgnoresCommentLine(t *testing.T) {
	doc, err := Parse("Dockerfile", strings.NewReader("FROM alpine\nRUN echo one \\\n# ignored\n && echo two\n"))
	if err != nil {
		t.Fatal(err)
	}
	run := doc.Stages[0].Instructions[0]
	if run.Value != "echo one && echo two" || run.Range != (Range{StartLine: 2, EndLine: 4}) {
		t.Fatalf("run=%#v", run)
	}
}

func TestParseContinuationRange(t *testing.T) {
	doc, err := Parse("Dockerfile", strings.NewReader("FROM golang AS build\nRUN echo prep \\\n && go build -o /app\n"))
	if err != nil {
		t.Fatal(err)
	}
	run := doc.Stages[0].Instructions[0]
	if run.Value != "echo prep && go build -o /app" || run.Range != (Range{StartLine: 2, EndLine: 3}) {
		t.Fatalf("run=%#v", run)
	}
}

func TestParseRejectsTrailingContinuationAtEOF(t *testing.T) {
	_, err := Parse("Dockerfile", strings.NewReader("FROM alpine\nRUN echo unfinished \\\n"))
	var parseErr *ParseError
	if !errors.As(err, &parseErr) {
		t.Fatalf("error=%#v, want *ParseError", err)
	}
	if parseErr.Line != 2 {
		t.Fatalf("line=%d, want 2", parseErr.Line)
	}
}

func TestParseCustomEscapeAndJSON(t *testing.T) {
	input := "# escape=`\nFROM windows/servercore:ltsc2022`\n AS runtime\nCMD [\"cmd\", \"/C\", \"echo ok\"]\n"
	doc, err := Parse("Dockerfile", strings.NewReader(input))
	if err != nil {
		t.Fatal(err)
	}
	if doc.EscapeToken != '`' || doc.Stages[0].Name != "runtime" || !doc.Stages[0].Instructions[0].JSON {
		t.Fatalf("doc=%#v", doc)
	}
}

func TestParseHeredocIsolation(t *testing.T) {
	input := "FROM alpine\nRUN <<EOF\nFROM rust:latest\ngo build ./...\nEOF\nRUN echo done\n"
	doc, err := Parse("Dockerfile", strings.NewReader(input))
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Instructions) != 3 {
		t.Fatalf("instructions=%d", len(doc.Instructions))
	}
	if got := doc.Stages[0].Instructions[0].Range; got != (Range{StartLine: 2, EndLine: 5}) {
		t.Fatalf("range=%#v", got)
	}
}

func TestParseUnterminatedHeredoc(t *testing.T) {
	_, err := Parse("Dockerfile", strings.NewReader("FROM alpine\nRUN <<EOF\necho hi\n"))
	if err == nil || !strings.Contains(err.Error(), "unterminated heredoc EOF") {
		t.Fatalf("error=%v", err)
	}
}

func TestParseHereStringIsNotHeredoc(t *testing.T) {
	doc, err := Parse("Dockerfile", strings.NewReader("FROM alpine:3.19\nRUN cat <<<\"inline\"\nRUN echo done\n"))
	if err != nil {
		t.Fatalf("here-string must not be treated as a heredoc: %v", err)
	}
	if len(doc.Instructions) != 3 {
		t.Fatalf("instructions=%d, want 3", len(doc.Instructions))
	}
}

func FuzzParse(f *testing.F) {
	f.Add("FROM alpine\nRUN echo ok\n")
	f.Fuzz(func(t *testing.T, input string) {
		doc, err := Parse("fuzz", strings.NewReader(input))
		if err != nil {
			return
		}
		for _, item := range doc.Instructions {
			if item.Range.StartLine < 1 || item.Range.EndLine < item.Range.StartLine {
				t.Fatalf("range=%#v", item.Range)
			}
		}
	})
}

type failingReader struct{ err error }

func (r failingReader) Read([]byte) (int, error) { return 0, r.err }
