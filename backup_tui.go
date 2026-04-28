package main

import (
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"os"
	"runtime/debug"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	dockercontainer "github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/jedib0t/go-pretty/v6/text"
	"github.com/spf13/cobra"
)

// ── states ───────────────────────────────────────────────────────────────────

type btState int

const (
	btList      btState = iota // container list
	btEnterUser                // ask pg user (+ schedule for cron) before fetching DBs
	btLoadingDB                // fetching pg databases
	btPickDB                   // select database from list
	btBusy                     // backup / cron running
)

// focus inside btEnterUser
const (
	fuUser  = iota
	fuSched // cron only
)

// ── types ────────────────────────────────────────────────────────────────────

type btContainer struct {
	id     string
	fullID string
	name   string
	image  string
	state  string
}

type btLogEntry struct {
	at   time.Time
	text string
	ok   bool
}

// ── messages ─────────────────────────────────────────────────────────────────

type btContainersMsg []btContainer
type btDBsMsg struct {
	dbs []string
	err error
}
type btBackupDoneMsg struct {
	path string
	err  error
}
type btCronDoneMsg struct {
	schedule string
	err      error
}

// ── model ────────────────────────────────────────────────────────────────────

type btModel struct {
	ctx context.Context
	cli *client.Client

	state  btState
	action string // "backup" | "cron"

	// container list
	containers  []btContainer
	cursor      int
	filter      string
	searching   bool
	searchInput textinput.Model

	// btEnterUser fields
	userInput  textinput.Model
	schedInput textinput.Model
	fuFocus    int // fuUser | fuSched

	// btPickDB fields
	databases []string
	dbCursor  int

	// resolved values (set after btEnterUser confirmed)
	pgUser   string
	schedule string

	// log
	logs []btLogEntry

	// layout
	status string
	width  int
	height int
	vp     viewport.Model
	ready  bool
}

// ── constructor ───────────────────────────────────────────────────────────────

func newBackupTUI(ctx context.Context, cli *client.Client) btModel {
	si := textinput.New()
	si.Placeholder = "filter containers..."
	si.CharLimit = 64

	ui := textinput.New()
	ui.Placeholder = "pg user (e.g. postgres, testuser)"
	ui.CharLimit = 32

	sc := textinput.New()
	sc.Placeholder = "cron expression (e.g. 0 2 * * *)"
	sc.CharLimit = 32

	return btModel{
		ctx:        ctx,
		cli:        cli,
		searchInput: si,
		userInput:  ui,
		schedInput: sc,
	}
}

// ── init ─────────────────────────────────────────────────────────────────────

func (m btModel) Init() tea.Cmd {
	return m.fetchContainers()
}

// ── commands ──────────────────────────────────────────────────────────────────

func (m btModel) fetchContainers() tea.Cmd {
	return func() tea.Msg {
		list, err := m.cli.ContainerList(m.ctx, dockercontainer.ListOptions{All: false})
		if err != nil {
			return btContainersMsg{}
		}
		var rows []btContainer
		for _, c := range list {
			name := c.ID[:12]
			if len(c.Names) > 0 {
				name = strings.TrimPrefix(c.Names[0], "/")
			}
			img := c.Image
			if strings.HasPrefix(img, "sha256:") {
				img = img[:19]
			}
			id := c.ID
			if len(id) > 12 {
				id = id[:12]
			}
			rows = append(rows, btContainer{
				id: id, fullID: c.ID,
				name: name, image: img, state: c.State,
			})
		}
		return btContainersMsg(rows)
	}
}

func (m btModel) fetchDatabases(containerID, pgUser string) tea.Cmd {
	return func() tea.Msg {
		execResp, err := m.cli.ContainerExecCreate(m.ctx, containerID, dockercontainer.ExecOptions{
			Cmd: []string{
				"psql", "-U", pgUser, "-d", "postgres", "-Atc",
				"SELECT datname FROM pg_database WHERE datistemplate = false ORDER BY datname",
			},
			AttachStdout: true,
			AttachStderr: true,
		})
		if err != nil {
			return btDBsMsg{err: err}
		}
		attach, err := m.cli.ContainerExecAttach(m.ctx, execResp.ID, dockercontainer.ExecAttachOptions{})
		if err != nil {
			return btDBsMsg{err: err}
		}
		defer attach.Close()

		var stdout, stderr bytes.Buffer
		stdcopy.StdCopy(&stdout, &stderr, attach.Reader) //nolint

		insp, _ := m.cli.ContainerExecInspect(m.ctx, execResp.ID)
		if insp.ExitCode != 0 {
			msg := strings.TrimSpace(stderr.String())
			if msg == "" {
				msg = fmt.Sprintf("psql exited %d", insp.ExitCode)
			}
			return btDBsMsg{err: fmt.Errorf("%s", msg)}
		}

		var dbs []string
		for _, line := range strings.Split(stdout.String(), "\n") {
			if db := strings.TrimSpace(line); db != "" {
				dbs = append(dbs, db)
			}
		}
		return btDBsMsg{dbs: dbs}
	}
}

func (m btModel) doBackup(containerID, pgUser, dbName string) tea.Cmd {
	return func() tea.Msg {
		settings, err := loadBackupSettings()
		if err != nil {
			return btBackupDoneMsg{err: fmt.Errorf("no R2 config — run 'sir autobackup config set'")}
		}
		sqlData, err := pgDumpContainer(m.ctx, m.cli, containerID, pgUser, dbName)
		if err != nil {
			return btBackupDoneMsg{err: err}
		}
		var buf bytes.Buffer
		gz := gzip.NewWriter(&buf)
		_, _ = gz.Write(sqlData)
		_ = gz.Close()
		compressed := buf.Bytes()

		ts := time.Now().UTC().Format("2006-01-02T15-04-05Z")
		key := fmt.Sprintf("backups/%s/%s-%s.sql.gz", dbName, dbName, ts)

		r2 := newR2Client(settings.R2)
		_, err = r2.PutObject(m.ctx, &s3.PutObjectInput{
			Bucket:        aws.String(settings.R2.BucketName),
			Key:           aws.String(key),
			Body:          bytes.NewReader(compressed),
			ContentType:   aws.String("application/gzip"),
			ContentLength: aws.Int64(int64(len(compressed))),
		})
		if err != nil {
			return btBackupDoneMsg{err: fmt.Errorf("upload: %w", err)}
		}
		return btBackupDoneMsg{path: fmt.Sprintf("%s/%s", settings.R2.BucketName, key)}
	}
}

func (m btModel) doCron(containerID, pgUser, dbName, schedule string) tea.Cmd {
	return func() tea.Msg {
		err := setCronJob(schedule, containerID, pgUser, dbName)
		if err != nil {
			return btCronDoneMsg{err: err}
		}
		return btCronDoneMsg{schedule: schedule}
	}
}

// ── update ────────────────────────────────────────────────────────────────────

func (m btModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		vpH := m.height - m.fixedLines()
		if vpH < 3 {
			vpH = 3
		}
		if !m.ready {
			m.vp = viewport.New(m.width, vpH)
			m.ready = true
		} else {
			m.vp.Width = m.width
			m.vp.Height = vpH
		}
		m.updateVP()

	case btContainersMsg:
		m.containers = []btContainer(msg)
		if m.cursor >= len(m.containers) && len(m.containers) > 0 {
			m.cursor = len(m.containers) - 1
		}
		m.updateVP()

	case btDBsMsg:
		if msg.err != nil {
			m.state = btEnterUser
			m.status = red("  ✗ %s", msg.err.Error())
			m.updateVP()
			return m, nil
		}
		m.databases = msg.dbs
		m.dbCursor = 0
		m.state = btPickDB
		m.status = ""
		m.updateVP()

	case btBackupDoneMsg:
		m.state = btList
		if msg.err != nil {
			m.addLog(false, msg.err.Error())
		} else {
			m.addLog(true, msg.path)
		}
		m.status = ""
		m.updateVP()

	case btCronDoneMsg:
		m.state = btList
		if msg.err != nil {
			m.addLog(false, "cron: "+msg.err.Error())
		} else {
			m.addLog(true, "cron set: "+msg.schedule)
		}
		m.status = ""
		m.updateVP()

	case clearStatusMsg:
		m.status = ""
		m.updateVP()

	case tea.KeyMsg:
		switch m.state {
		case btList:
			return m.updateList(msg)
		case btEnterUser:
			return m.updateEnterUser(msg)
		case btPickDB:
			return m.updatePickDB(msg)
		case btBusy, btLoadingDB:
			// block input
		}
	}

	var cmd tea.Cmd
	m.vp, cmd = m.vp.Update(msg)
	return m, cmd
}

// ── list keys ─────────────────────────────────────────────────────────────────

func (m btModel) updateList(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	filtered := m.filtered()

	if m.searching {
		switch msg.String() {
		case "esc":
			m.searching = false
			m.searchInput.SetValue("")
			m.filter = ""
			m.searchInput.Blur()
			m.cursor = 0
			m.updateVP()
			return m, nil
		case "enter":
			m.searching = false
			m.searchInput.Blur()
			m.updateVP()
			return m, nil
		default:
			var cmd tea.Cmd
			m.searchInput, cmd = m.searchInput.Update(msg)
			m.filter = m.searchInput.Value()
			m.cursor = 0
			m.updateVP()
			return m, cmd
		}
	}

	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "up":
		if m.cursor > 0 {
			m.cursor--
			m.updateVP()
		}
	case "down":
		if m.cursor < len(filtered)-1 {
			m.cursor++
			m.updateVP()
		}
	case "b", "c":
		if len(filtered) == 0 {
			break
		}
		m.action = map[string]string{"b": "backup", "c": "cron"}[msg.String()]
		m.userInput.SetValue("")
		m.userInput.Focus()
		m.schedInput.SetValue("")
		m.schedInput.Blur()
		m.fuFocus = fuUser
		m.status = ""
		m.state = btEnterUser
		m.updateVP()
		return m, textinput.Blink
	case "r":
		return m, m.fetchContainers()
	case "/":
		m.searching = true
		m.searchInput.Focus()
		return m, textinput.Blink
	}
	return m, nil
}

// ── enter-user keys ───────────────────────────────────────────────────────────

func (m btModel) updateEnterUser(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.state = btList
		m.status = ""
		m.updateVP()
		return m, nil

	case "tab", "shift+tab":
		if m.action == "cron" {
			if m.fuFocus == fuUser {
				m.fuFocus = fuSched
				m.userInput.Blur()
				m.schedInput.Focus()
			} else {
				m.fuFocus = fuUser
				m.schedInput.Blur()
				m.userInput.Focus()
			}
		}
		return m, textinput.Blink

	case "enter":
		pgUser := strings.TrimSpace(m.userInput.Value())
		if pgUser == "" {
			pgUser = "postgres"
			m.userInput.SetValue(pgUser)
		}
		if m.action == "cron" && strings.TrimSpace(m.schedInput.Value()) == "" {
			m.fuFocus = fuSched
			m.userInput.Blur()
			m.schedInput.Focus()
			return m, textinput.Blink
		}
		// store resolved values
		m.pgUser = pgUser
		m.schedule = strings.TrimSpace(m.schedInput.Value())

		filtered := m.filtered()
		if m.cursor >= len(filtered) {
			m.state = btList
			return m, nil
		}
		ct := filtered[m.cursor]
		m.state = btLoadingDB
		m.status = dim("  Fetching databases from %s...", ct.name)
		m.updateVP()
		return m, m.fetchDatabases(ct.fullID, m.pgUser)
	}

	var cmd tea.Cmd
	switch m.fuFocus {
	case fuUser:
		m.userInput, cmd = m.userInput.Update(msg)
	case fuSched:
		m.schedInput, cmd = m.schedInput.Update(msg)
	}
	m.updateVP()
	return m, cmd
}

// ── pick-db keys ──────────────────────────────────────────────────────────────

func (m btModel) updatePickDB(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.state = btEnterUser
		m.userInput.Focus()
		m.fuFocus = fuUser
		m.status = ""
		m.updateVP()
		return m, nil
	case "up":
		if m.dbCursor > 0 {
			m.dbCursor--
			m.updateVP()
		}
	case "down":
		if m.dbCursor < len(m.databases)-1 {
			m.dbCursor++
			m.updateVP()
		}
	case "enter":
		if len(m.databases) == 0 {
			return m, nil
		}
		dbName := m.databases[m.dbCursor]
		filtered := m.filtered()
		if m.cursor >= len(filtered) {
			m.state = btList
			return m, nil
		}
		ct := filtered[m.cursor]

		if m.action == "backup" {
			m.state = btBusy
			m.status = dim("  Dumping '%s' from %s...", dbName, ct.name)
			m.updateVP()
			return m, m.doBackup(ct.fullID, m.pgUser, dbName)
		}
		m.state = btBusy
		m.status = dim("  Setting cron (%s) for %s/%s...", m.schedule, ct.name, dbName)
		m.updateVP()
		return m, m.doCron(ct.fullID, m.pgUser, dbName, m.schedule)
	}
	return m, nil
}

// ── view ──────────────────────────────────────────────────────────────────────

func (m btModel) View() string {
	if !m.ready {
		return "  Loading..."
	}
	var b strings.Builder

	b.WriteString("\n")
	b.WriteString(lgCyan.Render("  🗄  SIR - Autobackup Manager"))
	cfg, _ := loadBackupSettings()
	if cfg.R2.BucketName != "" {
		b.WriteString(lgDim.Render("  ·  R2: " + cfg.R2.BucketName))
	}
	b.WriteString("\n\n")

	switch m.state {
	case btEnterUser:
		b.WriteString(m.viewEnterUser())
	case btPickDB:
		b.WriteString(m.viewPickDB())
	default:
		b.WriteString(m.vp.View())
		b.WriteString("\n")
		b.WriteString(m.viewLogs())
		if m.status != "" {
			b.WriteString(m.status + "\n")
		}
		if m.searching {
			b.WriteString("  🔍 " + m.searchInput.View() + "\n")
		} else if m.filter != "" {
			b.WriteString("  🔍 " + lgDim.Render(m.filter) + "\n")
		}
		b.WriteString(m.viewHelp())
	}
	return b.String()
}

func (m btModel) formWidth() int {
	w := m.width - 6
	if w < 52 {
		w = 52
	}
	return w
}

func (m btModel) viewEnterUser() string {
	filtered := m.filtered()
	ctName := "(unknown)"
	if m.cursor < len(filtered) {
		ctName = filtered[m.cursor].name
	}
	title := map[string]string{"backup": "Backup", "cron": "Schedule Cron"}[m.action]

	w := m.formWidth()
	hr := strings.Repeat("─", w)
	pad := func(s string, n int) string {
		l := visLen(s)
		if n > l {
			return s + strings.Repeat(" ", n-l)
		}
		return s
	}

	var b strings.Builder
	b.WriteString(fmt.Sprintf("  ╭%s╮\n", hr))
	b.WriteString(fmt.Sprintf("  │%s│\n", pad(fmt.Sprintf(" %s %s ", lgBold.Render(title+":"), lgCyan.Render(ctName)), w)))
	b.WriteString(fmt.Sprintf("  ├%s┤\n", hr))

	// PG User row
	uFocused := m.fuFocus == fuUser
	uMark := dim("  ")
	if uFocused {
		uMark = cyan("> ")
	}
	b.WriteString(fmt.Sprintf("  │%s│\n", pad(fmt.Sprintf(" %s%s  %s", uMark, lgBold.Render("PG User "), m.userInput.View()), w)))

	// Schedule row (cron only)
	if m.action == "cron" {
		sFocused := m.fuFocus == fuSched
		sMark := dim("  ")
		if sFocused {
			sMark = cyan("> ")
		}
		b.WriteString(fmt.Sprintf("  │%s│\n", pad(fmt.Sprintf(" %s%s  %s", sMark, lgBold.Render("Schedule"), m.schedInput.View()), w)))
	}

	if m.status != "" {
		b.WriteString(fmt.Sprintf("  │%s│\n", pad(" "+m.status, w)))
	}

	b.WriteString(fmt.Sprintf("  ├%s┤\n", hr))
	help := ""
	if m.action == "cron" {
		help = lgDim.Render("  tab next  enter confirm  esc back")
	} else {
		help = lgDim.Render("  enter confirm  esc back")
	}
	b.WriteString(fmt.Sprintf("  │%s│\n", pad(" "+help, w)))
	b.WriteString(fmt.Sprintf("  ╰%s╯\n", hr))
	return b.String()
}

func (m btModel) viewPickDB() string {
	filtered := m.filtered()
	ctName := "(unknown)"
	if m.cursor < len(filtered) {
		ctName = filtered[m.cursor].name
	}
	title := map[string]string{"backup": "Backup", "cron": "Schedule Cron"}[m.action]

	w := m.formWidth()
	hr := strings.Repeat("─", w)
	pad := func(s string, n int) string {
		l := visLen(s)
		if n > l {
			return s + strings.Repeat(" ", n-l)
		}
		return s
	}

	var b strings.Builder
	b.WriteString(fmt.Sprintf("  ╭%s╮\n", hr))
	b.WriteString(fmt.Sprintf("  │%s│\n", pad(fmt.Sprintf(" %s %s  %s %s",
		lgBold.Render(title+":"), lgCyan.Render(ctName),
		lgDim.Render("user:"), lgDim.Render(m.pgUser),
	), w)))
	if m.action == "cron" {
		b.WriteString(fmt.Sprintf("  │%s│\n", pad(fmt.Sprintf(" %s %s",
			lgDim.Render("schedule:"), lgDim.Render(m.schedule),
		), w)))
	}
	b.WriteString(fmt.Sprintf("  ├%s┤\n", hr))
	b.WriteString(fmt.Sprintf("  │%s│\n", pad(" "+lgBold.Render("Select Database"), w)))

	maxRows := 10
	start := 0
	if m.dbCursor >= maxRows {
		start = m.dbCursor - maxRows + 1
	}
	end := start + maxRows
	if end > len(m.databases) {
		end = len(m.databases)
	}

	if len(m.databases) == 0 {
		b.WriteString(fmt.Sprintf("  │%s│\n", pad("  "+lgDim.Render("no databases found"), w)))
	}
	for i := start; i < end; i++ {
		db := m.databases[i]
		if i == m.dbCursor {
			b.WriteString(fmt.Sprintf("  │%s│\n", pad(fmt.Sprintf("  %s %s", cyan(">"), lgBold.Render(db)), w)))
		} else {
			b.WriteString(fmt.Sprintf("  │%s│\n", pad(fmt.Sprintf("    %s", db), w)))
		}
	}

	b.WriteString(fmt.Sprintf("  ├%s┤\n", hr))
	b.WriteString(fmt.Sprintf("  │%s│\n", pad(" "+lgDim.Render("↑↓ select  enter confirm  esc back"), w)))
	b.WriteString(fmt.Sprintf("  ╰%s╯\n", hr))
	return b.String()
}

func (m btModel) viewLogs() string {
	if len(m.logs) == 0 {
		return lgDim.Render("  No recent backups") + "\n"
	}
	var b strings.Builder
	b.WriteString(lgBold.Render("  Recent") + "\n")
	for _, e := range m.logs {
		ts := lgDim.Render(e.at.Format("15:04:05"))
		if e.ok {
			b.WriteString(fmt.Sprintf("  %s %s  %s\n", green("✓"), ts, lgDim.Render(e.text)))
		} else {
			b.WriteString(fmt.Sprintf("  %s %s  %s\n", red("✗"), ts, red("%s", e.text)))
		}
	}
	return b.String()
}

func (m btModel) viewHelp() string {
	if m.state == btBusy || m.state == btLoadingDB {
		return lgHelp.Render("  please wait...")
	}
	return lgHelp.Render("  ↑↓ move  b backup  c cron  r refresh  / search  q quit")
}

// ── helpers ───────────────────────────────────────────────────────────────────

func (m *btModel) addLog(ok bool, text string) {
	m.logs = append([]btLogEntry{{at: time.Now(), text: text, ok: ok}}, m.logs...)
	if len(m.logs) > 6 {
		m.logs = m.logs[:6]
	}
}

func (m btModel) filtered() []btContainer {
	if m.filter == "" {
		return m.containers
	}
	q := strings.ToLower(m.filter)
	var out []btContainer
	for _, c := range m.containers {
		if strings.Contains(strings.ToLower(c.name), q) ||
			strings.Contains(strings.ToLower(c.image), q) ||
			strings.Contains(strings.ToLower(c.id), q) {
			out = append(out, c)
		}
	}
	return out
}

func (m btModel) fixedLines() int {
	n := 4 + 1 + len(m.logs) + 2
	if m.status != "" {
		n++
	}
	if m.searching || m.filter != "" {
		n++
	}
	return n
}

func (m *btModel) updateVP() {
	if !m.ready {
		return
	}
	vpH := m.height - m.fixedLines()
	if vpH < 3 {
		vpH = 3
	}
	m.vp.Height = vpH
	m.vp.SetContent(m.renderContainerTable())

	filtered := m.filtered()
	if len(filtered) == 0 {
		return
	}
	cursorLine := m.cursor + 3
	if cursorLine < m.vp.YOffset {
		m.vp.SetYOffset(cursorLine)
	} else if cursorLine >= m.vp.YOffset+m.vp.Height {
		m.vp.SetYOffset(cursorLine - m.vp.Height + 1)
	}
}

func (m btModel) renderContainerTable() string {
	filtered := m.filtered()
	if len(filtered) == 0 {
		return lgDim.Render("  No running containers")
	}
	t := table.NewWriter()
	t.AppendHeader(table.Row{" ", "Container", "Image", "Status"})
	for i, c := range filtered {
		marker := " "
		if i == m.cursor {
			marker = cyan(">")
		}
		stateStr := green("● running")
		if c.state != "running" {
			stateStr = yellow("○ %s", c.state)
		}
		t.AppendRow(table.Row{marker, cyan(c.name), dim(c.image), stateStr})
	}
	style := table.StyleLight
	style.Color.Header = text.Colors{text.FgCyan, text.Bold}
	style.Color.Border = text.Colors{text.FgHiBlack}
	style.Color.Separator = text.Colors{text.FgHiBlack}
	t.SetStyle(style)
	t.Style().Options.SeparateRows = false
	return t.Render()
}

func visLen(s string) int {
	inEsc := false
	n := 0
	for _, r := range s {
		if r == '\x1b' {
			inEsc = true
		} else if inEsc && r == 'm' {
			inEsc = false
		} else if !inEsc {
			n++
		}
	}
	return n
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// ── cobra command ─────────────────────────────────────────────────────────────

func newAutobackupTUICmd() *cobra.Command {
	return &cobra.Command{
		Use:   "tui",
		Short: "Interactive TUI for managing autobackups",
		Args:  cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			logFile, _ := os.OpenFile("/tmp/sir-tui.log", os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
			defer func() {
				if r := recover(); r != nil {
					fmt.Fprintf(logFile, "panic: %v\n\n%s\n", r, debug.Stack())
					logFile.Close()
					panic(r)
				}
				logFile.Close()
			}()

			ctx := context.Background()
			cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
			if err != nil {
				cRed.Printf("  Error: cannot connect to Docker: %v\n", err)
				os.Exit(1)
			}
			defer cli.Close()

			m := newBackupTUI(ctx, cli)
			p := tea.NewProgram(m, tea.WithAltScreen())
			if _, err := p.Run(); err != nil {
				fmt.Fprintf(logFile, "run error: %v\n", err)
				cRed.Printf("  TUI error: %v\n", err)
				os.Exit(1)
			}
		},
	}
}
