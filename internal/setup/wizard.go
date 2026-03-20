package setup

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/augustose/infuser-go/internal/api"
	"github.com/augustose/infuser-go/internal/export"
	"github.com/augustose/infuser-go/internal/memory"
	"github.com/augustose/infuser-go/internal/parser"
)

// RunWizard guides the user through first-time setup for a server.
func RunWizard(client *api.GiteaClient, configDir, stateFile string, autoApprove bool) error {
	fmt.Println()
	fmt.Println("=== Infuser Setup Wizard ===")
	fmt.Println()

	// Step 1: Test API connection
	fmt.Println("[1/4] Testing API connection...")
	version, readOK, writeOK, err := client.Ping()
	if err != nil {
		return fmt.Errorf("connection failed: %w", err)
	}
	if !readOK {
		return fmt.Errorf("read token is invalid — cannot authenticate with %s", client.BaseURL())
	}

	fmt.Printf("  URL:         %s\n", client.BaseURL())
	if version != "" {
		fmt.Printf("  Version:     %s\n", version)
	}
	fmt.Printf("  Read token:  %s\n", boolStatus(readOK))
	fmt.Printf("  Write token: %s\n", boolStatus(writeOK))
	fmt.Println()

	// Step 2: Export current server state
	fmt.Println("[2/4] Export current server state")
	if askYesNo("Export current server state to YAML files?", autoApprove) {
		fmt.Println("  Exporting...")
		if err := export.ExportState(client, configDir); err != nil {
			return fmt.Errorf("export failed: %w", err)
		}
		fmt.Println("  Export complete.")
	} else {
		fmt.Println("  Skipped export. Will initialize from existing YAML files.")
	}
	fmt.Println()

	// Step 3: Save baseline to state file
	fmt.Println("[3/4] Saving baseline state...")
	desired, err := parser.ParseAllConfig(configDir)
	if err != nil {
		return fmt.Errorf("failed to parse config from %s: %w", configDir, err)
	}

	if len(desired.Users) == 0 && len(desired.Organizations) == 0 {
		fmt.Println("  WARNING: No users or organizations found in YAML files.")
		fmt.Println("  The next run will trigger the setup wizard again.")
		fmt.Println("  Consider running the export step or adding YAML files manually.")
		return nil
	}

	mem := memory.NewMemory(stateFile)
	if err := mem.Save(desired); err != nil {
		return fmt.Errorf("failed to save state file: %w", err)
	}
	fmt.Printf("  State file saved: %s\n", stateFile)
	fmt.Println()

	// Step 4: Setup complete
	fmt.Println("[4/4] Setup complete!")
	fmt.Println()
	fmt.Println("Next steps:")
	fmt.Println("  1. Review/edit YAML files in: " + configDir)
	fmt.Println("  2. Run a dry-run reconciliation to preview changes")
	fmt.Println("  3. Apply to push changes to the server")
	fmt.Println()
	fmt.Println("Use the TUI menu (go run .) or the CLI directly.")
	fmt.Println()

	return nil
}

// askYesNo prompts the user with a yes/no question. If autoApprove is true,
// it automatically answers yes without waiting for input.
func askYesNo(prompt string, autoApprove bool) bool {
	if autoApprove {
		fmt.Printf("  %s [y/n]: y (auto-approved)\n", prompt)
		return true
	}

	reader := bufio.NewReader(os.Stdin)
	for {
		fmt.Printf("  %s [y/n]: ", prompt)
		input, _ := reader.ReadString('\n')
		input = strings.TrimSpace(strings.ToLower(input))
		switch input {
		case "y", "yes":
			return true
		case "n", "no":
			return false
		default:
			fmt.Println("  Please enter y or n.")
		}
	}
}

// boolStatus returns a human-readable status string for a boolean flag.
func boolStatus(ok bool) string {
	if ok {
		return "OK"
	}
	return "not configured"
}
