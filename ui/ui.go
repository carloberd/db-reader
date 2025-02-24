package ui

import (
	"fmt"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/carloberd/db-reader/postgresql"
	t "github.com/carloberd/db-reader/types"
)

// DBInspector is the main application structure
type DBInspector struct {
	app       fyne.App
	window    fyne.Window
	connector t.DatabaseConnector
	connInfo  *t.ConnectionParams

	// Main widgets
	tableList    *widget.List
	statusLabel  *widget.Label
	tableDetails *widget.TextGrid

	// Data
	tables        []string
	selectedTable *t.Table
}

// NewDBInspector creates a new database inspector
func NewDBInspector(a fyne.App) *DBInspector {
	w := a.NewWindow("PostgreSQL Database Inspector")

	inspector := &DBInspector{
		app:         a,
		window:      w,
		statusLabel: widget.NewLabel("Not connected"),
		connector:   postgresql.NewPostgresConnector(),
	}

	inspector.setupUI()

	return inspector
}

// setupUI initializes the user interface
func (di *DBInspector) setupUI() {
	// New connection button
	newConnBtn := widget.NewButtonWithIcon("New Connection", theme.ContentAddIcon(), func() {
		di.showConnectionDialog()
	})

	// Table list (initially empty)
	di.tableList = widget.NewList(
		func() int { return len(di.tables) },
		func() fyne.CanvasObject { return widget.NewLabel("Table name") },
		func(id widget.ListItemID, obj fyne.CanvasObject) {
			obj.(*widget.Label).SetText(di.tables[id])
		},
	)

	// When user selects a table
	di.tableList.OnSelected = func(id widget.ListItemID) {
		if id < len(di.tables) {
			di.loadTableDetails(di.tables[id])
		}
	}

	// Table details area
	di.tableDetails = widget.NewTextGrid()

	// Main layout
	split := container.NewHSplit(
		container.NewBorder(
			container.NewVBox(
				widget.NewLabel("Available tables:"),
				widget.NewSeparator(),
			),
			nil, nil, nil,
			di.tableList,
		),
		container.NewBorder(
			nil, nil, nil, nil,
			container.NewScroll(di.tableDetails),
		),
	)
	split.SetOffset(0.3) // 30% left, 70% right

	// Overall layout
	content := container.NewBorder(
		container.NewVBox(
			container.NewHBox(
				newConnBtn,
				layout.NewSpacer(),
				di.statusLabel,
			),
			widget.NewSeparator(),
		),
		nil, nil, nil,
		split,
	)

	di.window.SetContent(content)
	di.window.Resize(fyne.NewSize(900, 600))
}

// showConnectionDialog displays the connection dialog
func (di *DBInspector) showConnectionDialog() {
	// Create input fields for connection parameters
	hostEntry := widget.NewEntry()
	hostEntry.SetPlaceHolder("localhost")

	portEntry := widget.NewEntry()
	portEntry.SetPlaceHolder("5432")

	userEntry := widget.NewEntry()
	userEntry.SetPlaceHolder("postgres")

	passEntry := widget.NewPasswordEntry()

	dbEntry := widget.NewEntry()

	schemaEntry := widget.NewEntry()
	schemaEntry.SetText("public")

	// Populate fields if there's already a connection
	if di.connInfo != nil {
		hostEntry.SetText(di.connInfo.Host)
		portEntry.SetText(di.connInfo.Port)
		userEntry.SetText(di.connInfo.User)
		passEntry.SetText(di.connInfo.Password)
		dbEntry.SetText(di.connInfo.Database)
		schemaEntry.SetText(di.connInfo.Schema)
	}

	// Create the form
	form := &widget.Form{
		Items: []*widget.FormItem{
			{Text: "Host", Widget: hostEntry},
			{Text: "Port", Widget: portEntry},
			{Text: "User", Widget: userEntry},
			{Text: "Password", Widget: passEntry},
			{Text: "Database", Widget: dbEntry},
			{Text: "Schema", Widget: schemaEntry},
		},
		OnSubmit: func() {
			// Collect connection parameters
			host := hostEntry.Text
			if host == "" {
				host = "localhost"
			}

			port := portEntry.Text
			if port == "" {
				port = "5432"
			}

			user := userEntry.Text
			if user == "" {
				user = "postgres"
			}

			password := passEntry.Text
			database := dbEntry.Text
			schema := schemaEntry.Text
			if schema == "" {
				schema = "public"
			}

			// Verify database name is provided
			if database == "" {
				dialog.ShowError(fmt.Errorf("database name is required"), di.window)
				return
			}

			// Store parameters
			di.connInfo = &t.ConnectionParams{
				Host:     host,
				Port:     port,
				User:     user,
				Password: password,
				Database: database,
				Schema:   schema,
			}

			// Attempt connection
			di.connect()
		},
	}

	// Show the dialog
	dialog.ShowCustom("Connect to Database", "Cancel", form, di.window)
}

// connect establishes a database connection
func (di *DBInspector) connect() {
	// Close existing connection, if any
	if di.connector != nil {
		di.connector.Disconnect()
	}

	// Update status
	di.statusLabel.SetText("Connecting...")

	// Connect to database
	err := di.connector.Connect(*di.connInfo)
	if err != nil {
		dialog.ShowError(fmt.Errorf("connection error: %v", err), di.window)
		di.statusLabel.SetText("Connection error")
		return
	}

	// Connection successful
	di.statusLabel.SetText(fmt.Sprintf("Connected to %s", di.connInfo.Database))

	// Load table list
	di.loadTableList()
}

// loadTableList fetches and displays the list of tables
func (di *DBInspector) loadTableList() {
	// Get tables from database
	var err error
	di.tables, err = di.connector.GetTables(di.connInfo.Schema)
	if err != nil {
		dialog.ShowError(fmt.Errorf("error loading tables: %v", err), di.window)
		return
	}

	// Update the list widget
	di.tableList.Refresh()
}

// loadTableDetails loads and displays details of the selected table
func (di *DBInspector) loadTableDetails(tableName string) {
	// Get table structure from database
	table, err := di.connector.GetTableStructure(di.connInfo.Schema, tableName)
	if err != nil {
		dialog.ShowError(fmt.Errorf("error loading table details: %v", err), di.window)
		return
	}

	di.selectedTable = table

	// Format table details
	details := di.formatTableDetails(table)

	// Update the TextGrid
	di.tableDetails.SetText(details)
}

// formatTableDetails formats table structure as a string
func (di *DBInspector) formatTableDetails(table *t.Table) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("Table: %s.%s\n\n", table.Schema, table.Name))

	sb.WriteString("COLUMNS:\n")
	sb.WriteString(fmt.Sprintf("%-20s %-25s %-10s %-25s %-10s %-25s\n",
		"Name", "Type", "Nullable", "Default", "PrimaryKey", "Foreign Key"))
	sb.WriteString(strings.Repeat("-", 115) + "\n")

	for _, col := range table.Columns {
		defaultVal := "NULL"
		if col.DefaultValue.Valid {
			defaultVal = col.DefaultValue.String
		}

		foreignKey := ""
		if col.ForeignKey.Valid {
			foreignKey = col.ForeignKey.String
		}

		sb.WriteString(fmt.Sprintf("%-20s %-25s %-10t %-25s %-10t %-25s\n",
			col.Name, col.Type, col.Nullable, defaultVal, col.IsPrimaryKey, foreignKey))
	}

	if len(table.Indexes) > 0 {
		sb.WriteString("\nINDEXES:\n")
		sb.WriteString(fmt.Sprintf("%-30s %-40s %-10s %-10s\n", "Name", "Columns", "Unique", "PrimaryKey"))
		sb.WriteString(strings.Repeat("-", 90) + "\n")

		for _, idx := range table.Indexes {
			columns := strings.Join(idx.Columns, ", ")
			sb.WriteString(fmt.Sprintf("%-30s %-40s %-10t %-10t\n",
				idx.Name, columns, idx.Unique, idx.PrimaryKey))
		}
	}

	return sb.String()
}

// Show displays the application window
func (di *DBInspector) Show() error {
	di.window.ShowAndRun()
	return nil
}
