package config

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type Config struct {
	FailOn string
	Ignore []string
	Stack  string
}

func Find(startDir string) (string, error) {
	dir, err := filepath.Abs(startDir)
	if err != nil {
		return "", err
	}
	for {
		candidate := filepath.Join(dir, ".dockopt.yml")
		info, err := os.Stat(candidate)
		if err == nil && !info.IsDir() {
			return candidate, nil
		}
		if err != nil && !os.IsNotExist(err) {
			return "", err
		}
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			return "", nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", nil
		}
		dir = parent
	}
}

func Load(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}
	return Parse(data)
}

func Parse(data []byte) (Config, error) {
	var cfg Config
	scanner := bufio.NewScanner(bytes.NewReader(data))
	inIgnoreList := false
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if inIgnoreList {
			if strings.HasPrefix(line, "-") {
				item := strings.TrimSpace(strings.TrimPrefix(line, "-"))
				if item != "" {
					cfg.Ignore = append(cfg.Ignore, item)
				}
				continue
			}
			inIgnoreList = false
		}
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			return Config{}, fmt.Errorf("line %d: expected key: value", lineNo)
		}
		key = strings.ToLower(strings.TrimSpace(key))
		value = strings.TrimSpace(value)
		switch key {
		case "fail-on":
			cfg.FailOn = value
		case "stack":
			cfg.Stack = value
		case "ignore":
			if value == "" {
				inIgnoreList = true
				continue
			}
			for _, part := range strings.Split(value, ",") {
				if id := strings.TrimSpace(part); id != "" {
					cfg.Ignore = append(cfg.Ignore, id)
				}
			}
		default:
			return Config{}, fmt.Errorf("unknown key %q", key)
		}
	}
	if err := scanner.Err(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}
