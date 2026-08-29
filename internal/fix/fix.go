package fix

import (
	"regexp"
	"strings"

	"github.com/davidgrldo/dockerfile-optimizer/internal/analyzer"
)

var (
	aptInstallPattern = regexp.MustCompile(`(?i)(apt-get(?:[ \t]+-[^ \t]+)*[ \t]+install)([ \t])`)
	apkAddPattern     = regexp.MustCompile(`(?i)(apk(?:[ \t]+-[^ \t]+)*[ \t]+add)([ \t])`)
)

func Apply(src []byte, findings []analyzer.Finding) ([]byte, error) {
	lines := strings.Split(string(src), "\n")
	keepNL := strings.HasSuffix(string(src), "\n")
	for _, finding := range findings {
		switch finding.ID {
		case "GEN002", "GEN003", "GEN006":
		default:
			continue
		}
		start := finding.Range.StartLine - 1
		end := finding.Range.EndLine - 1
		if start < 0 || end >= len(lines) || start > end {
			continue
		}
		block := strings.Join(lines[start:end+1], "\n")
		if strings.Contains(block, "<<") {
			continue
		}
		switch finding.ID {
		case "GEN002":
			if strings.Contains(block, "--no-install-recommends") {
				continue
			}
			replaced := aptInstallPattern.ReplaceAllString(block, "$1 --no-install-recommends$2")
			if replaced == block {
				continue
			}
			copy(lines[start:end+1], strings.Split(replaced, "\n"))
		case "GEN006":
			if strings.Contains(block, "--no-cache") {
				continue
			}
			replaced := apkAddPattern.ReplaceAllString(block, "$1 --no-cache$2")
			if replaced == block {
				continue
			}
			copy(lines[start:end+1], strings.Split(replaced, "\n"))
		case "GEN003":
			if strings.Contains(block, "/var/lib/apt/lists") {
				continue
			}
			lines[end] = strings.TrimRight(lines[end], "\r") + " && rm -rf /var/lib/apt/lists/*"
		}
	}
	out := strings.Join(lines, "\n")
	if keepNL && !strings.HasSuffix(out, "\n") {
		out += "\n"
	}
	return []byte(out), nil
}
