package uiutil

import "github.com/gosuri/uitable"

// ListTable stores list of headers and rows
type ListTable struct {
	headers []string
	rows    [][]interface{}
}

// NewListTable returns new ListTable instance
func NewListTable() *ListTable {
	return &ListTable{}
}

// UITable implements ListTable interface
func (t *ListTable) UITable() *uitable.Table {
	var heads []interface{}
	for _, head := range t.headers {
		head = NewTitle(head).WithUnderliner("-").String()
		heads = append(heads, head)
	}
	out := uitable.New().AddRow(heads...)
	for _, row := range t.rows {
		out.AddRow(row...)
	}
	return out
}

// AddHeader appends header to ListTable headers and return ListTable
func (t *ListTable) AddHeader(header ...string) *ListTable {
	t.headers = append(t.headers, header...)
	return t
}

// AddRow appends row to ListTable rows and return ListTable
func (t *ListTable) AddRow(row ...interface{}) *ListTable {
	t.rows = append(t.rows, row)
	return t
}
