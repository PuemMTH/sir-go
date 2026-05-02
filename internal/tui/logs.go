package tui

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	"github.com/charmbracelet/lipgloss"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"

	"sir/internal/styles"
	"sir/internal/types"
)

// ── message types ─────────────────────────────────────────────────────────────

type logLineMsg    string
type execOutputMsg string
type cwdChangedMsg string
type olderLogsMsg struct{ lines []logLine }

// ── helpers ───────────────────────────────────────────────────────────────────

type logLine struct {
	ts   string
	text string
}

// splitDockerLogLine splits the RFC3339Nano timestamp prefix added by --timestamps.
func splitDockerLogLine(raw string) (ts, text string) {
	i := strings.Index(raw, " ")
	if i <= 0 || raw[0] < '0' || raw[0] > '9' {
		return "", raw
	}
	return raw[:i], raw[i+1:]
}

func subtractOneNano(ts string) string {
	t, err := time.Parse(time.RFC3339Nano, ts)
	if err != nil {
		return ts
	}
	return t.Add(-time.Nanosecond).UTC().Format(time.RFC3339Nano)
}

func isCdCommand(command string) (path string, ok bool) {
	if command == "cd" {
		return "", true
	}
	if rest, found := strings.CutPrefix(command, "cd "); found {
		return strings.TrimSpace(rest), true
	}
	return "", false
}

// sdkExec runs a command in the container via the Docker SDK and returns combined output.
func sdkExec(ctx context.Context, cli *client.Client, id, cwd string, cmd []string) (string, error) {
	execResp, err := cli.ContainerExecCreate(ctx, id, container.ExecOptions{
		Cmd:          cmd,
		AttachStdout: true,
		AttachStderr: true,
		WorkingDir:   cwd,
	})
	if err != nil {
		return "", err
	}
	attach, err := cli.ContainerExecAttach(ctx, execResp.ID, container.ExecStartOptions{})
	if err != nil {
		return "", err
	}
	defer attach.Close()
	var out bytes.Buffer
	stdcopy.StdCopy(&out, &out, attach.Reader)
	return out.String(), nil
}

// ── layout constants ──────────────────────────────────────────────────────────

const (
	logBatchSize    = 50
	sidebarW        = 26 // content width; +1 right border = 27 total chars
	rightFixedLines = 6  // log title + status + divider + exec title + input + help
)

// ── model ─────────────────────────────────────────────────────────────────────

type logViewModel struct {
	row    types.Row
	ctx    context.Context
	cli    *client.Client
	cancel context.CancelFunc
	logCh  chan string

	lines       []logLine
	oldestTS    string
	logViewport viewport.Model

	loadingMore bool
	noMoreLogs  bool

	cwd          string
	input        textinput.Model
	execLines    []string
	execViewport viewport.Model
	history      []string
	histIdx      int

	focusExec bool
	width     int
	height    int
	ready     bool
}

func newLogView(ctx context.Context, cli *client.Client, row types.Row) (logViewModel, tea.Cmd) {
	streamCtx, cancel := context.WithCancel(ctx)

	ti := textinput.New()
	ti.CharLimit = 256

	lv := logViewModel{
		row:     row,
		ctx:     ctx,
		cli:     cli,
		cancel:  cancel,
		logCh:   make(chan string, 256),
		input:   ti,
		histIdx: -1,
	}

	return lv, tea.Batch(lv.startStream(streamCtx), lv.initCwdCmd())
}

// ── commands ──────────────────────────────────────────────────────────────────

func (lv logViewModel) startStream(ctx context.Context) tea.Cmd {
	return func() tea.Msg {
		reader, err := lv.cli.ContainerLogs(ctx, lv.row.FullContainerID, container.LogsOptions{
			ShowStdout: true,
			ShowStderr: true,
			Follow:     true,
			Tail:       fmt.Sprintf("%d", logBatchSize),
			Timestamps: true,
		})
		if err != nil {
			lv.logCh <- fmt.Sprintf("[logs error: %v]", err)
			close(lv.logCh)
			return lv.readNextLine()()
		}

		pr, pw := io.Pipe()
		go func() {
			stdcopy.StdCopy(pw, pw, reader)
			reader.Close()
			pw.Close()
		}()
		go func() {
			sc := bufio.NewScanner(pr)
			for sc.Scan() {
				lv.logCh <- sc.Text()
			}
			close(lv.logCh)
		}()
		return lv.readNextLine()()
	}
}

func (lv logViewModel) readNextLine() tea.Cmd {
	return func() tea.Msg {
		line, ok := <-lv.logCh
		if !ok {
			return logLineMsg("[stream ended]")
		}
		return logLineMsg(line)
	}
}

func (lv logViewModel) initCwdCmd() tea.Cmd {
	ctx := lv.ctx
	cli := lv.cli
	id := lv.row.FullContainerID
	return func() tea.Msg {
		out, err := sdkExec(ctx, cli, id, "", []string{"pwd"})
		if err != nil {
			// fallback: read WORKDIR from image config
			info, ierr := cli.ContainerInspect(ctx, id)
			if ierr == nil && info.Config.WorkingDir != "" {
				return cwdChangedMsg(info.Config.WorkingDir)
			}
			return cwdChangedMsg("/")
		}
		if cwd := strings.TrimSpace(out); cwd != "" {
			return cwdChangedMsg(cwd)
		}
		return cwdChangedMsg("/")
	}
}

func (lv logViewModel) loadMoreCmd() tea.Cmd {
	ctx := lv.ctx
	cli := lv.cli
	id := lv.row.FullContainerID
	until := subtractOneNano(lv.oldestTS)
	return func() tea.Msg {
		reader, err := cli.ContainerLogs(ctx, id, container.LogsOptions{
			ShowStdout: true,
			ShowStderr: true,
			Timestamps: true,
			Until:      until,
			Tail:       fmt.Sprintf("%d", logBatchSize),
		})
		if err != nil || reader == nil {
			return olderLogsMsg{}
		}
		defer reader.Close()

		var buf bytes.Buffer
		stdcopy.StdCopy(&buf, &buf, reader)

		raw := strings.TrimRight(buf.String(), "\n")
		if raw == "" {
			return olderLogsMsg{}
		}
		var lines []logLine
		for _, line := range strings.Split(raw, "\n") {
			if line == "" {
				continue
			}
			ts, text := splitDockerLogLine(line)
			lines = append(lines, logLine{ts: ts, text: text})
		}
		return olderLogsMsg{lines: lines}
	}
}

func (lv logViewModel) execNormalCmd(command string) tea.Cmd {
	ctx := lv.ctx
	cli := lv.cli
	id := lv.row.FullContainerID
	cwd := lv.cwd
	return func() tea.Msg {
		out, err := sdkExec(ctx, cli, id, cwd, []string{"sh", "-c", command})
		result := strings.TrimRight(out, "\n")
		if err != nil {
			if result != "" {
				result += "\n"
			}
			result += fmt.Sprintf("[exit: %v]", err)
		}
		return execOutputMsg(result)
	}
}

func (lv logViewModel) execCdCmd(path string) tea.Cmd {
	ctx := lv.ctx
	cli := lv.cli
	id := lv.row.FullContainerID
	cwd := lv.cwd
	return func() tea.Msg {
		var sh string
		if path == "" {
			sh = "cd && pwd" // bare "cd" → home
		} else {
			sh = fmt.Sprintf("cd %q && pwd", path)
		}
		out, err := sdkExec(ctx, cli, id, cwd, []string{"sh", "-c", sh})
		newPwd := strings.TrimSpace(out)
		if err != nil || newPwd == "" {
			return execOutputMsg("[cd: no such directory]")
		}
		return cwdChangedMsg(newPwd)
	}
}

// ── layout ────────────────────────────────────────────────────────────────────

func (lv logViewModel) rightW() int {
	w := lv.width - sidebarW - 1
	if w < 20 {
		return 20
	}
	return w
}

func (lv logViewModel) resized(width, height int) logViewModel {
	lv.width = width
	lv.height = height

	rw := lv.rightW()
	available := height - rightFixedLines
	if available < 6 {
		available = 6
	}
	logH := int(float64(available) * 0.70)
	execH := available - logH
	if logH < 3 {
		logH = 3
	}
	if execH < 3 {
		execH = 3
	}

	lv.logViewport.Width = rw
	lv.logViewport.Height = logH
	lv.execViewport.Width = rw
	lv.execViewport.Height = execH
	lv.input.Width = rw - 8
	lv.ready = true
	return lv
}

func (lv logViewModel) buildLogContent() string {
	texts := make([]string, len(lv.lines))
	for i, l := range lv.lines {
		texts[i] = l.text
	}
	return strings.Join(texts, "\n")
}

func (lv *logViewModel) maybeLoadMore() tea.Cmd {
	if lv.logViewport.YOffset == 0 && !lv.loadingMore && !lv.noMoreLogs && lv.oldestTS != "" {
		lv.loadingMore = true
		return lv.loadMoreCmd()
	}
	return nil
}

// ── update ────────────────────────────────────────────────────────────────────

func (lv logViewModel) update(msg tea.Msg) (logViewModel, tea.Cmd, bool) {
	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		return lv.resized(msg.Width, msg.Height), nil, false

	case cwdChangedMsg:
		lv.cwd = string(msg)
		lv.input.Placeholder = lv.cwd
		return lv, nil, false

	case logLineMsg:
		raw := string(msg)
		if raw == "[stream ended]" {
			return lv, nil, false
		}
		atBottom := lv.logViewport.AtBottom()
		ts, text := splitDockerLogLine(raw)
		lv.lines = append(lv.lines, logLine{ts: ts, text: text})
		if lv.oldestTS == "" && ts != "" {
			lv.oldestTS = ts
		}
		lv.logViewport.SetContent(lv.buildLogContent())
		if atBottom {
			lv.logViewport.GotoBottom()
		}
		return lv, lv.readNextLine(), false

	case olderLogsMsg:
		lv.loadingMore = false
		if len(msg.lines) == 0 {
			lv.noMoreLogs = true
			return lv, nil, false
		}
		prevOffset := lv.logViewport.YOffset
		lv.lines = append(msg.lines, lv.lines...)
		lv.oldestTS = msg.lines[0].ts
		lv.logViewport.SetContent(lv.buildLogContent())
		lv.logViewport.SetYOffset(prevOffset + len(msg.lines))
		return lv, nil, false

	case execOutputMsg:
		output := string(msg)
		if output != "" {
			lv.execLines = append(lv.execLines, strings.Split(output, "\n")...)
		}
		lv.execViewport.SetContent(strings.Join(lv.execLines, "\n"))
		lv.execViewport.GotoBottom()
		return lv, nil, false

	case tea.KeyMsg:
		k := msg.String()

		if !lv.focusExec && (k == "q" || k == "esc") {
			lv.cancel()
			return lv, nil, true
		}
		if lv.focusExec && k == "esc" {
			lv.focusExec = false
			lv.input.Blur()
			return lv, nil, false
		}

		if k == "tab" {
			lv.focusExec = !lv.focusExec
			if lv.focusExec {
				lv.input.Focus()
				return lv, textinput.Blink, false
			}
			lv.input.Blur()
			return lv, nil, false
		}
		if k == "right" && !lv.focusExec {
			lv.focusExec = true
			lv.input.Focus()
			return lv, textinput.Blink, false
		}
		if k == "left" && lv.focusExec {
			lv.focusExec = false
			lv.input.Blur()
			return lv, nil, false
		}

		if lv.focusExec && k == "enter" {
			command := strings.TrimSpace(lv.input.Value())
			if command != "" {
				lv.history = append(lv.history, command)
				lv.histIdx = -1
				lv.execLines = append(lv.execLines, fmt.Sprintf("%s $ %s", lv.cwd, command))
				lv.execViewport.SetContent(strings.Join(lv.execLines, "\n"))
				lv.execViewport.GotoBottom()
				lv.input.SetValue("")
				if path, ok := isCdCommand(command); ok {
					return lv, lv.execCdCmd(path), false
				}
				return lv, lv.execNormalCmd(command), false
			}
			return lv, nil, false
		}

		if lv.focusExec && k == "up" {
			if len(lv.history) > 0 {
				if lv.histIdx == -1 {
					lv.histIdx = len(lv.history) - 1
				} else if lv.histIdx > 0 {
					lv.histIdx--
				}
				lv.input.SetValue(lv.history[lv.histIdx])
				lv.input.CursorEnd()
			}
			return lv, nil, false
		}
		if lv.focusExec && k == "down" {
			if lv.histIdx != -1 {
				lv.histIdx++
				if lv.histIdx >= len(lv.history) {
					lv.histIdx = -1
					lv.input.SetValue("")
				} else {
					lv.input.SetValue(lv.history[lv.histIdx])
					lv.input.CursorEnd()
				}
			}
			return lv, nil, false
		}

		if lv.focusExec {
			var c tea.Cmd
			lv.input, c = lv.input.Update(msg)
			return lv, c, false
		}
		var c tea.Cmd
		lv.logViewport, c = lv.logViewport.Update(msg)
		return lv, tea.Batch(c, lv.maybeLoadMore()), false
	}

	var c1, c2 tea.Cmd
	lv.logViewport, c1 = lv.logViewport.Update(msg)
	lv.execViewport, c2 = lv.execViewport.Update(msg)
	return lv, tea.Batch(c1, c2, lv.maybeLoadMore()), false
}

// ── view ──────────────────────────────────────────────────────────────────────

func (lv logViewModel) renderSidebar() string {
	r := lv.row
	var b strings.Builder

	b.WriteString(styles.LgCyan.Render("Service Info") + "\n")
	b.WriteString(styles.LgDim.Render(strings.Repeat("─", sidebarW)) + "\n")

	field := func(label, value string) {
		if value == "" || value == "-" {
			return
		}
		maxV := sidebarW - 9
		if len(value) > maxV {
			value = value[:maxV-1] + "…"
		}
		b.WriteString(fmt.Sprintf(" %-7s %s\n", label, value))
	}

	field("Service", r.Service)
	field("Folder", r.Folder)

	statusStr := r.State
	switch r.Status {
	case types.StatusRunning:
		statusStr = styles.Green("● " + r.State)
	case types.StatusStopped:
		statusStr = styles.Red("○ " + r.State)
	default:
		statusStr = styles.Yellow("◌ " + r.State)
	}
	b.WriteString(fmt.Sprintf(" %-7s %s\n", "Status", statusStr))

	field("Uptime", r.Uptime)
	field("Image", r.Image)
	field("Ports", r.Ports)
	field("ID", r.ContainerID)

	if lv.cwd != "" {
		b.WriteString(styles.LgDim.Render(strings.Repeat("─", sidebarW)) + "\n")
		field("CWD", lv.cwd)
	}

	return b.String()
}

func (lv logViewModel) renderRight() string {
	var b strings.Builder

	b.WriteString(styles.LgCyan.Render("Logs") + "\n")

	switch {
	case lv.loadingMore:
		b.WriteString(styles.LgDim.Render("↑ loading older logs…") + "\n")
	case lv.noMoreLogs:
		b.WriteString(styles.LgDim.Render("↑ beginning of logs") + "\n")
	case lv.oldestTS != "":
		b.WriteString(styles.LgDim.Render("↑ scroll up to load more") + "\n")
	default:
		b.WriteString("\n")
	}

	b.WriteString(lv.logViewport.View() + "\n")
	b.WriteString(styles.LgDim.Render(strings.Repeat("─", lv.rightW())) + "\n")

	cwdLabel := ""
	if lv.cwd != "" {
		cwdLabel = styles.LgDim.Render(fmt.Sprintf(" [%s]", lv.cwd))
	}
	if lv.focusExec {
		b.WriteString(styles.LgBold.Render("Exec") + cwdLabel + styles.LgDim.Render("  [active]") + "\n")
	} else {
		b.WriteString(styles.LgBold.Render("Exec") + cwdLabel + "\n")
	}

	b.WriteString(lv.execViewport.View() + "\n")

	if lv.cwd != "" {
		b.WriteString(styles.LgDim.Render(lv.cwd+" >") + " " + lv.input.View())
	} else {
		b.WriteString("> " + lv.input.View())
	}

	return b.String()
}

func (lv logViewModel) View() string {
	if !lv.ready {
		return "\n  Loading...\n"
	}

	mainH := lv.height - 1

	sideStyle := lipgloss.NewStyle().
		Width(sidebarW).
		Height(mainH).
		PaddingLeft(1).
		BorderStyle(lipgloss.NormalBorder()).
		BorderRight(true).
		BorderForeground(lipgloss.Color("8"))

	combined := lipgloss.JoinHorizontal(lipgloss.Top,
		sideStyle.Render(lv.renderSidebar()),
		lv.renderRight(),
	)

	help := styles.LgHelp.Render(
		"↑↓ scroll  ←/→ tab switch panel  enter exec  ↑↓ cmd history  esc unfocus  q back")

	return combined + "\n" + help
}
