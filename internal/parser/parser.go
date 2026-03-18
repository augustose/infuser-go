package parser

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// DesiredState represents the full desired infrastructure state.
type DesiredState struct {
	Users         map[string]map[string]any `json:"users"`
	Organizations map[string]map[string]any `json:"organizations"`
}

// NewDesiredState returns an initialized empty state.
func NewDesiredState() *DesiredState {
	return &DesiredState{
		Users:         make(map[string]map[string]any),
		Organizations: make(map[string]map[string]any),
	}
}

func readYAML(path string) (map[string]any, error) {
	ext := filepath.Ext(path)
	if ext != ".yaml" && ext != ".yml" {
		return nil, nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var result map[string]any
	if err := yaml.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	return result, nil
}

// ParseAllConfig reads the infuser-config directory and returns the desired state.
func ParseAllConfig(configDir string) (*DesiredState, error) {
	state := NewDesiredState()

	if _, err := os.Stat(configDir); os.IsNotExist(err) {
		fmt.Printf("Directory not found: %s\n", configDir)
		return state, nil
	}

	// Parse users
	usersDir := filepath.Join(configDir, "users")
	if info, err := os.Stat(usersDir); err == nil && info.IsDir() {
		entries, _ := os.ReadDir(usersDir)
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}

			userName := entry.Name()
			userPath := filepath.Join(usersDir, userName)

			data, err := readYAML(filepath.Join(userPath, "user.yaml"))
			if err != nil {
				fmt.Printf("Error reading user %s: %v\n", userName, err)
				continue
			}
			if data == nil {
				continue
			}

			kind, _ := data["kind"].(string)
			if kind != "User" {
				continue
			}

			metadata, _ := data["metadata"].(map[string]any)
			name, _ := metadata["name"].(string)
			if name == "" {
				continue
			}

			data["repositories"] = make(map[string]any)

			// Parse personal repositories
			reposDir := filepath.Join(userPath, "repositories")
			if info, err := os.Stat(reposDir); err == nil && info.IsDir() {
				repoEntries, _ := os.ReadDir(reposDir)
				for _, re := range repoEntries {
					repoData, err := readYAML(filepath.Join(reposDir, re.Name()))
					if err != nil {
						fmt.Printf("Error reading repo %s/%s: %v\n", userName, re.Name(), err)
						continue
					}
					if repoData == nil {
						continue
					}

					repoKind, _ := repoData["kind"].(string)
					if repoKind != "Repository" {
						continue
					}

					repoMeta, _ := repoData["metadata"].(map[string]any)
					repoName, _ := repoMeta["name"].(string)
					if repoName != "" {
						repos := data["repositories"].(map[string]any)
						repos[repoName] = repoData
					}
				}
			}

			state.Users[name] = data
		}
	}

	// Parse organizations
	orgsDir := filepath.Join(configDir, "organizations")
	if info, err := os.Stat(orgsDir); err == nil && info.IsDir() {
		entries, _ := os.ReadDir(orgsDir)
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}

			orgName := entry.Name()
			orgPath := filepath.Join(orgsDir, orgName)
			orgState := map[string]any{
				"teams":        make(map[string]any),
				"repositories": make(map[string]any),
			}

			// Org definition
			orgDef, err := readYAML(filepath.Join(orgPath, "org.yaml"))
			if err != nil {
				fmt.Printf("Error reading org %s: %v\n", orgName, err)
			}
			if orgDef != nil {
				if meta, ok := orgDef["metadata"]; ok {
					orgState["metadata"] = meta
				}
				if spec, ok := orgDef["spec"]; ok {
					orgState["spec"] = spec
				}
			}

			// Teams
			teamsDir := filepath.Join(orgPath, "teams")
			if info, err := os.Stat(teamsDir); err == nil && info.IsDir() {
				teamEntries, _ := os.ReadDir(teamsDir)
				for _, te := range teamEntries {
					if te.IsDir() || (!strings.HasSuffix(te.Name(), ".yaml") && !strings.HasSuffix(te.Name(), ".yml")) {
						continue
					}
					teamData, err := readYAML(filepath.Join(teamsDir, te.Name()))
					if err != nil {
						fmt.Printf("Error reading team %s/%s: %v\n", orgName, te.Name(), err)
						continue
					}
					if teamData == nil {
						continue
					}

					teamKind, _ := teamData["kind"].(string)
					if teamKind != "Team" {
						continue
					}

					teamMeta, _ := teamData["metadata"].(map[string]any)
					teamName, _ := teamMeta["name"].(string)
					if teamName != "" {
						teams := orgState["teams"].(map[string]any)
						teams[teamName] = teamData
					}
				}
			}

			// Repositories
			reposDir := filepath.Join(orgPath, "repositories")
			if info, err := os.Stat(reposDir); err == nil && info.IsDir() {
				repoEntries, _ := os.ReadDir(reposDir)
				for _, re := range repoEntries {
					if re.IsDir() || (!strings.HasSuffix(re.Name(), ".yaml") && !strings.HasSuffix(re.Name(), ".yml")) {
						continue
					}
					repoData, err := readYAML(filepath.Join(reposDir, re.Name()))
					if err != nil {
						fmt.Printf("Error reading repo %s/%s: %v\n", orgName, re.Name(), err)
						continue
					}
					if repoData == nil {
						continue
					}

					repoKind, _ := repoData["kind"].(string)
					if repoKind != "Repository" {
						continue
					}

					repoMeta, _ := repoData["metadata"].(map[string]any)
					repoName, _ := repoMeta["name"].(string)
					if repoName != "" {
						repos := orgState["repositories"].(map[string]any)
						repos[repoName] = repoData
					}
				}
			}

			state.Organizations[orgName] = orgState
		}
	}

	return state, nil
}
