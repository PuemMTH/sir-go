package styles

import (
	"github.com/charmbracelet/lipgloss"
	"github.com/fatih/color"
)

var (
	CGreen  = color.New(color.FgGreen)
	CRed    = color.New(color.FgRed)
	CYellow = color.New(color.FgYellow, color.Bold)
	CCyan   = color.New(color.FgCyan, color.Bold)
	CBold   = color.New(color.Bold)
	CDim    = color.New(color.Faint)

	Green  = CGreen.SprintfFunc()
	Red    = CRed.SprintfFunc()
	Yellow = CYellow.SprintfFunc()
	Cyan   = CCyan.SprintfFunc()
	Bold   = CBold.SprintfFunc()
	Dim    = CDim.SprintfFunc()

	LgCyan = lipgloss.NewStyle().Foreground(lipgloss.Color("6")).Bold(true)
	LgBold = lipgloss.NewStyle().Bold(true)
	LgDim  = lipgloss.NewStyle().Faint(true)
	LgHelp = lipgloss.NewStyle().Faint(true).PaddingLeft(2)
)
