package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestParseIgnoreListAndFailOn(t *testing.T) {
	cfg, err := Parse([]byte("fail-on: warn\nignore:\n  - GEN010\n  - GEN005\nstack: go\n"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.FailOn != "warn" || cfg.Stack != "go" || !reflect.DeepEqual(cfg.Ignore, []string{"GEN010", "GEN005"}) {
		t.Fatalf("cfg=%#v", cfg)
	}
}

func TestParseInlineIgnore(t *testing.T) {
	cfg, err := Parse([]byte("ignore: GEN001, GEN005\n"))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(cfg.Ignore, []string{"GEN001", "GEN005"}) {
		t.Fatalf("ignore=%v", cfg.Ignore)
	}
}

func TestParseRejectsUnknownKey(t *testing.T) {
	_, err := Parse([]byte("pretty: true\n"))
	if err == nil || !strings.Contains(err.Error(), "unknown key") {
		t.Fatalf("err=%v", err)
	}
}

func TestFindWalksParents(t *testing.T) {
	root := t.TempDir()
	nested := filepath.Join(root, "app", "svc")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, ".dockopt.yml")
	if err := os.WriteFile(path, []byte("fail-on: none\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	found, err := Find(nested)
	if err != nil || found != path {
		t.Fatalf("found=%q err=%v want %q", found, err, path)
	}
}

func TestFindStopsAtGitRoot(t *testing.T) {
	root := t.TempDir()
	nested := filepath.Join(root, "app")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	found, err := Find(nested)
	if err != nil || found != "" {
		t.Fatalf("found=%q err=%v", found, err)
	}
}
