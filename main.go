package main

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/augustose/infuser-go/internal/config"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

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

	footerStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("241")).
			MarginTop(1)
)

// -- menu items --

type action struct {
	name string
	desc string
}

var actions = []action{
	{"Reconcile (dry-run)", "Shows what changes would be made without touching Gitea"},
	{"Reconcile (apply)", "Applies pending changes after interactive confirmation"},
	{"Reconcile (apply + auto-approve)", "Applies changes without confirmation (CI/CD)"},
	{"Export Gitea state", "Downloads users, orgs, repos into YAML files"},
	{"Reset local memory", "Deletes state file and rebuilds from current YAMLs"},
	{"Repository grid report", "Generates CSV+MD with repos, owners, and access info"},
}

// -- view enum --

type view int

const (
	viewServerSelect view = iota
	viewActionSelect
)

// -- model --

type model struct {
	servers     []config.ServerConfig
	serverIdx   int
	actionIdx   int
	currentView view
	width       int
	height      int
	err         error
	quitting    bool
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
	}

	return m
}

func (m model) Init() tea.Cmd {
	return nil
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tea.KeyMsg:
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
				if m.actionIdx > 0 {
					m.actionIdx--
				}
			}

		case "down", "j":
			switch m.currentView {
			case viewServerSelect:
				if m.serverIdx < len(m.servers)-1 {
					m.serverIdx++
				}
			case viewActionSelect:
				if m.actionIdx < len(actions)-1 {
					m.actionIdx++
				}
			}

		case "enter":
			switch m.currentView {
			case viewServerSelect:
				m.currentView = viewActionSelect
				m.actionIdx = 0
				return m, nil
			case viewActionSelect:
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

func (m model) View() string {
	if m.err != nil {
		return fmt.Sprintf("\n  Error: %v\n\n  Press any key to exit.\n", m.err)
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
	s := "\n" + header + "\n\n"
	s += "  Select a server:\n\n"

	for i, srv := range m.servers {
		name := srv.Name
		url := serverStyle.Render(fmt.Sprintf("(%s)", srv.URL))

		if i == m.serverIdx {
			s += selectedItemStyle.Render("▸ "+name) + " " + url + "\n"
		} else {
			s += itemStyle.Render("  "+name) + " " + url + "\n"
		}
	}

	s += footerStyle.Render("\n  ↑/↓ navigate • enter select • q quit")
	return s
}

func (m model) actionSelectView(header string) string {
	srv := m.servers[m.serverIdx]

	s := "\n" + header + "\n"
	s += serverStyle.Render(fmt.Sprintf("  Server: %s (%s)", srv.Name, srv.URL)) + "\n\n"

	for i, a := range actions {
		if i == m.actionIdx {
			s += selectedItemStyle.Render("▸ "+a.name) + "\n"
			s += "    " + selectedDescStyle.Render(a.desc) + "\n"
		} else {
			s += itemStyle.Render("  "+a.name) + "\n"
			s += "    " + descStyle.Render(a.desc) + "\n"
		}
	}

	back := ""
	if len(m.servers) > 1 {
		back = " • esc back"
	}
	s += footerStyle.Render(fmt.Sprintf("\n  ↑/↓ navigate • enter select%s • q quit", back))
	return s
}

// -- command execution --

func (m model) runAction() tea.Cmd {
	srv := m.servers[m.serverIdx]

	switch m.actionIdx {
	case 0: // dry-run
		return tea.ExecProcess(
			buildCmd("go", "run", "./cmd/reconcile/", "--server", srv.Name),
			func(err error) tea.Msg { return tea.KeyMsg{} },
		)
	case 1: // apply
		return tea.ExecProcess(
			buildCmd("go", "run", "./cmd/reconcile/", "--apply", "--server", srv.Name),
			func(err error) tea.Msg { return tea.KeyMsg{} },
		)
	case 2: // apply + auto-approve
		return tea.ExecProcess(
			buildCmd("go", "run", "./cmd/reconcile/", "--apply", "--auto-approve", "--server", srv.Name),
			func(err error) tea.Msg { return tea.KeyMsg{} },
		)
	case 3: // export
		return tea.ExecProcess(
			buildCmd("go", "run", "./cmd/export/", "--server", srv.Name),
			func(err error) tea.Msg { return tea.KeyMsg{} },
		)
	case 4: // reset memory
		return m.resetMemory(srv)
	case 5: // report
		return tea.ExecProcess(
			buildCmd("go", "run", "./cmd/report/", "--server", srv.Name),
			func(err error) tea.Msg { return tea.KeyMsg{} },
		)
	}
	return nil
}

func (m model) resetMemory(srv config.ServerConfig) tea.Cmd {
	return tea.ExecProcess(
		buildResetCmd(srv),
		func(err error) tea.Msg { return tea.KeyMsg{} },
	)
}

func buildCmd(name string, args ...string) *exec.Cmd {
	cmd := exec.Command(name, args...)
	return cmd
}

func buildResetCmd(srv config.ServerConfig) *exec.Cmd {
	// Remove state file then rebuild
	if _, err := os.Stat(srv.StateFile); err == nil {
		os.Remove(srv.StateFile)
		fmt.Printf("Local memory deleted (%s removed).\n", srv.StateFile)
	} else {
		fmt.Println("No local memory file found, nothing to delete.")
	}
	fmt.Println("Rebuilding memory from current YAML files...")

	return buildCmd("go", "run", "./cmd/reconcile/", "--apply", "--auto-approve", "--server", srv.Name)
}

func main() {
	p := tea.NewProgram(initialModel(), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}
}
