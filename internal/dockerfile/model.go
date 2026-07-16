package dockerfile

import "fmt"

type Range struct{ StartLine, EndLine int }
type Instruction struct {
	Opcode, Original, Value string
	JSON                    bool
	Range                   Range
}
type Stage struct {
	Index                     int
	Name, BaseImage, Platform string
	From                      Instruction
	Instructions              []Instruction
}
type Document struct {
	Name         string
	EscapeToken  rune
	Instructions []Instruction
	Stages       []Stage
}
type ParseError struct {
	Source  string
	Line    int
	Message string
}

func (e *ParseError) Error() string { return fmt.Sprintf("%s:%d: %s", e.Source, e.Line, e.Message) }
