package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/docker/docker/client"
	"github.com/fatih/color"
)

type tickMsg time.Time

func tickCmd(d time.Duration) tea.Cmd {
	return tea.Tick(d, func(t time.Time) tea.Msg { return tickMsg(t) })
}

type tuiModel struct {
	cfg        ScanConfig
	targetPath string
	ctx        context.Context
	cli        *client.Client
	interval   time.Duration

	data       scanResult
	lastUpdate time.Time

	searchMode bool
	input      textinput.Model
	viewport   viewport.Model
	ready      bool
	width      int
	height     int
}

func newTUI(ctx context.Context, cli *client.Client, targetPath string, cfg ScanConfig, interval time.Duration) tuiModel {
	ti := textinput.New()
	ti.Placeholder = "type to filter..."
	ti.CharLimit = 64

	color.NoColor = false

	return tuiModel{
		cfg: cfg, targetPath: targetPath,
		ctx: ctx, cli: cli, interval: interval,
		input: ti,
	}
}

func (m tuiModel) Init() tea.Cmd {
	return tea.Batch(m.refresh(), tickCmd(m.interval))
}

func (m tuiModel) refresh() tea.Cmd {
	return func() tea.Msg { return m.doRefresh() }
}

type refreshMsg struct {
	data scanResult
	at   time.Time
}

func (m tuiModel) doRefresh() tea.Msg {
	var data scanResult
	if m.targetPath == "" {
		data = collectAllContainers(m.ctx, m.cli, m.cfg)
	} else {
		data = collectRows(m.ctx, m.cli, m.targetPath, m.cfg)
	}
	return refreshMsg{data: data, at: time.Now()}
}

func (m tuiModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		if m.searchMode {
			switch msg.String() {
			case "esc":
				m.searchMode = false
				m.input.SetValue("")
				m.input.Blur()
				m.updateViewport()
				return m, nil
			case "enter":
				m.searchMode = false
				m.input.Blur()
				return m, nil
			default:
				var cmd tea.Cmd
				m.input, cmd = m.input.Update(msg)
				m.updateViewport()
				return m, cmd
			}
		}
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "/":
			m.searchMode = true
			m.input.Focus()
			return m, textinput.Blink
		case "t":
			m.cfg.Technical = !m.cfg.Technical
			m.updateViewport()
			return m, nil
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		vpH := m.height - m.fixedLines()
		if vpH < 3 {
			vpH = 3
		}
		if !m.ready {
			m.viewport = viewport.New(m.width, vpH)
			m.ready = true
		} else {
			m.viewport.Width = m.width
			m.viewport.Height = vpH
		}
		m.updateViewport()

	case refreshMsg:
		m.data = msg.data
		m.lastUpdate = msg.at
		m.updateViewport()

	case tickMsg:
		cmds = append(cmds, m.refresh(), tickCmd(m.interval))
	}

	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(msg)
	cmds = append(cmds, cmd)
	return m, tea.Batch(cmds...)
}

func (m tuiModel) fixedLines() int {
	n := 9
	if m.searchMode || m.input.Value() != "" {
		n++
	}
	return n
}

func (m *tuiModel) updateViewport() {
	filtered := filterRows(m.data.rows, m.input.Value())
	if len(filtered) == 0 {
		m.viewport.SetContent(lgDim.Render("  No matching services"))
	} else {
		m.viewport.SetContent(renderTable(filtered, m.cfg))
	}
}

func (m tuiModel) View() string {
	if !m.ready {
		return "  Loading..."
	}

	var b strings.Builder

	b.WriteString("\n")
	b.WriteString(lgCyan.Render("  🐳  SIR - Service Inspector Reporter"))
	b.WriteString("\n\n")

	scanLabel := m.targetPath
	if scanLabel == "" {
		scanLabel = "(all Docker Compose containers)"
	}
	b.WriteString(fmt.Sprintf("  %s %s    %s every %ds    %s %s\n\n",
		lgBold.Render("Scanning:"), lgDim.Render(scanLabel),
		lgBold.Render("Refresh:"), int(m.interval.Seconds()),
		lgBold.Render("Updated:"), m.lastUpdate.Format("15:04:05"),
	))

	b.WriteString(m.viewport.View())
	b.WriteString("\n")

	b.WriteString(fmt.Sprintf("\n  %s %d   %s   %s\n",
		lgBold.Render("Total:"), m.data.total,
		green("● Running: %d", m.data.run),
		red("○ Stopped: %d", m.data.stop),
	))

	if m.searchMode {
		b.WriteString(fmt.Sprintf("  🔍 %s\n", m.input.View()))
	} else if v := m.input.Value(); v != "" {
		b.WriteString(fmt.Sprintf("  🔍 %s\n", lgDim.Render(v)))
	}

	b.WriteString(lgHelp.Render("↑↓ scroll  / search  esc clear  t toggle tech  q quit"))

	return b.String()
}
