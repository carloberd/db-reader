package models

import "database/sql"

type Column struct {
	Name         string
	Type         string
	Nullable     bool
	DefaultValue sql.NullString
	IsPrimaryKey bool
	ForeignKey   sql.NullString
}
