package main

import (
	"github.com/charmbracelet/lipgloss"
	"github.com/fatih/color"
)

var (
	cGreen  = color.New(color.FgGreen)
	cRed    = color.New(color.FgRed)
	cYellow = color.New(color.FgYellow, color.Bold)
	cCyan   = color.New(color.FgCyan, color.Bold)
	cBold   = color.New(color.Bold)
	cDim    = color.New(color.Faint)

	green  = cGreen.SprintfFunc()
	red    = cRed.SprintfFunc()
	yellow = cYellow.SprintfFunc()
	cyan   = cCyan.SprintfFunc()
	bold   = cBold.SprintfFunc()
	dim    = cDim.SprintfFunc()

	lgCyan = lipgloss.NewStyle().Foreground(lipgloss.Color("6")).Bold(true)
	lgBold = lipgloss.NewStyle().Bold(true)
	lgDim  = lipgloss.NewStyle().Faint(true)
	lgHelp = lipgloss.NewStyle().Faint(true).PaddingLeft(2)
)
