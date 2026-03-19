package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/augustose/infuser-go/internal/config"
	"github.com/augustose/infuser-go/internal/report"
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

		if err := report.GenerateRepoGrid(srv.ConfigDir, srv.URL, srv.Name); err != nil {
			fmt.Fprintf(os.Stderr, "Error generating report for %s: %v\n", srv.Name, err)
		}
	}
}
