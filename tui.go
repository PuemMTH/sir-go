package main

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/fatih/color"
)

type tickMsg time.Time

func tickCmd(d time.Duration) tea.Cmd {
	return tea.Tick(d, func(t time.Time) tea.Msg { return tickMsg(t) })
}

type refreshMsg struct {
	data scanResult
	at   time.Time
}

type execDoneMsg struct{ err error }

type bulkDoneMsg struct {
	done, failed int
	action       string
}

type clearStatusMsg struct{}

func clearStatusAfter(d time.Duration) tea.Cmd {
	return tea.Tick(d, func(time.Time) tea.Msg { return clearStatusMsg{} })
}

type tuiModel struct {
	cfg        ScanConfig
	targetPath string
	ctx        context.Context
	cli        *client.Client
	interval   time.Duration

	data       scanResult
	lastUpdate time.Time

	cursor   int
	selected map[int]bool

	statusMsg string

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
		input:    ti,
		selected: make(map[int]bool),
	}
}

func (m tuiModel) Init() tea.Cmd {
	return tea.Batch(m.refresh(), tickCmd(m.interval))
}

func (m tuiModel) refresh() tea.Cmd {
	return func() tea.Msg { return m.doRefresh() }
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

func (m tuiModel) selectedIDs() []string {
	var ids []string
	for _, r := range m.data.rows {
		if m.selected[r.Num] && r.FullContainerID != "" {
			ids = append(ids, r.FullContainerID)
		}
	}
	return ids
}

func (m tuiModel) bulkStopCmd(ids []string) tea.Cmd {
	return func() tea.Msg {
		done, failed := 0, 0
		for _, id := range ids {
			if err := m.cli.ContainerStop(m.ctx, id, container.StopOptions{}); err != nil {
				failed++
			} else {
				done++
			}
		}
		return bulkDoneMsg{done: done, failed: failed, action: "stopped"}
	}
}

func (m tuiModel) bulkRestartCmd(ids []string) tea.Cmd {
	return func() tea.Msg {
		done, failed := 0, 0
		for _, id := range ids {
			if err := m.cli.ContainerRestart(m.ctx, id, container.StopOptions{}); err != nil {
				failed++
			} else {
				done++
			}
		}
		return bulkDoneMsg{done: done, failed: failed, action: "restarted"}
	}
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
				m.cursor = 0
				m.updateViewport()
				return m, nil
			case "enter":
				m.searchMode = false
				m.cursor = 0
				m.input.Blur()
				m.updateViewport()
				return m, nil
			default:
				var cmd tea.Cmd
				m.input, cmd = m.input.Update(msg)
				m.cursor = 0
				m.updateViewport()
				return m, cmd
			}
		}

		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit

		case "up":
			if m.cursor > 0 {
				m.cursor--
				m.updateViewport()
			}
			return m, nil

		case "down":
			filtered := filterRows(m.data.rows, m.input.Value())
			if m.cursor < len(filtered)-1 {
				m.cursor++
				m.updateViewport()
			}
			return m, nil

		case " ":
			filtered := filterRows(m.data.rows, m.input.Value())
			if m.cursor < len(filtered) {
				num := filtered[m.cursor].Num
				if m.selected[num] {
					delete(m.selected, num)
				} else {
					m.selected[num] = true
				}
				m.updateViewport()
			}
			return m, nil

		case "s":
			filtered := filterRows(m.data.rows, m.input.Value())
			if m.cursor < len(filtered) {
				r := filtered[m.cursor]
				if r.Status == StatusRunning && r.FullContainerID != "" {
					shellCmd := exec.Command("docker", "exec", "-it", r.FullContainerID, "sh")
					return m, tea.ExecProcess(shellCmd, func(err error) tea.Msg {
						return execDoneMsg{err: err}
					})
				}
				m.statusMsg = yellow("  ⚠  '%s' is not running", r.Service)
				return m, clearStatusAfter(3 * time.Second)
			}
			return m, nil

		case "S":
			ids := m.selectedIDs()
			if len(ids) == 0 {
				m.statusMsg = yellow("  ⚠  No services selected — use space to select")
				return m, clearStatusAfter(3 * time.Second)
			}
			m.statusMsg = dim("  Stopping %d service(s)...", len(ids))
			return m, m.bulkStopCmd(ids)

		case "R":
			ids := m.selectedIDs()
			if len(ids) == 0 {
				m.statusMsg = yellow("  ⚠  No services selected — use space to select")
				return m, clearStatusAfter(3 * time.Second)
			}
			m.statusMsg = dim("  Restarting %d service(s)...", len(ids))
			return m, m.bulkRestartCmd(ids)

		case "/":
			m.searchMode = true
			m.input.Focus()
			return m, textinput.Blink

		case "t":
			m.cfg.Technical = !m.cfg.Technical
			m.updateViewport()
			return m, nil

		case "f":
			m.cfg.FullPath = !m.cfg.FullPath
			cmds = append(cmds, m.refresh())
			return m, tea.Batch(cmds...)
		}

	case execDoneMsg:
		if msg.err != nil {
			m.statusMsg = red("  ✗  Shell exited: %v", msg.err)
		} else {
			m.statusMsg = green("  ✓  Shell session ended")
		}
		m.updateViewport()
		return m, clearStatusAfter(3 * time.Second)

	case bulkDoneMsg:
		m.selected = make(map[int]bool)
		if msg.failed > 0 {
			m.statusMsg = yellow("  ⚠  %s: %d done, %d failed", msg.action, msg.done, msg.failed)
		} else {
			m.statusMsg = green("  ✓  %s %d service(s)", msg.action, msg.done)
		}
		m.updateViewport()
		return m, tea.Batch(clearStatusAfter(3*time.Second), m.refresh())

	case clearStatusMsg:
		m.statusMsg = ""
		m.updateViewport()
		return m, nil

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
		// clamp cursor in case rows shrank
		filtered := filterRows(m.data.rows, m.input.Value())
		if m.cursor >= len(filtered) && len(filtered) > 0 {
			m.cursor = len(filtered) - 1
		}
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
	if m.statusMsg != "" {
		n++
	}
	return n
}

func (m *tuiModel) updateViewport() {
	filtered := filterRows(m.data.rows, m.input.Value())
	if m.cursor >= len(filtered) && len(filtered) > 0 {
		m.cursor = len(filtered) - 1
	}
	if len(filtered) == 0 {
		m.viewport.SetContent(lgDim.Render("  No matching services"))
		return
	}
	m.viewport.SetContent(renderTable(filtered, m.cfg, m.cursor, m.selected))
	// keep cursor row visible: header takes 3 lines (top border + header + separator)
	cursorLine := m.cursor + 3
	if cursorLine < m.viewport.YOffset {
		m.viewport.SetYOffset(cursorLine)
	} else if cursorLine >= m.viewport.YOffset+m.viewport.Height {
		m.viewport.SetYOffset(cursorLine - m.viewport.Height + 1)
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

	selCount := len(m.selected)
	selInfo := ""
	if selCount > 0 {
		selInfo = fmt.Sprintf("   %s", cyan("selected: %d", selCount))
	}
	b.WriteString(fmt.Sprintf("\n  %s %d   %s   %s%s\n",
		lgBold.Render("Total:"), m.data.total,
		green("● Running: %d", m.data.run),
		red("○ Stopped: %d", m.data.stop),
		selInfo,
	))

	if m.statusMsg != "" {
		b.WriteString(m.statusMsg + "\n")
	}

	if m.searchMode {
		b.WriteString(fmt.Sprintf("  🔍 %s\n", m.input.View()))
	} else if v := m.input.Value(); v != "" {
		b.WriteString(fmt.Sprintf("  🔍 %s\n", lgDim.Render(v)))
	}

	b.WriteString(lgHelp.Render("↑↓ move  space select  s shell  S stop  R restart  / search  esc clear  t tech  f full-path  q quit"))

	return b.String()
}
