package main

import (
	"strings"

	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/jedib0t/go-pretty/v6/text"
)

func statusText(r Row) string {
	switch r.Status {
	case StatusRunning:
		return green("● running")
	case StatusStopped:
		return red("○ stopped")
	case StatusOther:
		return yellow("○ %s", r.State)
	case StatusError:
		return yellow("! parse error")
	}
	return "-"
}

func buildTableRow(r Row, cfg ScanConfig) table.Row {
	row := table.Row{
		r.Num,
		cyan(r.Folder),
		dim(r.Compose),
		r.Service,
		dim(r.ContainerID),
		r.Uptime,
		statusText(r),
	}
	if cfg.Technical {
		row = append(row, dim(r.Image), dim(r.Ports))
	}
	return row
}

func renderTable(rows []Row, cfg ScanConfig, cursor int, selected map[int]bool) string {
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
			marker = cyan("✓")
		case isSel:
			marker = green("✓")
		case isCur:
			marker = cyan(">")
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

func filterRows(rows []Row, q string) []Row {
	if q == "" {
		return rows
	}
	q = strings.ToLower(q)
	var out []Row
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
