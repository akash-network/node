package uiutil

import (
	"bytes"
	"fmt"
	"io"
	"os"
)

// Out is the default out
var Out = os.Stdout

// Component in the interface that UI components need to implement
type Component interface {
	Bytes() []byte
}

type Printer interface {
	Component
	AddTitle(string) Printer
	Add(Component) Printer
	Flush() error
}

// printer represents a buffered container for components that can be flushed
type printer struct {
	out   io.Writer
	comps []Component
}

// NewPrinter returns a pointer to a new printer object
func NewPrinter(out io.Writer) Printer {
	if out == nil {
		out = Out
	}
	return &printer{out: out}
}

// Add adds the components to the printer
func (p *printer) Add(c Component) Printer {
	p.comps = append(p.comps, c)
	return p
}

// AddTitle adds a Title to the printer
func (p *printer) AddTitle(title string) Printer {
	return p.Add(&Title{text: title})
}

// Bytes returns the formmated string of the output
func (p *printer) Bytes() []byte {
	var buf bytes.Buffer
	for _, c := range p.comps {
		buf.Write(c.Bytes())
		buf.Write([]byte{'\n'})
	}
	return buf.Bytes()
}

// Flush prints the output to the writer and clears the buffer
func (p *printer) Flush() error {
	_, err := fmt.Fprintln(p.out, p)
	if err != nil {
		return err
	}
	p.comps = nil
	return nil
}
