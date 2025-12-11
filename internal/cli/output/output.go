package output

import (
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"
	"github.com/muesli/termenv"
)

// Column defines how to render a specific field from the data for the Table view.
type Column[T any] struct {
	// Header is the column title.
	Header string
	// Accessor extracts the value from the row object and formats it for the table.
	// If Accessor is provided, it takes precedence over Field.
	Accessor func(row T) string
	// Field is the name of the struct field to extract value from.
	// It is used if Accessor is nil.
	Field string
}

// Print renders the data in the requested format.
// data: must be a slice of structs (e.g., []Image, []Machine) for "table" format,
// or any JSON-marshalable object for "json" format.
// format: "table" or "json" (default is "table").
func Print[T any](data any, columns []Column[T], format string) error {
	switch format {
	case "json":
		return printJSON(data)
	default:
		return printTable(data, columns)
	}
}

func printJSON(data any) error {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(data)
}

func printTable[T any](data any, columns []Column[T]) error {
	// Use reflection to verify data is a slice and iterate over it.
	v := reflect.ValueOf(data)
	if v.Kind() != reflect.Slice {
		return fmt.Errorf("output.Print: data must be a slice, got %T", data)
	}

	// 1. Setup Lipgloss Table
	t := table.New().
		Border(lipgloss.Border{}).
		BorderTop(false).
		BorderBottom(false).
		BorderLeft(false).
		BorderRight(false).
		BorderHeader(false).
		BorderColumn(false).
		StyleFunc(func(row, col int) lipgloss.Style {
			if row == table.HeaderRow {
				return lipgloss.NewStyle().Bold(true).PaddingRight(3)
			}
			// Regular style for data rows with padding.
			return lipgloss.NewStyle().PaddingRight(3)
		})

	// 2. Add Headers
	var headers []string
	for _, col := range columns {
		headers = append(headers, col.Header)
	}
	t.Headers(headers...)

	// 3. Add Rows
	for i := 0; i < v.Len(); i++ {
		rowItem := v.Index(i).Interface().(T)
		var rowStrings []string
		for _, col := range columns {
			val := getValue(rowItem, col)
			rowStrings = append(rowStrings, val)
		}
		t.Row(rowStrings...)
	}

	// 4. Print
	if v.Len() == 0 {
		return nil
	}
	fmt.Println(t.String())
	return nil
}

func getValue[T any](row T, col Column[T]) string {
	if col.Accessor != nil {
		return col.Accessor(row)
	}

	if col.Field == "" {
		return ""
	}

	v := reflect.ValueOf(row)
	// If it's a pointer, dereference it
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}

	f := v.FieldByName(col.Field)
	if !f.IsValid() {
		return ""
	}

	// Handle pointers
	if f.Kind() == reflect.Ptr {
		if f.IsNil() {
			return "-"
		}
		f = f.Elem()
	}

	switch f.Kind() {
	case reflect.Slice:
		if f.Type().Elem().Kind() == reflect.String {
			// Handle []string
			var parts []string
			for i := 0; i < f.Len(); i++ {
				parts = append(parts, fmt.Sprint(f.Index(i).Interface()))
			}
			return strings.Join(parts, ", ")
		}
		return fmt.Sprint(f.Interface())
	default:
		return fmt.Sprint(f.Interface())
	}
}

// PillStyle returns a standard style for "pills" (like platform tags).
func PillStyle() lipgloss.Style {
	style := lipgloss.NewStyle().
		BorderForeground(lipgloss.Color("152")).
		Foreground(lipgloss.Color("0")).
		Background(lipgloss.Color("152"))

	if lipgloss.ColorProfile() != termenv.Ascii {
		style = style.Border(lipgloss.Border{Left: "", Right: ""}, false, true, false, true)
	}
	return style
}
