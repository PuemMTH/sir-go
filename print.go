package main

import (
	"fmt"
	"os"

	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/jedib0t/go-pretty/v6/text"
)

func printOneShot(targetPath string, cfg ScanConfig, res scanResult) {
	fmt.Println()
	cCyan.Println("  🐳  SIR - Service Inspector Reporter")
	fmt.Printf("\n  %s %s\n\n", bold("Scanning:"), dim(targetPath))

	if len(res.rows) == 0 {
		cYellow.Println("  No docker-compose files found")
		return
	}

	t := table.NewWriter()
	t.SetOutputMirror(os.Stdout)
	hdr := table.Row{"#", "Folder", "Compose File", "Service", "Container ID", "Uptime", "Status"}
	if cfg.Technical {
		hdr = append(hdr, "Image", "Ports")
	}
	t.AppendHeader(hdr)
	for _, r := range res.rows {
		t.AppendRow(buildTableRow(r, cfg))
	}
	style := table.StyleLight
	style.Color.Header = text.Colors{text.FgCyan, text.Bold}
	style.Color.Border = text.Colors{text.FgHiBlack}
	style.Color.Separator = text.Colors{text.FgHiBlack}
	t.SetStyle(style)
	t.Style().Options.SeparateRows = false
	t.Render()

	fmt.Printf("\n  %s %d   %s   %s\n\n",
		bold("Total:"), res.total,
		green("● Running: %d", res.run),
		red("○ Stopped: %d", res.stop),
	)
}
