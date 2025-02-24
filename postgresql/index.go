package postgresql

import (
	"database/sql"
	"fmt"
	"strings"

	t "github.com/carloberd/db-reader/types"
	_ "github.com/lib/pq" // PostgreSQL driver
)

// PostgresConnector implements the DatabaseConnector interface for PostgreSQL
type PostgresConnector struct {
	db *sql.DB
}

// Connect establishes a connection to the PostgreSQL database
func (pc *PostgresConnector) Connect(params t.ConnectionParams) error {
	// Create connection string
	dsn := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		params.Host, params.Port, params.User, params.Password, params.Database)

	// Open the connection
	var err error
	pc.db, err = sql.Open("postgres", dsn)
	if err != nil {
		return fmt.Errorf("failed to connect to database: %v", err)
	}

	// Test the connection
	err = pc.db.Ping()
	if err != nil {
		pc.db.Close()
		pc.db = nil
		return fmt.Errorf("failed to ping database: %v", err)
	}

	return nil
}

// Disconnect closes the database connection
func (pc *PostgresConnector) Disconnect() error {
	if pc.db != nil {
		err := pc.db.Close()
		pc.db = nil
		if err != nil {
			return fmt.Errorf("error closing database connection: %v", err)
		}
	}
	return nil
}

// GetTables returns a list of tables in the specified schema
func (pc *PostgresConnector) GetTables(schema string) ([]string, error) {
	if pc.db == nil {
		return nil, fmt.Errorf("not connected to database")
	}

	query := `
		SELECT 
			table_name 
		FROM 
			information_schema.tables 
		WHERE 
			table_schema = $1
		AND
			table_type = 'BASE TABLE'
		ORDER BY 
			table_name
	`

	rows, err := pc.db.Query(query, schema)
	if err != nil {
		return nil, fmt.Errorf("error querying tables: %v", err)
	}
	defer rows.Close()

	var tables []string
	for rows.Next() {
		var tableName string
		if err := rows.Scan(&tableName); err != nil {
			return nil, fmt.Errorf("error scanning table results: %v", err)
		}
		tables = append(tables, tableName)
	}

	return tables, nil
}

// formatDataType converts PostgreSQL type names to more concise formats
func formatDataType(pgType string) string {
	// Replace "character varying" with "varchar"
	pgType = strings.Replace(pgType, "character varying", "varchar", -1)

	// Other replacements
	pgType = strings.Replace(pgType, "character", "char", -1)
	pgType = strings.Replace(pgType, "double precision", "double", -1)

	return pgType
}

// GetTableStructure returns the structure of the specified table
func (pc *PostgresConnector) GetTableStructure(schema, tableName string) (*t.Table, error) {
	if pc.db == nil {
		return nil, fmt.Errorf("not connected to database")
	}

	// Check if table exists
	var exists bool
	checkQuery := `
		SELECT EXISTS (
			SELECT 1 
			FROM information_schema.tables 
			WHERE table_schema = $1 
			AND table_name = $2
		)
	`
	err := pc.db.QueryRow(checkQuery, schema, tableName).Scan(&exists)
	if err != nil {
		return nil, fmt.Errorf("error checking table existence: %v", err)
	}

	if !exists {
		return nil, fmt.Errorf("table '%s.%s' does not exist", schema, tableName)
	}

	table := &t.Table{
		Name:   tableName,
		Schema: schema,
	}

	// Get column information with foreign keys
	query := `
		SELECT 
			a.attname AS column_name,
			pg_catalog.format_type(a.atttypid, a.atttypmod) AS data_type,
			CASE WHEN a.attnotnull = false THEN true ELSE false END AS is_nullable,
			CASE WHEN a.atthasdef = true THEN pg_get_expr(adef.adbin, adef.adrelid) ELSE NULL END AS column_default,
			CASE WHEN prim.contype = 'p' THEN true ELSE false END AS is_primary_key,
			CASE 
				WHEN fk.conname IS NOT NULL THEN 
					fk_cl.relname || ' (' || att2.attname || ')'
				ELSE NULL 
			END AS foreign_key_ref
		FROM 
			pg_catalog.pg_attribute a
		LEFT JOIN 
			pg_catalog.pg_attrdef adef ON a.attrelid = adef.adrelid AND a.attnum = adef.adnum
		LEFT JOIN 
			pg_catalog.pg_constraint prim ON prim.conrelid = a.attrelid AND a.attnum = ANY(prim.conkey) AND prim.contype = 'p'
		LEFT JOIN 
			pg_catalog.pg_constraint fk ON fk.conrelid = a.attrelid AND a.attnum = ANY(fk.conkey) AND fk.contype = 'f'
		LEFT JOIN 
			pg_catalog.pg_class fk_cl ON fk.confrelid = fk_cl.oid
		LEFT JOIN 
			pg_catalog.pg_attribute att2 ON fk.confrelid = att2.attrelid AND 
			att2.attnum = ANY(fk.confkey) AND fk.conkey[array_position(fk.conkey, a.attnum)] = a.attnum AND 
			fk.confkey[array_position(fk.conkey, a.attnum)] = att2.attnum
		WHERE 
			a.attrelid = (SELECT oid FROM pg_catalog.pg_class WHERE relname = $1 AND 
						  relnamespace = (SELECT oid FROM pg_catalog.pg_namespace WHERE nspname = $2))
			AND a.attnum > 0
			AND NOT a.attisdropped
		ORDER BY 
			a.attnum
	`

	rows, err := pc.db.Query(query, tableName, schema)
	if err != nil {
		return nil, fmt.Errorf("error querying columns: %v", err)
	}
	defer rows.Close()

	for rows.Next() {
		var col t.Column
		var defaultValue sql.NullString
		var pgType string
		var foreignKeyRef sql.NullString

		err := rows.Scan(
			&col.Name,
			&pgType,
			&col.Nullable,
			&defaultValue,
			&col.IsPrimaryKey,
			&foreignKeyRef,
		)
		if err != nil {
			return nil, fmt.Errorf("error scanning column results: %v", err)
		}

		col.Type = formatDataType(pgType)
		col.DefaultValue = defaultValue
		col.ForeignKey = foreignKeyRef
		table.Columns = append(table.Columns, col)
	}

	// Get index information
	indexQuery := `
		SELECT
			i.relname AS index_name,
			a.attname AS column_name,
			ix.indisunique AS is_unique,
			ix.indisprimary AS is_primary
		FROM
			pg_catalog.pg_class t,
			pg_catalog.pg_class i,
			pg_catalog.pg_index ix,
			pg_catalog.pg_attribute a,
			pg_catalog.pg_namespace n
		WHERE
			t.oid = ix.indrelid
			AND i.oid = ix.indexrelid
			AND a.attrelid = t.oid
			AND a.attnum = ANY(ix.indkey)
			AND t.relkind = 'r'
			AND t.relname = $1
			AND n.oid = t.relnamespace
			AND n.nspname = $2
		ORDER BY
			i.relname, a.attnum
	`

	indexRows, err := pc.db.Query(indexQuery, tableName, schema)
	if err != nil {
		return nil, fmt.Errorf("error querying indexes: %v", err)
	}
	defer indexRows.Close()

	indexMap := make(map[string]*t.Index)

	for indexRows.Next() {
		var indexName, columnName string
		var isUnique, isPrimary bool

		err := indexRows.Scan(&indexName, &columnName, &isUnique, &isPrimary)
		if err != nil {
			return nil, fmt.Errorf("error scanning index results: %v", err)
		}

		if idx, exists := indexMap[indexName]; exists {
			idx.Columns = append(idx.Columns, columnName)
		} else {
			idx := &t.Index{
				Name:       indexName,
				Columns:    []string{columnName},
				Unique:     isUnique,
				PrimaryKey: isPrimary,
			}
			indexMap[indexName] = idx
		}
	}

	// Convert map to slice
	for _, idx := range indexMap {
		table.Indexes = append(table.Indexes, *idx)
	}

	return table, nil
}

// Implementation of factory method
func NewPostgresConnector() t.DatabaseConnector {
	return &PostgresConnector{}
}
