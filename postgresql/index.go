package postgresql

import (
	"database/sql"
	"fmt"
	"strings"

	"github.com/carloberd/db-reader/postgresql/models"
)

func GetTableList(db *sql.DB, schema string) ([]string, error) {
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

	rows, err := db.Query(query, schema)
	if err != nil {
		return nil, fmt.Errorf("an error occurred fetching tables: %v", err)
	}
	defer rows.Close()

	var tables []string
	for rows.Next() {
		var tableName string
		if err := rows.Scan(&tableName); err != nil {
			return nil, fmt.Errorf("an error occurred scanning table: %v", err)
		}
		tables = append(tables, tableName)
	}

	return tables, nil
}

func GetTableStructure(db *sql.DB, schema, tableName string) (*models.Table, error) {
	// Verifica prima se la tabella esiste
	var exists bool
	checkQuery := `
		SELECT EXISTS (
			SELECT 1 
			FROM information_schema.tables 
			WHERE table_schema = $1 
			AND table_name = $2
		)
	`
	err := db.QueryRow(checkQuery, schema, tableName).Scan(&exists)
	if err != nil {
		return nil, fmt.Errorf("an error occurred checking table existence: %v", err)
	}

	if !exists {
		return nil, fmt.Errorf("the table '%s.%s' does not exist", schema, tableName)
	}

	table := &models.Table{
		Name:   tableName,
		Schema: schema,
	}

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

	rows, err := db.Query(query, tableName, schema)
	if err != nil {
		return nil, fmt.Errorf("an error occurred fetching columns: %v", err)
	}
	defer rows.Close()

	for rows.Next() {
		var col models.Column
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
			return nil, fmt.Errorf("an error occurred scanning columns: %v", err)
		}

		col.Type = formatDataType(pgType)
		col.DefaultValue = defaultValue
		col.ForeignKey = foreignKeyRef
		table.Columns = append(table.Columns, col)
	}

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

	indexRows, err := db.Query(indexQuery, tableName, schema)
	if err != nil {
		return nil, fmt.Errorf("an error occurred fetching indexes: %v", err)
	}
	defer indexRows.Close()

	indexMap := make(map[string]*models.Index)

	for indexRows.Next() {
		var indexName, columnName string
		var isUnique, isPrimary bool

		err := indexRows.Scan(&indexName, &columnName, &isUnique, &isPrimary)
		if err != nil {
			return nil, fmt.Errorf("an error occurred scanning indexes: %v", err)
		}

		if idx, exists := indexMap[indexName]; exists {
			idx.Columns = append(idx.Columns, columnName)
		} else {
			idx := &models.Index{
				Name:       indexName,
				Columns:    []string{columnName},
				Unique:     isUnique,
				PrimaryKey: isPrimary,
			}
			indexMap[indexName] = idx
		}
	}

	// Converts indexes map to slice
	for _, idx := range indexMap {
		table.Indexes = append(table.Indexes, *idx)
	}

	return table, nil
}

// Format PostgreSQL type in a more compact form
func formatDataType(pgType string) string {
	// Replace "character varying" with "varchar"
	pgType = strings.Replace(pgType, "character varying", "varchar", -1)

	// More replacements
	pgType = strings.Replace(pgType, "character", "char", -1)
	pgType = strings.Replace(pgType, "double precision", "double", -1)
	pgType = strings.Replace(pgType, "timestamp without time zone", "timestamp", -1)
	pgType = strings.Replace(pgType, "timestamp with time zone", "timestampz", -1)

	return pgType
}

func PrintTableStructure(table *models.Table) {
	fmt.Printf("\nTable structure '%s.%s':\n\n", table.Schema, table.Name)

	fmt.Println("COLONNE:")
	fmt.Printf("%-20s %-25s %-10s %-25s %-10s %-25s\n",
		"Name", "Type", "Nullable", "Default", "Primary Key", "Foreign Key")
	fmt.Println(strings.Repeat("-", 115))

	for _, col := range table.Columns {
		defaultVal := "NULL"
		if col.DefaultValue.Valid {
			defaultVal = col.DefaultValue.String
		}

		foreignKey := ""
		if col.ForeignKey.Valid {
			foreignKey = col.ForeignKey.String
		}

		fmt.Printf("%-20s %-25s %-10t %-25s %-10t %-25s\n",
			col.Name, col.Type, col.Nullable, defaultVal, col.IsPrimaryKey, foreignKey)
	}

	if len(table.Indexes) > 0 {
		fmt.Println("\nINDEXES:")
		fmt.Printf("%-30s %-40s %-10s %-10s\n", "Name", "Columns", "Unique", "Primary Key")
		fmt.Println(strings.Repeat("-", 90))

		for _, idx := range table.Indexes {
			columns := strings.Join(idx.Columns, ", ")
			fmt.Printf("%-30s %-40s %-10t %-10t\n",
				idx.Name, columns, idx.Unique, idx.PrimaryKey)
		}
	}
}
