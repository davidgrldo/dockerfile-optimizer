package fix

import (
	"strings"
	"testing"

	"github.com/davidgrldo/dockerfile-optimizer/internal/analyzer"
	"github.com/davidgrldo/dockerfile-optimizer/internal/dockerfile"
)

func TestApplyGEN002AndGEN003(t *testing.T) {
	src := "FROM debian:bookworm\nRUN apt-get install -y curl\nUSER app\n"
	doc, err := dockerfile.Parse("Dockerfile", strings.NewReader(src))
	if err != nil {
		t.Fatal(err)
	}
	result := analyzer.Analyze(doc, analyzer.StackGeneric, "GEN010")
	out, err := Apply([]byte(src), result.Findings)
	if err != nil {
		t.Fatal(err)
	}
	got := string(out)
	if !strings.Contains(got, "--no-install-recommends") || !strings.Contains(got, "/var/lib/apt/lists") {
		t.Fatalf("fixed=%q", got)
	}
}

func TestApplyGEN006(t *testing.T) {
	src := "FROM alpine:3.20\nRUN apk add curl\nUSER app\n"
	doc, err := dockerfile.Parse("Dockerfile", strings.NewReader(src))
	if err != nil {
		t.Fatal(err)
	}
	result := analyzer.Analyze(doc, analyzer.StackGeneric, "GEN010")
	out, err := Apply([]byte(src), result.Findings)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "--no-cache") {
		t.Fatalf("fixed=%q", out)
	}
}

func TestApplySkipsHeredoc(t *testing.T) {
	src := "FROM debian:bookworm\nRUN <<EOF\napt-get install -y curl\nEOF\nUSER app\n"
	doc, err := dockerfile.Parse("Dockerfile", strings.NewReader(src))
	if err != nil {
		t.Fatal(err)
	}
	result := analyzer.Analyze(doc, analyzer.StackGeneric, "GEN010")
	out, err := Apply([]byte(src), result.Findings)
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != src {
		t.Fatalf("heredoc should be left untouched: %q", out)
	}
}
