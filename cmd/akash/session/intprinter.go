package session

import (
	"bytes"
	"fmt"
	"io"
	"os"

	"github.com/ovrclk/akash/util/uiutil"
)

// UIComponent in the interface that UI components need to implement
type UIComponent interface {
	Bytes() []byte
}

type IPrinter interface {
	// AddTitle adds a title with the given string and
	// returns the instance of IPrinter with the title
	AddTitle(string) IPrinter

	// AddText adds a text component with the given string and
	// returns the instance of IPrinter with the text
	AddText(string) IPrinter

	// Add adds the components to the printer
	Add(UIComponent) IPrinter

	// Flush prints the output to the writer and clears the buffer
	Flush() error

	// Bytes returns the formmated string of the output
	Bytes() []byte
}
type iprinter struct {
	comps []UIComponent
	out   io.Writer
}

// NewIPrinter returns a pointer to a new printer object
func NewIPrinter(out io.Writer) IPrinter {
	if out == nil {
		out = os.Stdout
	}
	return &iprinter{out: out}
}

// AddTitle adds a title with the given string and
// returns the instance of IPrinter with the title
func (p *iprinter) AddTitle(str string) IPrinter {
	return p.Add(uiutil.NewTitle(str))
}

func (p *iprinter) AddText(str string) IPrinter {
	return p.Add(bytes.NewBufferString(str))
}

// Add adds the components to the printer
func (p *iprinter) Add(c UIComponent) IPrinter {
	p.comps = append(p.comps, c)
	return p
}

// Bytes returns the formmated string of the output
func (p *iprinter) Bytes() []byte {
	var buf bytes.Buffer
	for _, c := range p.comps {
		buf.Write(c.Bytes())
		buf.Write([]byte{'\n'})
	}
	return buf.Bytes()
}

// Flush prints the output to the writer and clears the buffer
func (p *iprinter) Flush() error {
	_, err := fmt.Fprintln(p.out, string(p.Bytes()))
	if err != nil {
		return err
	}
	p.comps = nil
	return nil
}
