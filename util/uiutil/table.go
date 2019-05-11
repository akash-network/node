package uiutil

import "github.com/gosuri/uitable"

type ListTable struct {
	headers []string
	rows    [][]interface{}
}

func NewListTable() *ListTable {
	return &ListTable{}
}

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

func (t *ListTable) AddHeader(header ...string) *ListTable {
	t.headers = append(t.headers, header...)
	return t
}

func (t *ListTable) AddRow(row ...interface{}) *ListTable {
	t.rows = append(t.rows, row)
	return t
}
