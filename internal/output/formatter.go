package output

import "io"

// Formatter defines how command output is rendered.
type Formatter interface {
	// Format writes a single data object.
	Format(w io.Writer, data any) error
	// FormatList writes tabular data with headers and rows.
	FormatList(w io.Writer, headers []string, rows [][]string) error
	// FormatPage writes a paginated list response. In JSON mode, data
	// (the native API structs) is wrapped with pagination metadata; in
	// text mode it renders headers+rows as a table. This preserves native
	// field names, types, and omitted fields in JSON output.
	FormatPage(w io.Writer, data any, headers []string, rows [][]string, meta PageMeta) error
}

// PageMeta carries pagination context so agents can iterate through
// result sets without parsing text or guessing when to stop.
type PageMeta struct {
	Page    int `json:"page"`
	PerPage int `json:"per_page"`
	HasNext bool `json:"has_next"`
}

// NewFormatter returns a Formatter for the given format string.
// Supported: "json", "text" (default).
func NewFormatter(format string) Formatter {
	switch format {
	case "json":
		return &JSONFormatter{}
	default:
		return &TextFormatter{}
	}
}
