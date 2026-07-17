package dockerfile

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"unicode"
	"unicode/utf8"
)

const maxScannerToken = 4 * 1024 * 1024

type physicalLine struct {
	number int
	text   string
}

func Parse(name string, r io.Reader) (*Document, error) {
	doc := &Document{
		Name:         name,
		EscapeToken:  '\\',
		Instructions: []Instruction{},
		Stages:       []Stage{},
	}
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 64*1024), maxScannerToken)
	lines := []physicalLine{}
	for scanner.Scan() {
		lines = append(lines, physicalLine{number: len(lines) + 1, text: scanner.Text()})
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("%s:%d: %w", name, len(lines)+1, err)
	}

	var logical string
	start := 0
	end := 0
	continued := false

	for next := 0; next < len(lines); {
		line := lines[next]
		next++
		physical := line.text
		trimmed := strings.TrimSpace(physical)
		if trimmed == "" {
			continue
		}
		if strings.HasPrefix(trimmed, "#") {
			if !continued {
				if escape, ok := parseEscapeDirective(physical); ok {
					doc.EscapeToken = escape
				}
			}
			continue
		}
		if !continued {
			start = line.number
			logical = ""
		}
		end = line.number

		var text string
		continued, text = stripContinuation(physical, doc.EscapeToken)
		if part := strings.TrimSpace(text); part != "" {
			if logical != "" {
				logical += " "
			}
			logical += part
		}
		if continued {
			continue
		}
		var err error
		next, err = parseAndAdd(doc, logical, start, end, lines, next)
		if err != nil {
			return nil, err
		}
	}
	if continued {
		return nil, &ParseError{Source: name, Line: start, Message: "unterminated line continuation"}
	}
	return doc, nil
}

func parseAndAdd(doc *Document, text string, start, end int, lines []physicalLine, next int) (int, error) {
	instruction, err := parseInstruction(doc.Name, text, start, end)
	if err != nil {
		return next, err
	}
	for _, delimiter := range heredocDelimiters(instruction.Value) {
		found := false
		for next < len(lines) {
			line := lines[next]
			next++
			if strings.TrimSpace(line.text) == delimiter {
				instruction.Range.EndLine = line.number
				found = true
				break
			}
		}
		if !found {
			return next, &ParseError{Source: doc.Name, Line: start, Message: "unterminated heredoc " + delimiter}
		}
	}
	doc.addInstruction(instruction)
	return next, nil
}

func heredocDelimiters(value string) []string {
	var delimiters []string
	for _, field := range strings.Fields(value) {
		if !strings.HasPrefix(field, "<<") {
			continue
		}
		if strings.HasPrefix(field, "<<<") { // bash here-string, not a heredoc
			continue
		}
		delimiter := strings.TrimPrefix(field, "<<")
		delimiter = strings.TrimPrefix(delimiter, "-")
		if len(delimiter) >= 2 && (delimiter[0] == '\'' && delimiter[len(delimiter)-1] == '\'' || delimiter[0] == '"' && delimiter[len(delimiter)-1] == '"') {
			delimiter = delimiter[1 : len(delimiter)-1]
		}
		if delimiter != "" {
			delimiters = append(delimiters, delimiter)
		}
	}
	return delimiters
}

func parseInstruction(source, text string, start, end int) (Instruction, error) {
	original := strings.TrimSpace(text)
	if original == "" {
		return Instruction{}, &ParseError{Source: source, Line: start, Message: "missing instruction"}
	}

	opcode := original
	value := ""
	if separator := strings.IndexFunc(original, unicode.IsSpace); separator >= 0 {
		opcode = original[:separator]
		value = strings.TrimSpace(original[separator:])
	}
	instruction := Instruction{
		Opcode:   strings.ToUpper(opcode),
		Original: original,
		Value:    value,
		Range:    Range{StartLine: start, EndLine: end},
	}

	if strings.HasPrefix(value, "[") {
		var items []json.RawMessage
		if err := json.Unmarshal([]byte(value), &items); err != nil {
			return Instruction{}, &ParseError{Source: source, Line: start, Message: fmt.Sprintf("invalid JSON array: %v", err)}
		}
		instruction.JSON = true
	}
	if instruction.Opcode == "FROM" {
		if _, _, _, err := parseFrom(value); err != nil {
			return Instruction{}, &ParseError{Source: source, Line: start, Message: err.Error()}
		}
	}
	return instruction, nil
}

func parseFrom(value string) (base, name, platform string, err error) {
	fields := strings.Fields(value)
	for len(fields) > 0 && strings.HasPrefix(fields[0], "--") {
		flag := fields[0]
		fields = fields[1:]
		key, flagValue, hasValue := strings.Cut(strings.TrimPrefix(flag, "--"), "=")
		if strings.EqualFold(key, "platform") {
			if !hasValue {
				if len(fields) == 0 {
					return "", "", "", errors.New("FROM --platform requires a value")
				}
				flagValue = fields[0]
				fields = fields[1:]
			}
			if flagValue == "" {
				return "", "", "", errors.New("FROM --platform requires a value")
			}
			platform = flagValue
		}
	}
	if len(fields) == 0 || strings.EqualFold(fields[0], "AS") {
		return "", "", "", errors.New("FROM requires a base image")
	}
	base = fields[0]
	fields = fields[1:]
	if len(fields) == 0 {
		return base, "", platform, nil
	}
	if len(fields) != 2 || !strings.EqualFold(fields[0], "AS") || fields[1] == "" {
		return "", "", "", errors.New("invalid FROM instruction")
	}
	return base, fields[1], platform, nil
}

func parseEscapeDirective(line string) (rune, bool) {
	line = strings.TrimSpace(line)
	if !strings.HasPrefix(line, "#") {
		return 0, false
	}
	key, value, ok := strings.Cut(strings.TrimSpace(strings.TrimPrefix(line, "#")), "=")
	value = strings.TrimSpace(value)
	if !ok || !strings.EqualFold(strings.TrimSpace(key), "escape") || utf8.RuneCountInString(value) != 1 {
		return 0, false
	}
	escape, _ := utf8.DecodeRuneInString(value)
	if escape != '\\' && escape != '`' {
		return 0, false
	}
	return escape, true
}

func stripContinuation(line string, escape rune) (continued bool, text string) {
	text = strings.TrimRightFunc(line, unicode.IsSpace)
	runes := []rune(text)
	count := 0
	for i := len(runes) - 1; i >= 0 && runes[i] == escape; i-- {
		count++
	}
	if count%2 == 0 {
		return false, text
	}
	return true, string(runes[:len(runes)-1])
}

func (d *Document) addInstruction(instruction Instruction) {
	d.Instructions = append(d.Instructions, instruction)
	if instruction.Opcode == "FROM" {
		base, name, platform, _ := parseFrom(instruction.Value)
		d.Stages = append(d.Stages, Stage{
			Index:        len(d.Stages),
			Name:         name,
			BaseImage:    base,
			Platform:     platform,
			From:         instruction,
			Instructions: []Instruction{},
		})
		return
	}
	if len(d.Stages) > 0 {
		stage := &d.Stages[len(d.Stages)-1]
		stage.Instructions = append(stage.Instructions, instruction)
	}
}
