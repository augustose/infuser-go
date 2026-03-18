package main

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/augustose/infuser-go/internal/config"
)

type menuItem struct {
	name string
	desc string
}

var actions = []menuItem{
	{"Reconcile (dry-run)", "Shows what changes would be made without touching Gitea"},
	{"Reconcile (apply)", "Applies pending changes after interactive confirmation"},
	{"Reconcile (apply + auto-approve)", "Applies changes without confirmation (CI/CD)"},
	{"Export Gitea state", "Downloads users, orgs, repos into YAML files"},
	{"Reset local memory", "Deletes state file and rebuilds from current YAMLs"},
	{"Repository grid report", "Generates CSV+MD with repos, owners, and access info"},
}

func main() {
	scanner := bufio.NewScanner(os.Stdin)

	for {
		clearScreen()
		printHeader()

		servers, err := config.LoadServers()
		if err != nil {
			fmt.Printf("Error loading config: %v\n", err)
			fmt.Print("\nPress Enter to exit...")
			scanner.Scan()
			return
		}

		serverIdx := 0
		if len(servers) > 1 {
			serverIdx = selectServer(scanner, servers)
			if serverIdx < 0 {
				return
			}
			clearScreen()
			printHeader()
		}

		srv := servers[serverIdx]

		fmt.Printf("  Server: %s (%s)\n\n", srv.Name, srv.URL)

		for i, item := range actions {
			fmt.Printf("  %d. %-35s %s\n", i+1, item.name, item.desc)
		}
		fmt.Printf("  0. Exit\n")
		fmt.Println()
		fmt.Print("Select an option: ")

		scanner.Scan()
		choice := strings.TrimSpace(scanner.Text())

		if choice == "0" || choice == "" {
			clearScreen()
			fmt.Println("Bye.")
			return
		}

		idx, err := strconv.Atoi(choice)
		if err != nil || idx < 1 || idx > len(actions) {
			fmt.Println("Invalid option.")
			waitForEnter(scanner)
			continue
		}

		clearScreen()
		fmt.Printf(">>> %s\n\n", actions[idx-1].name)
		runAction(idx, srv)
		waitForEnter(scanner)
	}
}

func runAction(idx int, srv config.ServerConfig) {
	exe, _ := os.Executable()
	goCmd := "go"

	// Try to use the built binary path pattern, fall back to go run
	_ = exe

	switch idx {
	case 1: // dry-run
		run(goCmd, "run", "./cmd/reconcile/", "--server", srv.Name)
	case 2: // apply
		run(goCmd, "run", "./cmd/reconcile/", "--apply", "--server", srv.Name)
	case 3: // apply + auto-approve
		run(goCmd, "run", "./cmd/reconcile/", "--apply", "--auto-approve", "--server", srv.Name)
	case 4: // export
		run(goCmd, "run", "./cmd/export/", "--server", srv.Name)
	case 5: // reset memory
		resetMemory(srv)
	case 6: // report
		run(goCmd, "run", "./cmd/report/", "--server", srv.Name)
	}
}

func resetMemory(srv config.ServerConfig) {
	if _, err := os.Stat(srv.StateFile); err == nil {
		os.Remove(srv.StateFile)
		fmt.Printf("Local memory deleted (%s removed).\n", srv.StateFile)
	} else {
		fmt.Println("No local memory file found, nothing to delete.")
	}

	fmt.Println("Rebuilding memory from current YAML files...")
	run("go", "run", "./cmd/reconcile/", "--apply", "--auto-approve", "--server", srv.Name)
	fmt.Println("\nLocal memory has been reset.")
}

func selectServer(scanner *bufio.Scanner, servers []config.ServerConfig) int {
	fmt.Println("  Available servers:\n")
	for i, s := range servers {
		fmt.Printf("  %d. %s (%s)\n", i+1, s.Name, s.URL)
	}
	fmt.Printf("  0. Exit\n")
	fmt.Println()
	fmt.Print("Select server: ")

	scanner.Scan()
	choice := strings.TrimSpace(scanner.Text())

	if choice == "0" || choice == "" {
		return -1
	}

	idx, err := strconv.Atoi(choice)
	if err != nil || idx < 1 || idx > len(servers) {
		return 0 // default to first
	}
	return idx - 1
}

func run(name string, args ...string) {
	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	if err := cmd.Run(); err != nil {
		fmt.Printf("Error: %v\n", err)
	}
}

func clearScreen() {
	cmd := exec.Command("clear")
	cmd.Stdout = os.Stdout
	cmd.Run()
}

func printHeader() {
	fmt.Println("========================================")
	fmt.Println("  Infuser")
	fmt.Println("  Infrastructure as Code for Gitea")
	fmt.Println("========================================")
	fmt.Println()
}

func waitForEnter(scanner *bufio.Scanner) {
	fmt.Print("\nPress Enter to return to menu...")
	scanner.Scan()
}
