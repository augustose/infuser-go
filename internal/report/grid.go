package report

import (
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/augustose/infuser-go/internal/parser"
)

// Row represents a single repository entry in the grid report.
type Row struct {
	Repository   string
	URL          string
	Description  string
	Organization string
	Owner        string
	Users        []string
}

// buildRows parses the config directory and returns sorted report rows.
func buildRows(configDir, serverURL string) ([]Row, error) {
	state, err := parser.ParseAllConfig(configDir)
	if err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}

	// Find global admins
	var globalAdmins []string
	for username, userData := range state.Users {
		spec, _ := userData["spec"].(map[string]any)
		if isAdmin, ok := spec["is_admin"].(bool); ok && isAdmin {
			globalAdmins = append(globalAdmins, username)
		}
	}

	var rows []Row

	// Organization repositories
	for orgName, orgData := range state.Organizations {
		teams, _ := orgData["teams"].(map[string]any)
		repos, _ := orgData["repositories"].(map[string]any)

		for repoName, rData := range repos {
			rd, _ := rData.(map[string]any)
			rSpec, _ := rd["spec"].(map[string]any)
			description, _ := rSpec["description"].(string)

			usersWithAccess := make(map[string]bool)

			// Via teams with includes_all_repositories
			for _, tData := range teams {
				td, _ := tData.(map[string]any)
				tSpec, _ := td["spec"].(map[string]any)
				if includesAll, ok := tSpec["includes_all_repositories"].(bool); ok && includesAll {
					for _, m := range getStringSlice(tSpec, "members") {
						usersWithAccess[m] = true
					}
				}
			}

			// Via direct collaborators
			if collabs, ok := rSpec["collaborators"].(map[string]any); ok {
				for collab := range collabs {
					usersWithAccess[collab] = true
				}
			}

			// Global admins
			for _, admin := range globalAdmins {
				usersWithAccess[admin] = true
			}

			repoURL := buildRepoURL(serverURL, orgName, repoName)

			rows = append(rows, Row{
				Repository:   repoName,
				URL:          repoURL,
				Description:  description,
				Organization: orgName,
				Owner:        orgName,
				Users:        sortedSet(usersWithAccess),
			})
		}
	}

	// Personal repositories
	for username, userData := range state.Users {
		repos, _ := userData["repositories"].(map[string]any)
		for repoName, rData := range repos {
			rd, _ := rData.(map[string]any)
			rSpec, _ := rd["spec"].(map[string]any)
			description, _ := rSpec["description"].(string)

			usersWithAccess := map[string]bool{username: true}

			if collabs, ok := rSpec["collaborators"].(map[string]any); ok {
				for collab := range collabs {
					usersWithAccess[collab] = true
				}
			}

			for _, admin := range globalAdmins {
				usersWithAccess[admin] = true
			}

			repoURL := buildRepoURL(serverURL, username, repoName)

			rows = append(rows, Row{
				Repository:   repoName,
				URL:          repoURL,
				Description:  description,
				Organization: "-",
				Owner:        username,
				Users:        sortedSet(usersWithAccess),
			})
		}
	}

	// Sort by org then repo name
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Organization != rows[j].Organization {
			return rows[i].Organization < rows[j].Organization
		}
		return rows[i].Repository < rows[j].Repository
	})

	return rows, nil
}

// GenerateRepoGrid creates CSV and Markdown reports of the repository access grid.
func GenerateRepoGrid(configDir, serverURL, serverName string) error {
	rows, err := buildRows(configDir, serverURL)
	if err != nil {
		return err
	}

	reportDir := filepath.Join("output", "reports", "grid")
	if err := os.MkdirAll(reportDir, 0755); err != nil {
		return err
	}

	timestamp := time.Now().Format("2006-01-02_150405")

	// Write CSV
	csvPath := filepath.Join(reportDir, fmt.Sprintf("repo_grid_%s.csv", timestamp))
	csvFile, err := os.Create(csvPath)
	if err != nil {
		return err
	}
	defer csvFile.Close()

	w := csv.NewWriter(csvFile)
	w.Write([]string{"Repository", "URL", "Description", "Organization", "Owner", "Users with Access"})
	for _, r := range rows {
		w.Write([]string{r.Repository, r.URL, r.Description, r.Organization, r.Owner, strings.Join(r.Users, ", ")})
	}
	w.Flush()

	// Write Markdown
	mdPath := filepath.Join(reportDir, fmt.Sprintf("repo_grid_%s.md", timestamp))
	mdFile, err := os.Create(mdPath)
	if err != nil {
		return err
	}
	defer mdFile.Close()

	fmt.Fprintf(mdFile, "# Repository Access Grid\n\n")
	fmt.Fprintf(mdFile, "> **Generated on:** `%s`\n\n", timestamp)
	fmt.Fprintln(mdFile, "| Repository | URL | Description | Organization | Owner | Users with Access |")
	fmt.Fprintln(mdFile, "|---|---|---|---|---|---|")

	for _, r := range rows {
		usersStr := strings.Join(r.Users, ", ")
		if usersStr == "" {
			usersStr = "-"
		}
		desc := strings.ReplaceAll(r.Description, "|", "\\|")
		fmt.Fprintf(mdFile, "| %s | %s | %s | %s | %s | %s |\n", r.Repository, r.URL, desc, r.Organization, r.Owner, usersStr)
	}

	// Write HTML
	htmlPath, err := writeHTML(rows, serverName, reportDir, timestamp)
	if err != nil {
		return err
	}

	fmt.Printf("Repository grid report generated:\n")
	fmt.Printf("  CSV:  %s\n", csvPath)
	fmt.Printf("  MD:   %s\n", mdPath)
	fmt.Printf("  HTML: %s\n", htmlPath)
	return nil
}

func buildRepoURL(serverURL, owner, repo string) string {
	if serverURL == "" {
		return ""
	}
	return fmt.Sprintf("%s/%s/%s", serverURL, owner, repo)
}

func getStringSlice(m map[string]any, key string) []string {
	v, ok := m[key]
	if !ok {
		return nil
	}
	switch val := v.(type) {
	case []any:
		result := make([]string, 0, len(val))
		for _, item := range val {
			if s, ok := item.(string); ok {
				result = append(result, s)
			}
		}
		return result
	case []string:
		return val
	}
	return nil
}

func sortedSet(m map[string]bool) []string {
	result := make([]string, 0, len(m))
	for k := range m {
		result = append(result, k)
	}
	sort.Strings(result)
	return result
}
