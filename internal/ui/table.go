package ui

import (
	"strings"

	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/jedib0t/go-pretty/v6/text"

	"sir/internal/styles"
	"sir/internal/types"
)

func statusText(r types.Row) string {
	switch r.Status {
	case types.StatusRunning:
		return styles.Green("● running")
	case types.StatusStopped:
		return styles.Red("○ stopped")
	case types.StatusOther:
		return styles.Yellow("○ %s", r.State)
	case types.StatusError:
		return styles.Yellow("! parse error")
	}
	return "-"
}

func buildTableRow(r types.Row, cfg types.ScanConfig) table.Row {
	row := table.Row{
		r.Num,
		styles.Cyan(r.Folder),
		styles.Dim(r.Compose),
		r.Service,
		styles.Dim(r.ContainerID),
		r.Uptime,
		statusText(r),
	}
	if cfg.Technical {
		row = append(row, styles.Dim(r.Image), styles.Dim(r.Ports))
	}
	return row
}

func RenderTable(rows []types.Row, cfg types.ScanConfig, cursor int, selected map[int]bool) string {
	t := table.NewWriter()
	hdr := table.Row{" ", "#", "Folder", "Compose File", "Service", "Container ID", "Uptime", "Status"}
	if cfg.Technical {
		hdr = append(hdr, "Image", "Ports")
	}
	t.AppendHeader(hdr)
	for i, r := range rows {
		isSel := selected != nil && selected[r.Num]
		isCur := i == cursor
		marker := " "
		switch {
		case isSel && isCur:
			marker = styles.Cyan("✓")
		case isSel:
			marker = styles.Green("✓")
		case isCur:
			marker = styles.Cyan(">")
		}
		t.AppendRow(append(table.Row{marker}, buildTableRow(r, cfg)...))
	}
	style := table.StyleLight
	style.Color.Header = text.Colors{text.FgCyan, text.Bold}
	style.Color.Border = text.Colors{text.FgHiBlack}
	style.Color.Separator = text.Colors{text.FgHiBlack}
	t.SetStyle(style)
	t.Style().Options.SeparateRows = false
	return t.Render()
}

func FilterRows(rows []types.Row, q string) []types.Row {
	if q == "" {
		return rows
	}
	q = strings.ToLower(q)
	var out []types.Row
	for _, r := range rows {
		if strings.Contains(strings.ToLower(r.Folder), q) ||
			strings.Contains(strings.ToLower(r.Compose), q) ||
			strings.Contains(strings.ToLower(r.Service), q) ||
			strings.Contains(strings.ToLower(r.State), q) ||
			strings.Contains(strings.ToLower(r.ContainerID), q) ||
			strings.Contains(strings.ToLower(r.Image), q) {
			out = append(out, r)
		}
	}
	return out
}
