package uiutil

import (
	"bytes"
)

// TitleUnderliner is the underline character for the title
var TitleUnderliner = "="

// Title is a UI component that renders a title. Title implements Component interface.
type Title struct {
	text   string
	uliner string
}

func NewTitle(text string) *Title {
	return &Title{text: text, uliner: TitleUnderliner}
}

func (t *Title) WithUnderliner(u string) *Title {
	t.uliner = u
	return t
}

// String returns the formated string of the title
func (t *Title) Bytes() []byte {
	var buf bytes.Buffer
	buf.WriteString(t.text + "\n")
	for i := 0; i < len(t.text); i++ {
		buf.Write([]byte(t.uliner))
	}
	buf.WriteString("\n")
	return buf.Bytes()
}

func (t *Title) String() string {
	return string(t.Bytes())
}
