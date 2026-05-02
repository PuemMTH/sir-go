package ui

import (
	"fmt"
	"os"

	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/jedib0t/go-pretty/v6/text"

	"sir/internal/styles"
	"sir/internal/types"
)

func PrintOneShot(targetPath string, cfg types.ScanConfig, res types.ScanResult) {
	fmt.Println()
	styles.CCyan.Println("  🐳  SIR - Service Inspector Reporter")
	fmt.Printf("\n  %s %s\n\n", styles.Bold("Scanning:"), styles.Dim(targetPath))

	if len(res.Rows) == 0 {
		styles.CYellow.Println("  No docker-compose files found")
		return
	}

	t := table.NewWriter()
	t.SetOutputMirror(os.Stdout)
	hdr := table.Row{"#", "Folder", "Compose File", "Service", "Container ID", "Uptime", "Status"}
	if cfg.Technical {
		hdr = append(hdr, "Image", "Ports")
	}
	t.AppendHeader(hdr)
	for _, r := range res.Rows {
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
		styles.Bold("Total:"), res.Total,
		styles.Green("● Running: %d", res.Run),
		styles.Red("○ Stopped: %d", res.Stop),
	)
}
