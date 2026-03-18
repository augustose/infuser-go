package export

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/augustose/infuser-go/internal/api"
	"gopkg.in/yaml.v3"
)

// ExportState downloads the current Gitea state and writes it to YAML files.
func ExportState(client *api.GiteaClient, exportDir string) error {
	if err := exportUsers(client, exportDir); err != nil {
		return err
	}
	if err := exportOrganizations(client, exportDir); err != nil {
		return err
	}
	fmt.Printf("\nExport completed! Check folder: %s\n", exportDir)
	return nil
}

func writeYAML(path string, data any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	out, err := yaml.Marshal(data)
	if err != nil {
		return err
	}
	return os.WriteFile(path, out, 0644)
}

func exportUsers(client *api.GiteaClient, exportDir string) error {
	fmt.Println("Exporting users...")
	users, err := client.GetPaginated("/api/v1/admin/users")
	if err != nil {
		return fmt.Errorf("fetching users: %w", err)
	}

	for _, u := range users {
		username, _ := u["login"].(string)
		if username == "" {
			continue
		}

		userDir := filepath.Join(exportDir, "users", username)

		userData := map[string]any{
			"apiVersion": "v1",
			"kind":       "User",
			"metadata":   map[string]any{"name": username},
			"spec": map[string]any{
				"email":     u["email"],
				"full_name": getString(u, "full_name"),
				"is_admin":  getBool(u, "is_admin"),
				"active":    getBool(u, "active"),
			},
		}

		if err := writeYAML(filepath.Join(userDir, "user.yaml"), userData); err != nil {
			fmt.Printf("Error writing user %s: %v\n", username, err)
			continue
		}

		// Personal repositories
		repos, err := client.GetPaginated(fmt.Sprintf("/api/v1/users/%s/repos", username))
		if err != nil {
			fmt.Printf("Error fetching repos for %s: %v\n", username, err)
			continue
		}

		for _, repo := range repos {
			repoName, _ := repo["name"].(string)
			if repoName == "" {
				continue
			}

			collaborators := fetchCollaborators(client, username, repoName, username)
			protections := fetchBranchProtections(client, username, repoName)

			repoData := map[string]any{
				"apiVersion": "v1",
				"kind":       "Repository",
				"metadata":   map[string]any{"name": repoName, "owner": username},
				"spec": map[string]any{
					"private":            repo["private"],
					"description":        getString(repo, "description"),
					"default_branch":     getString(repo, "default_branch"),
					"collaborators":      collaborators,
					"branch_protections": protections,
				},
			}

			if err := writeYAML(filepath.Join(userDir, "repositories", repoName+".yaml"), repoData); err != nil {
				fmt.Printf("Error writing repo %s/%s: %v\n", username, repoName, err)
			}
		}
	}
	return nil
}

func exportOrganizations(client *api.GiteaClient, exportDir string) error {
	fmt.Println("Exporting organizations, teams, and repositories...")
	orgs, err := client.GetPaginated("/api/v1/admin/orgs")
	if err != nil {
		return fmt.Errorf("fetching orgs: %w", err)
	}

	for _, org := range orgs {
		orgName, _ := org["username"].(string)
		if orgName == "" {
			continue
		}

		orgDir := filepath.Join(exportDir, "organizations", orgName)

		// Org definition
		orgData := map[string]any{
			"apiVersion": "v1",
			"kind":       "Organization",
			"metadata":   map[string]any{"name": orgName},
			"spec": map[string]any{
				"description": getString(org, "description"),
				"full_name":   getString(org, "full_name"),
			},
		}
		if err := writeYAML(filepath.Join(orgDir, "org.yaml"), orgData); err != nil {
			fmt.Printf("Error writing org %s: %v\n", orgName, err)
		}

		// Teams
		teams, err := client.GetPaginated(fmt.Sprintf("/api/v1/orgs/%s/teams", orgName))
		if err != nil {
			fmt.Printf("Error fetching teams for %s: %v\n", orgName, err)
		} else {
			for _, team := range teams {
				teamName, _ := team["name"].(string)
				if teamName == "" {
					continue
				}
				normalizedName := normalizeTeamName(teamName)

				teamID := getFloat64(team, "id")
				members, _ := client.GetPaginated(fmt.Sprintf("/api/v1/teams/%d/members", int64(teamID)))
				var memberLogins []string
				for _, m := range members {
					if login, ok := m["login"].(string); ok {
						memberLogins = append(memberLogins, login)
					}
				}

				unitsMap, _ := team["units_map"]

				teamData := map[string]any{
					"apiVersion": "v1",
					"kind":       "Team",
					"metadata":   map[string]any{"name": normalizedName},
					"spec": map[string]any{
						"permission":                team["permission"],
						"includes_all_repositories": getBool(team, "includes_all_repositories"),
						"can_create_org_repo":       getBool(team, "can_create_org_repo"),
						"units_map":                 unitsMap,
						"members":                   memberLogins,
					},
				}
				if err := writeYAML(filepath.Join(orgDir, "teams", normalizedName+".yaml"), teamData); err != nil {
					fmt.Printf("Error writing team %s/%s: %v\n", orgName, normalizedName, err)
				}
			}
		}

		// Repositories
		repos, err := client.GetPaginated(fmt.Sprintf("/api/v1/orgs/%s/repos", orgName))
		if err != nil {
			fmt.Printf("Error fetching repos for org %s: %v\n", orgName, err)
		} else {
			for _, repo := range repos {
				repoName, _ := repo["name"].(string)
				if repoName == "" {
					continue
				}

				collaborators := fetchCollaborators(client, orgName, repoName, "")
				protections := fetchBranchProtections(client, orgName, repoName)

				repoData := map[string]any{
					"apiVersion": "v1",
					"kind":       "Repository",
					"metadata":   map[string]any{"name": repoName, "owner": orgName},
					"spec": map[string]any{
						"private":            repo["private"],
						"description":        getString(repo, "description"),
						"default_branch":     getString(repo, "default_branch"),
						"collaborators":      collaborators,
						"branch_protections": protections,
					},
				}
				if err := writeYAML(filepath.Join(orgDir, "repositories", repoName+".yaml"), repoData); err != nil {
					fmt.Printf("Error writing repo %s/%s: %v\n", orgName, repoName, err)
				}
			}
		}
	}
	return nil
}

func fetchCollaborators(client *api.GiteaClient, owner, repoName, skipUser string) map[string]string {
	collabs, err := client.GetPaginated(fmt.Sprintf("/api/v1/repos/%s/%s/collaborators", owner, repoName))
	if err != nil {
		return nil
	}

	result := make(map[string]string)
	for _, c := range collabs {
		login, _ := c["login"].(string)
		if login == "" || login == skipUser {
			continue
		}

		roleName, _ := c["role_name"].(string)
		if roleName == "" {
			perms, _ := c["permissions"].(map[string]any)
			if perms != nil {
				if getBool(perms, "admin") {
					roleName = "admin"
				} else if getBool(perms, "push") {
					roleName = "write"
				} else {
					roleName = "read"
				}
			}
		}
		result[login] = roleName
	}
	return result
}

func fetchBranchProtections(client *api.GiteaClient, owner, repoName string) []map[string]any {
	bps, err := client.GetPaginated(fmt.Sprintf("/api/v1/repos/%s/%s/branch_protections", owner, repoName))
	if err != nil {
		return nil
	}

	var result []map[string]any
	for _, bp := range bps {
		result = append(result, map[string]any{
			"branch_name":        bp["branch_name"],
			"enable_push":        getBool(bp, "enable_push"),
			"required_approvals": getFloat64(bp, "required_approvals"),
		})
	}
	return result
}

func normalizeTeamName(name string) string {
	result := make([]byte, 0, len(name))
	for i := 0; i < len(name); i++ {
		c := name[i]
		if c == ' ' {
			result = append(result, '-')
		} else if c >= 'A' && c <= 'Z' {
			result = append(result, c+32)
		} else {
			result = append(result, c)
		}
	}
	return string(result)
}

func getString(m map[string]any, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

func getBool(m map[string]any, key string) bool {
	if v, ok := m[key].(bool); ok {
		return v
	}
	return false
}

func getFloat64(m map[string]any, key string) float64 {
	if v, ok := m[key].(float64); ok {
		return v
	}
	return 0
}
