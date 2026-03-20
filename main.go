package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/augustose/infuser-go/internal/config"
	"github.com/augustose/infuser-go/internal/setup"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type cmdFinishedMsg struct{ err error }
type addServerFinishedMsg struct{ err error }
type removeServerFinishedMsg struct{ err error }
type blinkMsg struct{}

// -- banner --

const banner = `  ██╗███╗   ██╗███████╗██╗   ██╗███████╗███████╗██████╗
  ██║████╗  ██║██╔════╝██║   ██║██╔════╝██╔════╝██╔══██╗
  ██║██╔██╗ ██║█████╗  ██║   ██║███████╗█████╗  ██████╔╝
  ██║██║╚██╗██║██╔══╝  ██║   ██║╚════██║██╔══╝  ██╔══██╗
  ██║██║ ╚████║██║     ╚██████╔╝███████║███████╗██║  ██║
  ╚═╝╚═╝  ╚═══╝╚═╝      ╚═════╝ ╚══════╝╚══════╝╚═╝  ╚═╝`

// -- styles --

var (
	bannerStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("39")).
			Bold(true)

	subtitleStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("243")).
			PaddingLeft(2)

	serverStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("243"))

	itemStyle         = lipgloss.NewStyle().PaddingLeft(2)
	selectedItemStyle = lipgloss.NewStyle().
				PaddingLeft(2).
				Foreground(lipgloss.Color("39")).
				Bold(true)
	descStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("243"))
	selectedDescStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("111"))

	disabledStyle = lipgloss.NewStyle().
			PaddingLeft(2).
			Foreground(lipgloss.Color("238"))
	disabledDescStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("238"))

	statusBarStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("252")).
			Background(lipgloss.Color("236")).
			PaddingLeft(1).
			PaddingRight(1)

	statusKeyStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("39")).
			Background(lipgloss.Color("236")).
			Bold(true)

	statusOkStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("42")).
			Background(lipgloss.Color("236"))

	statusNoStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("196")).
			Background(lipgloss.Color("236"))

	hintStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("220")).
			Background(lipgloss.Color("236")).
			Bold(true)

	hintDimStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("58")).
			Background(lipgloss.Color("236"))
)

// -- menu items --

type action struct {
	name        string
	desc        string
	needsState  bool
	needsConfig bool
}

var actions = []action{
	{"Reconcile (dry-run)", "Shows what changes would be made without touching Gitea", true, false},
	{"Reconcile (apply)", "Applies pending changes after interactive confirmation", false, false},
	{"Reconcile (apply + auto-approve)", "Applies changes without confirmation (CI/CD)", false, false},
	{"Export Gitea state", "Downloads users, orgs, repos into YAML files", false, false},
	{"Reset local memory", "Deletes state file and rebuilds from current YAMLs", true, false},
	{"Repository grid report", "Generates CSV+MD with repos, owners, and access info", false, true},
}

// -- view enum --

type view int

const (
	viewServerSelect view = iota
	viewActionSelect
)

// -- model --

type model struct {
	servers      []config.ServerConfig
	serverIdx    int
	actionIdx    int
	currentView  view
	width        int
	height       int
	err          error
	quitting     bool
	returningCmd bool
	hasState     bool
	hasConfig    bool
	blinkOn      bool
}

func stateFileExists(srv config.ServerConfig) bool {
	_, err := os.Stat(srv.StateFile)
	return err == nil
}

func configDirExists(srv config.ServerConfig) bool {
	entries, err := os.ReadDir(srv.ConfigDir)
	return err == nil && len(entries) > 0
}

func (m model) actionDisabled(a action) bool {
	return (a.needsState && !m.hasState) || (a.needsConfig && !m.hasConfig)
}

func (m model) firstEnabledAction() int {
	for i, a := range actions {
		if !m.actionDisabled(a) {
			return i
		}
	}
	return 0
}

func initialModel() model {
	servers, err := config.LoadServers()
	if err != nil {
		// No servers configured — show server select with just "add server"
		return model{
			servers:     nil,
			currentView: viewServerSelect,
			serverIdx:   0, // points to the "add server" item
		}
	}

	return model{
		servers:     servers,
		currentView: viewServerSelect,
	}
}

func blinkTick() tea.Cmd {
	return tea.Tick(600*time.Millisecond, func(t time.Time) tea.Msg {
		return blinkMsg{}
	})
}

func (m model) Init() tea.Cmd {
	return blinkTick()
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case blinkMsg:
		m.blinkOn = !m.blinkOn
		return m, blinkTick()

	case cmdFinishedMsg:
		m.returningCmd = true
		if m.serverIdx < len(m.servers) {
			m.hasState = stateFileExists(m.servers[m.serverIdx])
			m.hasConfig = configDirExists(m.servers[m.serverIdx])
		}
		return m, nil

	case addServerFinishedMsg:
		m.returningCmd = true
		if servers, err := config.LoadServers(); err == nil {
			m.servers = servers
		}
		m.currentView = viewServerSelect
		if m.serverIdx > len(m.servers) {
			m.serverIdx = len(m.servers)
		}
		return m, nil

	case removeServerFinishedMsg:
		m.returningCmd = true
		if servers, err := config.LoadServers(); err == nil {
			m.servers = servers
		}
		m.currentView = viewServerSelect
		if m.serverIdx >= len(m.servers) {
			m.serverIdx = max(len(m.servers)-1, 0)
		}
		return m, nil

	case tea.KeyMsg:
		if m.returningCmd {
			m.returningCmd = false
		}

		if m.err != nil {
			m.quitting = true
			return m, tea.Quit
		}

		switch msg.String() {
		case "ctrl+c":
			m.quitting = true
			return m, tea.Quit

		case "up", "k":
			switch m.currentView {
			case viewServerSelect:
				if m.serverIdx > 0 {
					m.serverIdx--
				}
			case viewActionSelect:
				for i := m.actionIdx - 1; i >= 0; i-- {
					if !m.actionDisabled(actions[i]) {
						m.actionIdx = i
						break
					}
				}
			}

		case "down", "j":
			switch m.currentView {
			case viewServerSelect:
				// len(m.servers) is the "add server" item
				if m.serverIdx < len(m.servers) {
					m.serverIdx++
				}
			case viewActionSelect:
				for i := m.actionIdx + 1; i < len(actions); i++ {
					if !m.actionDisabled(actions[i]) {
						m.actionIdx = i
						break
					}
				}
			}

		case "enter":
			switch m.currentView {
			case viewServerSelect:
				if m.serverIdx == len(m.servers) {
					// "Add new server" selected
					return m, execAddServer()
				}
				m.currentView = viewActionSelect
				m.actionIdx = m.firstEnabledAction()
				m.hasState = stateFileExists(m.servers[m.serverIdx])
				m.hasConfig = configDirExists(m.servers[m.serverIdx])
				return m, nil
			case viewActionSelect:
				if m.actionDisabled(actions[m.actionIdx]) {
					return m, nil
				}
				return m, m.runAction()
			}

		case "d", "delete", "backspace":
			if m.currentView == viewServerSelect && m.serverIdx < len(m.servers) {
				return m, execRemoveServer(m.servers[m.serverIdx])
			}

		case "esc":
			switch m.currentView {
			case viewActionSelect:
				m.currentView = viewServerSelect
				return m, nil
			case viewServerSelect:
				m.quitting = true
				return m, tea.Quit
			}
		}
	}

	return m, nil
}

func execWithPause(args ...string) tea.Cmd {
	script := strings.Join(args, " ")
	script += `; echo ""; read -p "Press Enter to return to menu..." _`
	cmd := exec.Command("sh", "-c", script)
	return tea.ExecProcess(cmd, func(err error) tea.Msg {
		return cmdFinishedMsg{err: err}
	})
}

// -- views --

func (m model) View() string {
	if m.err != nil {
		return fmt.Sprintf("\n  Error: %v\n\n  Press any key to exit.\n\n", m.err)
	}
	if m.quitting {
		return ""
	}

	header := bannerStyle.Render(banner) + "\n" + subtitleStyle.Render("Infrastructure as Code for Gitea")

	var content string
	switch m.currentView {
	case viewServerSelect:
		content = m.serverSelectView()
	case viewActionSelect:
		content = m.actionSelectView()
	}

	statusBar := m.renderStatusBar()

	// Calculate padding to push status bar to bottom
	contentLines := strings.Count(header, "\n") + strings.Count(content, "\n") + 2
	statusBarLines := 1
	if m.nextStepHint() != "" {
		statusBarLines = 2
	}
	padding := 0
	if m.height > 0 {
		padding = m.height - contentLines - statusBarLines
	}
	if padding < 1 {
		padding = 1
	}

	return header + "\n" + content + strings.Repeat("\n", padding) + statusBar
}

func (m model) serverSelectView() string {
	var b strings.Builder
	b.WriteString("\n  Select a server:\n\n")

	for i, srv := range m.servers {
		name := srv.Name
		url := serverStyle.Render(fmt.Sprintf("(%s)", srv.URL))

		if i == m.serverIdx {
			b.WriteString(selectedItemStyle.Render("▸ "+name) + " " + url + "\n")
		} else {
			b.WriteString(itemStyle.Render("  "+name) + " " + url + "\n")
		}
	}

	// Separator and "Add new server" option
	b.WriteString(descStyle.Render("    ── ── ── ──") + "\n")
	addIdx := len(m.servers)
	if m.serverIdx == addIdx {
		b.WriteString(selectedItemStyle.Render("▸ + Add new server") + "\n")
	} else {
		b.WriteString(itemStyle.Render("  + Add new server") + "\n")
	}

	return b.String()
}

// addServerExec implements tea.ExecCommand to run the add-server wizard in-process.
type addServerExec struct {
	stdin  io.Reader
	stdout io.Writer
	stderr io.Writer
}

func (a *addServerExec) Run() error {
	_, err := setup.RunAddServer()
	if err != nil {
		fmt.Fprintf(a.stderr, "Error: %v\n", err)
	}
	fmt.Fprintln(a.stdout)
	fmt.Fprint(a.stdout, "Press Enter to return to menu...")
	reader := bufio.NewReader(a.stdin)
	_, _ = reader.ReadString('\n')
	return err
}

func (a *addServerExec) SetStdin(r io.Reader)  { a.stdin = r }
func (a *addServerExec) SetStdout(w io.Writer) { a.stdout = w }
func (a *addServerExec) SetStderr(w io.Writer) { a.stderr = w }

func execAddServer() tea.Cmd {
	return tea.Exec(&addServerExec{}, func(err error) tea.Msg {
		return addServerFinishedMsg{err: err}
	})
}

// removeServerExec implements tea.ExecCommand to confirm and remove a server.
type removeServerExec struct {
	name      string
	configDir string
	stateFile string
	stdin     io.Reader
	stdout    io.Writer
	stderr    io.Writer
}

func (r *removeServerExec) Run() error {
	fmt.Fprintf(r.stdout, "\n=== Remove Server ===\n\n")
	fmt.Fprintf(r.stdout, "  Server:     %s\n", r.name)
	fmt.Fprintf(r.stdout, "  Config dir: %s\n", r.configDir)
	fmt.Fprintf(r.stdout, "  State file: %s\n\n", r.stateFile)
	fmt.Fprintf(r.stdout, "  This will remove the server entry, config directory, and state file.\n\n")
	fmt.Fprint(r.stdout, "  Are you sure? [y/N]: ")

	reader := bufio.NewReader(r.stdin)
	input, _ := reader.ReadString('\n')
	input = strings.TrimSpace(strings.ToLower(input))

	if input != "y" && input != "yes" {
		fmt.Fprintln(r.stdout, "\n  Cancelled.")
		fmt.Fprintln(r.stdout)
		fmt.Fprint(r.stdout, "Press Enter to return to menu...")
		_, _ = reader.ReadString('\n')
		return nil
	}

	if err := config.RemoveServerFromYAML(r.name); err != nil {
		fmt.Fprintf(r.stderr, "\n  Error: %v\n", err)
		fmt.Fprintln(r.stdout)
		fmt.Fprint(r.stdout, "Press Enter to return to menu...")
		_, _ = reader.ReadString('\n')
		return err
	}
	fmt.Fprintf(r.stdout, "\n  Removed \"%s\" from servers.yaml\n", r.name)

	if err := os.RemoveAll(r.configDir); err != nil {
		fmt.Fprintf(r.stdout, "  Warning: could not delete config dir: %v\n", err)
	} else {
		fmt.Fprintf(r.stdout, "  Deleted config dir: %s\n", r.configDir)
	}

	if err := os.Remove(r.stateFile); err != nil && !os.IsNotExist(err) {
		fmt.Fprintf(r.stdout, "  Warning: could not delete state file: %v\n", err)
	} else if err == nil {
		fmt.Fprintf(r.stdout, "  Deleted state file: %s\n", r.stateFile)
	}

	fmt.Fprintln(r.stdout)
	fmt.Fprint(r.stdout, "Press Enter to return to menu...")
	_, _ = reader.ReadString('\n')
	return nil
}

func (r *removeServerExec) SetStdin(rd io.Reader) { r.stdin = rd }
func (r *removeServerExec) SetStdout(w io.Writer) { r.stdout = w }
func (r *removeServerExec) SetStderr(w io.Writer) { r.stderr = w }

func execRemoveServer(srv config.ServerConfig) tea.Cmd {
	return tea.Exec(&removeServerExec{
		name:      srv.Name,
		configDir: srv.ConfigDir,
		stateFile: srv.StateFile,
	}, func(err error) tea.Msg {
		return removeServerFinishedMsg{err: err}
	})
}

func (m model) actionSelectView() string {
	srv := m.servers[m.serverIdx]

	var b strings.Builder
	b.WriteString("\n" + serverStyle.Render(fmt.Sprintf("  Server: %s (%s)", srv.Name, srv.URL)) + "\n\n")

	for i, a := range actions {
		if m.actionDisabled(a) {
			reason := "no state file"
			if a.needsConfig && !m.hasConfig {
				reason = "export first"
			}
			b.WriteString(disabledStyle.Render("  "+a.name) + "\n")
			b.WriteString("    " + disabledDescStyle.Render(a.desc+" ("+reason+")") + "\n")
		} else if i == m.actionIdx {
			b.WriteString(selectedItemStyle.Render("▸ "+a.name) + "\n")
			b.WriteString("    " + selectedDescStyle.Render(a.desc) + "\n")
		} else {
			b.WriteString(itemStyle.Render("  "+a.name) + "\n")
			b.WriteString("    " + descStyle.Render(a.desc) + "\n")
		}
	}

	return b.String()
}

func (m model) nextStepHint() string {
	if m.currentView != viewActionSelect {
		return ""
	}
	if !m.hasConfig {
		return "▶ Export Gitea state to get started"
	}
	if !m.hasState {
		return "▶ Run Reconcile (apply) to initialize state"
	}
	return ""
}

func (m model) renderStatusBar() string {
	width := m.width
	if width == 0 {
		width = 80
	}

	var result string

	// Hint line above status bar
	if hint := m.nextStepHint(); hint != "" {
		style := hintDimStyle
		if m.blinkOn {
			style = hintStyle
		}
		styledHint := style.Render(hint)
		hintPlain := stripAnsi(styledHint)
		pad := max((width-len(hintPlain))/2, 0)
		hintLine := strings.Repeat(" ", pad) + styledHint + strings.Repeat(" ", max(width-pad-len(hintPlain), 0))
		result += statusBarStyle.Render(hintLine) + "\n"
	}

	var left string
	switch m.currentView {
	case viewServerSelect:
		left = fmt.Sprintf(" %s servers available",
			statusKeyStyle.Render(fmt.Sprintf("%d", len(m.servers))))
	case viewActionSelect:
		srv := m.servers[m.serverIdx]

		stateIndicator := statusOkStyle.Render("✓")
		if !m.hasState {
			stateIndicator = statusNoStyle.Render("✗")
		}
		configIndicator := statusOkStyle.Render("✓")
		if !m.hasConfig {
			configIndicator = statusNoStyle.Render("✗")
		}

		left = fmt.Sprintf(" %s (%s) │ State: %s │ Config: %s",
			statusKeyStyle.Render(srv.Name), srv.URL,
			stateIndicator, configIndicator)
	}

	escLabel := "esc quit"
	extra := ""
	if m.currentView == viewActionSelect {
		escLabel = "esc back"
	}
	if m.currentView == viewServerSelect && m.serverIdx < len(m.servers) {
		extra = "d remove • "
	}
	right := fmt.Sprintf("↑/↓ navigate • enter select • %s%s ", extra, escLabel)

	leftPlain := stripAnsi(left)
	rightPlain := right
	gap := max(width-len(leftPlain)-len(rightPlain), 1)

	bar := left + strings.Repeat(" ", gap) + right
	result += statusBarStyle.Render(bar)
	return result
}

func stripAnsi(s string) string {
	var b strings.Builder
	inEsc := false
	for _, r := range s {
		if r == '\x1b' {
			inEsc = true
			continue
		}
		if inEsc {
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
				inEsc = false
			}
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// -- command execution --

func (m model) runAction() tea.Cmd {
	srv := m.servers[m.serverIdx]

	switch m.actionIdx {
	case 0: // dry-run
		return execWithPause("go", "run", "./cmd/reconcile/", "--server", srv.Name)
	case 1: // apply
		return execWithPause("go", "run", "./cmd/reconcile/", "--apply", "--server", srv.Name)
	case 2: // apply + auto-approve
		return execWithPause("go", "run", "./cmd/reconcile/", "--apply", "--auto-approve", "--server", srv.Name)
	case 3: // export
		return execWithPause("go", "run", "./cmd/export/", "--server", srv.Name)
	case 4: // reset memory
		return execWithPause(
			fmt.Sprintf(`rm -f %q && echo "Local memory deleted (%s removed)." && echo "Rebuilding memory from current YAML files..." &&`, srv.StateFile, srv.StateFile),
			"go", "run", "./cmd/reconcile/", "--apply", "--auto-approve", "--server", srv.Name,
		)
	case 5: // report
		return execWithPause("go", "run", "./cmd/report/", "--server", srv.Name)
	}
	return nil
}

func main() {
	p := tea.NewProgram(initialModel(), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}
}
