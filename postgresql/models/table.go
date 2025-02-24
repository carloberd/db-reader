package models

type Table struct {
	Name    string
	Schema  string
	Columns []Column
	Indexes []Index
}