package types

import (
	"database/sql"
)

// ConnectionParams contains parameters needed to connect to a database
type ConnectionParams struct {
	Host     string
	Port     string
	User     string
	Password string
	Database string
	Schema   string
}

// Column represents a database table column
type Column struct {
	Name         string
	Type         string
	Nullable     bool
	DefaultValue sql.NullString
	IsPrimaryKey bool
	ForeignKey   sql.NullString // Foreign key reference information
}

// Index represents a database index
type Index struct {
	Name       string
	Columns    []string
	Unique     bool
	PrimaryKey bool
}

// Table represents a database table structure
type Table struct {
	Name    string
	Schema  string
	Columns []Column
	Indexes []Index
}

// DatabaseConnector defines the interface for database interactions
type DatabaseConnector interface {
	// Connect establishes a connection to the database
	Connect(params ConnectionParams) error

	// Disconnect closes the database connection
	Disconnect() error

	// GetTables returns a list of tables in the specified schema
	GetTables(schema string) ([]string, error)

	// GetTableStructure returns the structure of the specified table
	GetTableStructure(schema, tableName string) (*Table, error)
}

// DatabaseConnectorFactory is a function type that creates a specific DatabaseConnector
type DatabaseConnectorFactory func() DatabaseConnector
