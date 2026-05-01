package output

import (
	"encoding/json"
	"io"
)

// JSONFormatter outputs data as pretty-printed JSON.
type JSONFormatter struct{}

func (f *JSONFormatter) Format(w io.Writer, data any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(data)
}

func (f *JSONFormatter) FormatList(w io.Writer, headers []string, rows [][]string) error {
	items := make([]map[string]string, len(rows))
	for i, row := range rows {
		item := make(map[string]string)
		for j, h := range headers {
			if j < len(row) {
				item[h] = row[j]
			}
		}
		items[i] = item
	}
	return f.Format(w, items)
}

func (f *JSONFormatter) FormatPage(w io.Writer, headers []string, rows [][]string, meta PageMeta) error {
	items := make([]map[string]string, len(rows))
	for i, row := range rows {
		item := make(map[string]string)
		for j, h := range headers {
			if j < len(row) {
				item[h] = row[j]
			}
		}
		items[i] = item
	}
	envelope := struct {
		Items   []map[string]string `json:"items"`
		Page    int                 `json:"page"`
		PerPage int                 `json:"per_page"`
		HasNext bool                `json:"has_next"`
	}{
		Items:   items,
		Page:    meta.Page,
		PerPage: meta.PerPage,
		HasNext: meta.HasNext,
	}
	return f.Format(w, envelope)
}
