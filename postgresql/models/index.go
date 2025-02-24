package models

type Index struct {
	Name       string
	Columns    []string
	Unique     bool
	PrimaryKey bool
}