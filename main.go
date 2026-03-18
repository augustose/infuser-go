package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/augustose/infuser-go/internal/config"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type cmdFinishedMsg struct{ err error }

// -- styles --

var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("39")).
			MarginBottom(1)

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

	footerStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("241")).
			MarginTop(1)
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
	err          error
	quitting     bool
	returningCmd bool
	hasState     bool
	hasConfig    bool
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
		return model{err: err}
	}

	m := model{
		servers:     servers,
		currentView: viewServerSelect,
	}

	// Skip server selection if only one server
	if len(servers) == 1 {
		m.serverIdx = 0
		m.currentView = viewActionSelect
		m.hasState = stateFileExists(servers[0])
		m.hasConfig = configDirExists(servers[0])
		m.actionIdx = m.firstEnabledAction()
	}

	return m
}

func (m model) Init() tea.Cmd {
	return nil
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case cmdFinishedMsg:
		m.returningCmd = true
		m.hasState = stateFileExists(m.servers[m.serverIdx])
		m.hasConfig = configDirExists(m.servers[m.serverIdx])
		return m, nil

	case tea.KeyMsg:
		if m.returningCmd {
			m.returningCmd = false
			return m, nil
		}

		if m.err != nil {
			m.quitting = true
			return m, tea.Quit
		}

		switch msg.String() {
		case "ctrl+c", "q":
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
				if m.serverIdx < len(m.servers)-1 {
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

		case "esc":
			if m.currentView == viewActionSelect && len(m.servers) > 1 {
				m.currentView = viewServerSelect
				return m, nil
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

func (m model) View() string {
	if m.err != nil {
		return fmt.Sprintf("\n  Error: %v\n\n  Press any key to exit.\n\n", m.err)
	}
	if m.quitting {
		return ""
	}

	header := titleStyle.Render("Infuser — Infrastructure as Code for Gitea")

	switch m.currentView {
	case viewServerSelect:
		return m.serverSelectView(header)
	case viewActionSelect:
		return m.actionSelectView(header)
	}

	return ""
}

func (m model) serverSelectView(header string) string {
	var b strings.Builder
	b.WriteString("\n" + header + "\n\n")
	b.WriteString("  Select a server:\n\n")

	for i, srv := range m.servers {
		name := srv.Name
		url := serverStyle.Render(fmt.Sprintf("(%s)", srv.URL))

		if i == m.serverIdx {
			b.WriteString(selectedItemStyle.Render("▸ "+name) + " " + url + "\n")
		} else {
			b.WriteString(itemStyle.Render("  "+name) + " " + url + "\n")
		}
	}

	b.WriteString(footerStyle.Render("\n  ↑/↓ navigate • enter select • q quit"))
	return b.String()
}

func (m model) actionSelectView(header string) string {
	srv := m.servers[m.serverIdx]

	var b strings.Builder
	b.WriteString("\n" + header + "\n")
	b.WriteString(serverStyle.Render(fmt.Sprintf("  Server: %s (%s)", srv.Name, srv.URL)) + "\n\n")

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

	back := ""
	if len(m.servers) > 1 {
		back = " • esc back"
	}
	b.WriteString(footerStyle.Render(fmt.Sprintf("\n  ↑/↓ navigate • enter select%s • q quit", back)))
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
