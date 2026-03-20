package main

import (
	"bufio"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/augustose/infuser-go/internal/api"
	"github.com/augustose/infuser-go/internal/config"
	"github.com/augustose/infuser-go/internal/engine"
)

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func removeServer(srv config.ServerConfig, autoApprove bool) {
	stateExists := fileExists(srv.StateFile)
	configExists := dirExists(srv.ConfigDir)

	if !stateExists && !configExists {
		fmt.Printf("Nothing to remove for server %q — no state file or config dir found.\n", srv.Name)
		return
	}

	fmt.Printf("Will remove local files for server %q:\n", srv.Name)
	if stateExists {
		fmt.Printf("  State file: %s\n", srv.StateFile)
	}
	if configExists {
		fmt.Printf("  Config dir: %s\n", srv.ConfigDir)
	}
	fmt.Println()
	fmt.Println("WARNING: This does NOT touch the actual server.")
	fmt.Println()

	if !autoApprove {
		fmt.Print("Are you sure? (y/n): ")
		reader := bufio.NewReader(os.Stdin)
		answer, _ := reader.ReadString('\n')
		answer = strings.TrimSpace(strings.ToLower(answer))
		if answer != "y" && answer != "yes" {
			fmt.Println("Aborted.")
			return
		}
	}

	if stateExists {
		if err := os.Remove(srv.StateFile); err != nil {
			fmt.Fprintf(os.Stderr, "Error removing state file: %v\n", err)
		} else {
			fmt.Printf("Removed state file: %s\n", srv.StateFile)
		}
	}

	if configExists {
		// Safety check: only remove dirs that look like infuser config
		if !dirExists(filepath.Join(srv.ConfigDir, "users")) && !dirExists(filepath.Join(srv.ConfigDir, "organizations")) {
			fmt.Fprintf(os.Stderr, "Refusing to remove %s — does not look like an infuser config directory.\n", srv.ConfigDir)
		} else if err := os.RemoveAll(srv.ConfigDir); err != nil {
			fmt.Fprintf(os.Stderr, "Error removing config dir: %v\n", err)
		} else {
			fmt.Printf("Removed config dir: %s\n", srv.ConfigDir)
		}
	}

	fmt.Println("Done. Remove the entry from servers.yaml manually if needed.")
}

func main() {
	apply := flag.Bool("apply", false, "Apply changes to Gitea (default is dry-run)")
	autoApprove := flag.Bool("auto-approve", false, "Skip confirmation prompt when applying")
	serverName := flag.String("server", "", "Target a specific server by name")
	remove := flag.Bool("remove", false, "Remove local state file and config dir for a server")
	flag.Parse()

	servers, err := config.LoadServers()
	if err != nil {
		log.Fatalf("Error loading config: %v", err)
	}

	if *remove {
		if *serverName == "" {
			log.Fatal("--remove requires --server to specify which server to remove")
		}

		for _, srv := range servers {
			if srv.Name == *serverName {
				removeServer(srv, *autoApprove)
				return
			}
		}
		log.Fatalf("Server %q not found in configuration", *serverName)
	}

	opts := engine.Options{
		DryRun:      !*apply,
		AutoApprove: *autoApprove,
	}

	for _, srv := range servers {
		if *serverName != "" && srv.Name != *serverName {
			continue
		}

		if len(servers) > 1 {
			fmt.Printf("\n>>> Server: %s (%s)\n\n", srv.Name, srv.URL)
		}

		client := api.NewClient(&srv)
		if err := engine.RunEngine(client, opts, srv.ConfigDir, srv.StateFile); err != nil {
			fmt.Fprintf(os.Stderr, "Error on server %s: %v\n", srv.Name, err)
		}
	}
}
