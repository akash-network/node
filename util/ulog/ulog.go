package ulog

import (
	"bytes"
	"strings"

	"github.com/fatih/color"
	"github.com/gosuri/uitable"
	"github.com/gosuri/uitable/util/strutil"
)

func Success(msg string) string {
	return PrefixedMsg1("(success)", color.FgHiCyan, msg, color.FgHiWhite, 100)
}

func Error(msg string) string {
	return PrefixedMsg("(error)", color.FgHiRed, msg, color.FgHiWhite, 80)
}

func Warn(msg string) string {
	return PrefixedMsg("(warn)", color.FgHiYellow, msg, color.FgWhite, 80)
}

func PrefixedMsg(label string, labelCol color.Attribute, msg string, msgColor color.Attribute, width uint) string {
	t := uitable.New().AddRow(color.New(labelCol).Sprint(label), color.New(msgColor).Sprint(msg))
	t.MaxColWidth = width
	t.Wrap = true
	return t.String()
}

func PrefixedMsg1(label string, labelCol color.Attribute, msg string, msgColor color.Attribute, width uint) string {
	var buf bytes.Buffer
	cell := &uitable.Cell{Width: width, Wrap: true, Data: msg}
	for i, line := range strings.Split(cell.String(), "\n") {
		if i == 0 {
			buf.WriteString(color.New(labelCol).Sprint(label))
			buf.WriteString(" ")
			buf.WriteString(color.New(msgColor).Sprint(line))
		} else {
			s := strutil.PadLeft(line, len(label)+len(line)+1, ' ')
			buf.WriteString(color.New(msgColor).Sprint(s))
		}
		buf.WriteString("\n")
	}
	return buf.String()
}
