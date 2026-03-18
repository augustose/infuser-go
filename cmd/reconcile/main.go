package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/augustose/infuser-go/internal/api"
	"github.com/augustose/infuser-go/internal/config"
	"github.com/augustose/infuser-go/internal/engine"
)

func main() {
	apply := flag.Bool("apply", false, "Apply changes to Gitea (default is dry-run)")
	autoApprove := flag.Bool("auto-approve", false, "Skip confirmation prompt when applying")
	serverName := flag.String("server", "", "Target a specific server by name")
	flag.Parse()

	servers, err := config.LoadServers()
	if err != nil {
		log.Fatalf("Error loading config: %v", err)
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
