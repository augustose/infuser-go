# Browse Repositories & Deactivate User — Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add two new TUI actions: "Browse repositories" (table view of local YAML repos, edit in vim) and "Deactivate user" (list users from API, confirm, deactivate via API).

**Architecture:** Both features add new views to the existing Bubbletea TUI in main.go. Browse repos scans local YAML files and opens vim. Deactivate user calls the Gitea API directly. New API methods added to internal/api/client.go.

**Tech Stack:** Go, Bubbletea, Gitea API, vim

---

### Task 1: Add ListUsers and DeactivateUser API methods

**Files:**
- Modify: `internal/api/client.go`

**Step 1: Add ListUsers method**

```go
func (c *GiteaClient) ListUsers() ([]map[string]any, error) {
	return c.GetPaginated("/api/v1/admin/users")
}
```

**Step 2: Add DeactivateUser method**

```go
func (c *GiteaClient) DeactivateUser(username string) error {
	if err := c.checkWrite(); err != nil {
		return err
	}

	payload := map[string]any{
		"login_name":     username,
		"source_id":      0,
		"prohibit_login": true,
	}

	resp, err := c.doRequest("PATCH", "/api/v1/admin/users/"+username, payload, true)
	if err != nil {
		return err
	}
	body, _ := c.readBody(resp)

	if resp.StatusCode == 200 {
		return nil
	}
	return fmt.Errorf("deactivating user %s: HTTP %d: %s", username, resp.StatusCode, string(body))
}
```

**Step 3: Build and verify**

Run: `go build -o infuser-go .`

---

### Task 2: Add repo scanner function for local YAMLs

**Files:**
- Modify: `main.go`

**Step 1: Add repoEntry struct and scanLocalRepos function**

Struct to hold repo info for the table. Scanner walks `configDir/users/*/repositories/*.yaml` and `configDir/organizations/*/repositories/*.yaml`, extracts name, description, owner, private, and stores file paths for vim.

---

### Task 3: Add Browse Repositories view to TUI

**Files:**
- Modify: `main.go`

**Step 1: Add viewBrowseRepos to view enum and repoEntry fields to model**

**Step 2: Add "Browse repositories" action to actions slice**

**Step 3: Implement browseReposView() rendering a table with columns**

**Step 4: Handle navigation (j/k, Enter to open vim, Esc to go back)**

**Step 5: On Enter, exec vim with repo YAML + owner YAML**

**Step 6: Build and test manually**

---

### Task 4: Add Deactivate User view to TUI

**Files:**
- Modify: `main.go`

**Step 1: Add viewDeactivateUser to view enum and user fields to model**

**Step 2: Add "Deactivate user" action to actions slice**

**Step 3: Implement deactivateUserView() rendering user table**

**Step 4: On Enter, exec confirmation + API call via tea.ExecProcess**

**Step 5: Build and test manually**

---

### Task 5: Final build and commit

Run: `go build -o infuser-go .`
Commit all changes.
