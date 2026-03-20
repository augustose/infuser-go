package engine

import (
	"bufio"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/augustose/infuser-go/internal/api"
	"github.com/augustose/infuser-go/internal/memory"
	"github.com/augustose/infuser-go/internal/parser"
	"github.com/augustose/infuser-go/internal/setup"
)

type Options struct {
	DryRun      bool
	AutoApprove bool
}

type action struct {
	description string
	execute     func() error
}

// RunEngine orchestrates parse -> diff -> execute for a single server.
func RunEngine(client *api.GiteaClient, opts Options, configDir, stateFile string) error {
	fmt.Println("========================================")
	fmt.Println("  Infuser - Reconciliation Engine")
	fmt.Println("========================================")

	if opts.DryRun {
		fmt.Println("[DRY RUN MODE] - Showing Execution Plan.")
	} else {
		fmt.Println("[APPLY MODE] - Evaluating changes to persist.")
	}

	fmt.Println("\n[1/3] Building Desired State from YAMLs...")
	desired, err := parser.ParseAllConfig(configDir)
	if err != nil {
		return fmt.Errorf("parsing config: %w", err)
	}
	fmt.Printf("  Users found: %d\n", len(desired.Users))
	fmt.Printf("  Organizations found: %d\n", len(desired.Organizations))

	fmt.Println("\n[2/3] Loading Local Memory (Previous State)...")
	mem := memory.NewMemory(stateFile)
	if err := mem.Load(); err != nil {
		return fmt.Errorf("loading memory: %w", err)
	}

	if len(mem.State.Users) == 0 && len(mem.State.Organizations) == 0 {
		return setup.RunWizard(client, configDir, stateFile, opts.AutoApprove)
	}

	fmt.Println("\n[3/3] Computing Diff Plan...")
	actions := computeDiff(client, mem.State, desired)

	for _, a := range actions {
		fmt.Println(a.description)
	}

	fmt.Println("\n----------------------------------------")
	if len(actions) == 0 {
		fmt.Println("EVERYTHING IN SYNC: Desired State (YAML) matches Local Memory.")
		return nil
	}

	fmt.Printf("Calculated %d changes to reconcile infrastructure.\n", len(actions))

	if opts.DryRun {
		fmt.Println("Finished. Run with --apply to commit these changes to the server.")
		return nil
	}

	if !opts.AutoApprove {
		fmt.Print("\nApply these changes to Gitea? (y/n): ")
		scanner := bufio.NewScanner(os.Stdin)
		scanner.Scan()
		resp := strings.ToLower(strings.TrimSpace(scanner.Text()))
		if resp != "y" && resp != "yes" {
			fmt.Println("Action canceled by user.")
			return nil
		}
	}

	fmt.Println("\nApplying changes...")
	for _, a := range actions {
		if err := a.execute(); err != nil {
			fmt.Printf("  ERROR: %v\n", err)
		}
	}

	fmt.Println("\nSaving new state to local memory...")
	if err := mem.Save(desired); err != nil {
		return err
	}
	fmt.Println("Done.")
	return nil
}

func computeDiff(client *api.GiteaClient, current, desired *parser.DesiredState) []action {
	var actions []action

	// --- Users ---
	currentUsers := setKeys(current.Users)
	desiredUsers := setKeys(desired.Users)

	for _, user := range sortedDiff(desiredUsers, currentUsers) {
		u := user
		spec := getSpec(desired.Users[u])
		actions = append(actions, action{
			description: fmt.Sprintf("  [CREATE] User: %s", u),
			execute:     func() error { return client.CreateUser(u, spec) },
		})
	}

	for _, user := range sortedDiff(currentUsers, desiredUsers) {
		u := user
		actions = append(actions, action{
			description: fmt.Sprintf("  [DELETE] User: %s", u),
			execute:     func() error { return client.DeleteUser(u) },
		})
	}

	for _, user := range sortedIntersect(desiredUsers, currentUsers) {
		u := user
		cUser := current.Users[u]
		dUser := desired.Users[u]

		// Diff user properties
		cSpec := getSpec(cUser)
		dSpec := getSpec(dUser)
		changed := diffSpecs(cSpec, dSpec, []string{"email", "full_name", "active"})
		if len(changed) > 0 {
			updateSpec := pickFields(dSpec, changed)
			actions = append(actions, action{
				description: fmt.Sprintf("  [UPDATE] User: %s -- changed: %s", u, strings.Join(changed, ", ")),
				execute:     func() error { return client.UpdateUser(u, updateSpec) },
			})
		}

		// Diff personal repos
		cRepos := getRepoKeys(cUser)
		dRepos := getRepoKeys(dUser)

		for _, repo := range sortedDiff(dRepos, cRepos) {
			r := repo
			rSpec := getRepoSpec(dUser, r)
			actions = append(actions, action{
				description: fmt.Sprintf("  [CREATE] Repository: %s (User: %s)", r, u),
				execute:     func() error { return client.CreateUserRepo(u, r, rSpec) },
			})
		}

		for _, repo := range sortedDiff(cRepos, dRepos) {
			r := repo
			actions = append(actions, action{
				description: fmt.Sprintf("  [DELETE] Repository: %s (User: %s)", r, u),
				execute:     func() error { return client.DeleteUserRepo(u, r) },
			})
		}

		for _, repo := range sortedIntersect(dRepos, cRepos) {
			r := repo
			crSpec := getRepoSpec(cUser, r)
			drSpec := getRepoSpec(dUser, r)
			rChanged := diffSpecs(crSpec, drSpec, []string{"description", "private", "default_branch"})
			if len(rChanged) > 0 {
				updateSpec := pickFields(drSpec, rChanged)
				actions = append(actions, action{
					description: fmt.Sprintf("  [UPDATE] Repository: %s (User: %s) -- changed: %s", r, u, strings.Join(rChanged, ", ")),
					execute:     func() error { return client.UpdateRepo(u, r, updateSpec) },
				})
			}
		}
	}

	// --- Organizations ---
	currentOrgs := setKeys(current.Organizations)
	desiredOrgs := setKeys(desired.Organizations)

	for _, org := range sortedDiff(desiredOrgs, currentOrgs) {
		o := org
		dOrg := desired.Organizations[o]
		spec := getSpec(dOrg)
		actions = append(actions, action{
			description: fmt.Sprintf("  [CREATE] Organization: %s", o),
			execute:     func() error { return client.CreateOrganization(o, spec) },
		})

		// Create teams and repos for new org
		teams := getSubMap(dOrg, "teams")
		for _, teamName := range sortedKeys(teams) {
			t := teamName
			tSpec := getSpec(teams[t].(map[string]any))
			actions = append(actions, action{
				description: fmt.Sprintf("  [CREATE] Team: %s (Org: %s)", t, o),
				execute: func() error {
					tID, err := client.CreateTeam(o, t, tSpec)
					if err != nil {
						return err
					}
					if tID != 0 {
						for _, m := range getMembers(tSpec) {
							if err := client.AddTeamMember(tID, m); err != nil {
								fmt.Printf("  ERROR adding member %s: %v\n", m, err)
							}
						}
					}
					return nil
				},
			})
		}

		repos := getSubMap(dOrg, "repositories")
		for _, repoName := range sortedKeys(repos) {
			r := repoName
			rSpec := getSpec(repos[r].(map[string]any))
			actions = append(actions, action{
				description: fmt.Sprintf("  [CREATE] Repository: %s (Org: %s)", r, o),
				execute:     func() error { return client.CreateOrgRepo(o, r, rSpec) },
			})
		}
	}

	for _, org := range sortedIntersect(desiredOrgs, currentOrgs) {
		o := org
		cOrg := current.Organizations[o]
		dOrg := desired.Organizations[o]

		// Diff org properties
		cOrgSpec := getSpec(cOrg)
		dOrgSpec := getSpec(dOrg)
		orgChanged := diffSpecs(cOrgSpec, dOrgSpec, []string{"description", "full_name"})
		if len(orgChanged) > 0 {
			updateSpec := pickFields(dOrgSpec, orgChanged)
			actions = append(actions, action{
				description: fmt.Sprintf("  [UPDATE] Organization: %s -- changed: %s", o, strings.Join(orgChanged, ", ")),
				execute:     func() error { return client.UpdateOrganization(o, updateSpec) },
			})
		}

		// Diff teams
		cTeams := getSubMap(cOrg, "teams")
		dTeams := getSubMap(dOrg, "teams")
		cTeamKeys := setFromMap(cTeams)
		dTeamKeys := setFromMap(dTeams)

		for _, team := range sortedDiff(dTeamKeys, cTeamKeys) {
			t := team
			tSpec := getSpec(dTeams[t].(map[string]any))
			actions = append(actions, action{
				description: fmt.Sprintf("  [CREATE] Team: %s (Org: %s)", t, o),
				execute: func() error {
					tID, err := client.CreateTeam(o, t, tSpec)
					if err != nil {
						return err
					}
					if tID != 0 {
						for _, m := range getMembers(tSpec) {
							if err := client.AddTeamMember(tID, m); err != nil {
								fmt.Printf("  ERROR adding member %s: %v\n", m, err)
							}
						}
					}
					return nil
				},
			})
		}

		for _, team := range sortedDiff(cTeamKeys, dTeamKeys) {
			t := team
			actions = append(actions, action{
				description: fmt.Sprintf("  [DELETE] Team: %s (Org: %s)", t, o),
				execute:     func() error { return client.DeleteTeam(o, t) },
			})
		}

		for _, team := range sortedIntersect(dTeamKeys, cTeamKeys) {
			t := team
			cTeamSpec := getSpec(cTeams[t].(map[string]any))
			dTeamSpec := getSpec(dTeams[t].(map[string]any))

			// Member diff
			cMembers := getMemberSet(cTeamSpec)
			dMembers := getMemberSet(dTeamSpec)

			for _, m := range sortedDiff(dMembers, cMembers) {
				mbr := m
				actions = append(actions, action{
					description: fmt.Sprintf("  [ADD MEMBER] '%s' -> Team: %s (Org: %s)", mbr, t, o),
					execute: func() error {
						tID, err := client.FindTeamID(o, t)
						if err != nil {
							return err
						}
						if tID != 0 {
							return client.AddTeamMember(tID, mbr)
						}
						return fmt.Errorf("team %s not found in %s", t, o)
					},
				})
			}

			for _, m := range sortedDiff(cMembers, dMembers) {
				mbr := m
				actions = append(actions, action{
					description: fmt.Sprintf("  [REMOVE MEMBER] '%s' <- Team: %s (Org: %s)", mbr, t, o),
					execute: func() error {
						tID, err := client.FindTeamID(o, t)
						if err != nil {
							return err
						}
						if tID != 0 {
							return client.RemoveTeamMember(tID, mbr)
						}
						return fmt.Errorf("team %s not found in %s", t, o)
					},
				})
			}

			// Team property diff
			teamChanged := diffSpecs(cTeamSpec, dTeamSpec, []string{"permission", "includes_all_repositories", "can_create_org_repo", "units_map"})
			if len(teamChanged) > 0 {
				updateSpec := pickFields(dTeamSpec, teamChanged)
				actions = append(actions, action{
					description: fmt.Sprintf("  [UPDATE] Team: %s (Org: %s) -- changed: %s", t, o, strings.Join(teamChanged, ", ")),
					execute:     func() error { return client.UpdateTeam(o, t, updateSpec) },
				})
			}
		}

		// Diff repos
		cRepos := getSubMap(cOrg, "repositories")
		dRepos := getSubMap(dOrg, "repositories")
		cRepoKeys := setFromMap(cRepos)
		dRepoKeys := setFromMap(dRepos)

		for _, repo := range sortedDiff(dRepoKeys, cRepoKeys) {
			r := repo
			rSpec := getSpec(dRepos[r].(map[string]any))
			actions = append(actions, action{
				description: fmt.Sprintf("  [CREATE] Repository: %s (Org: %s)", r, o),
				execute:     func() error { return client.CreateOrgRepo(o, r, rSpec) },
			})
		}

		for _, repo := range sortedDiff(cRepoKeys, dRepoKeys) {
			r := repo
			actions = append(actions, action{
				description: fmt.Sprintf("  [DELETE] Repository: %s (Org: %s)", r, o),
				execute:     func() error { return client.DeleteOrgRepo(o, r) },
			})
		}

		for _, repo := range sortedIntersect(dRepoKeys, cRepoKeys) {
			r := repo
			crSpec := getSpec(cRepos[r].(map[string]any))
			drSpec := getSpec(dRepos[r].(map[string]any))
			rChanged := diffSpecs(crSpec, drSpec, []string{"description", "private", "default_branch"})
			if len(rChanged) > 0 {
				updateSpec := pickFields(drSpec, rChanged)
				actions = append(actions, action{
					description: fmt.Sprintf("  [UPDATE] Repository: %s (Org: %s) -- changed: %s", r, o, strings.Join(rChanged, ", ")),
					execute:     func() error { return client.UpdateRepo(o, r, updateSpec) },
				})
			}
		}
	}

	return actions
}

// --- Helpers ---

func getSpec(m map[string]any) map[string]any {
	if s, ok := m["spec"].(map[string]any); ok {
		return s
	}
	return map[string]any{}
}

func getSubMap(m map[string]any, key string) map[string]any {
	if s, ok := m[key].(map[string]any); ok {
		return s
	}
	return map[string]any{}
}

func getRepoKeys(user map[string]any) map[string]bool {
	repos := getSubMap(user, "repositories")
	s := make(map[string]bool, len(repos))
	for k := range repos {
		s[k] = true
	}
	return s
}

func getRepoSpec(user map[string]any, repoName string) map[string]any {
	repos := getSubMap(user, "repositories")
	if r, ok := repos[repoName].(map[string]any); ok {
		return getSpec(r)
	}
	return map[string]any{}
}

func setKeys(m map[string]map[string]any) map[string]bool {
	s := make(map[string]bool, len(m))
	for k := range m {
		s[k] = true
	}
	return s
}

func setFromMap(m map[string]any) map[string]bool {
	s := make(map[string]bool, len(m))
	for k := range m {
		s[k] = true
	}
	return s
}

func sortedDiff(a, b map[string]bool) []string {
	var result []string
	for k := range a {
		if !b[k] {
			result = append(result, k)
		}
	}
	sort.Strings(result)
	return result
}

func sortedIntersect(a, b map[string]bool) []string {
	var result []string
	for k := range a {
		if b[k] {
			result = append(result, k)
		}
	}
	sort.Strings(result)
	return result
}

func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func diffSpecs(current, desired map[string]any, fields []string) []string {
	var changed []string
	for _, f := range fields {
		if fmt.Sprintf("%v", current[f]) != fmt.Sprintf("%v", desired[f]) {
			changed = append(changed, f)
		}
	}
	return changed
}

func pickFields(spec map[string]any, fields []string) map[string]any {
	result := make(map[string]any, len(fields))
	for _, f := range fields {
		result[f] = spec[f]
	}
	return result
}

func getMembers(spec map[string]any) []string {
	members, ok := spec["members"]
	if !ok {
		return nil
	}
	switch v := members.(type) {
	case []any:
		result := make([]string, 0, len(v))
		for _, m := range v {
			if s, ok := m.(string); ok {
				result = append(result, s)
			}
		}
		return result
	case []string:
		return v
	}
	return nil
}

func getMemberSet(spec map[string]any) map[string]bool {
	members := getMembers(spec)
	s := make(map[string]bool, len(members))
	for _, m := range members {
		s[m] = true
	}
	return s
}
