package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/augustose/infuser-go/internal/api"
	"github.com/augustose/infuser-go/internal/config"
	"github.com/augustose/infuser-go/internal/export"
)

func main() {
	serverName := flag.String("server", "", "Target a specific server by name")
	flag.Parse()

	servers, err := config.LoadServers()
	if err != nil {
		log.Fatalf("Error loading config: %v", err)
	}

	for _, srv := range servers {
		if *serverName != "" && srv.Name != *serverName {
			continue
		}

		if len(servers) > 1 {
			fmt.Printf("\n>>> Server: %s (%s)\n\n", srv.Name, srv.URL)
		}

		client := api.NewClient(&srv)
		if err := export.ExportState(client, srv.ConfigDir); err != nil {
			fmt.Fprintf(os.Stderr, "Error exporting %s: %v\n", srv.Name, err)
		}
	}
}
