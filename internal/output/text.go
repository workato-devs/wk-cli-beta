package output

import (
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
)

// TextFormatter outputs human-readable text with aligned columns.
type TextFormatter struct{}

func (f *TextFormatter) Format(w io.Writer, data any) error {
	_, err := fmt.Fprintf(w, "%v\n", data)
	return err
}

func (f *TextFormatter) FormatList(w io.Writer, headers []string, rows [][]string) error {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, strings.Join(headers, "\t"))
	for _, row := range rows {
		fmt.Fprintln(tw, strings.Join(row, "\t"))
	}
	return tw.Flush()
}

func (f *TextFormatter) FormatPage(w io.Writer, headers []string, rows [][]string, _ PageMeta) error {
	return f.FormatList(w, headers, rows)
}
