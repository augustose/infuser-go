package api

import (
	"bytes"
	"crypto/rand"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"strings"

	"github.com/augustose/infuser-go/internal/config"
)

type GiteaClient struct {
	baseURL     string
	readToken   string
	writeToken  string
	allowWrites bool
	httpClient  *http.Client
}

func NewClient(cfg *config.ServerConfig) *GiteaClient {
	return &GiteaClient{
		baseURL:     cfg.URL,
		readToken:   cfg.ReadToken,
		writeToken:  cfg.WriteToken,
		allowWrites: cfg.AllowWrites,
		httpClient: &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
			},
		},
	}
}

func (c *GiteaClient) BaseURL() string {
	return c.baseURL
}

func (c *GiteaClient) checkWrite() error {
	if !c.allowWrites {
		return fmt.Errorf("write blocked: GITEA_ALLOW_WRITES is not enabled")
	}
	return nil
}

func (c *GiteaClient) doRequest(method, path string, body any, useWriteToken bool) (*http.Response, error) {
	var bodyReader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		bodyReader = bytes.NewReader(data)
	}

	url := c.baseURL + path
	req, err := http.NewRequest(method, url, bodyReader)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	token := c.readToken
	if useWriteToken {
		token = c.writeToken
	}
	if token != "" {
		req.Header.Set("Authorization", "token "+token)
	}

	return c.httpClient.Do(req)
}

func (c *GiteaClient) readBody(resp *http.Response) ([]byte, error) {
	defer resp.Body.Close()
	return io.ReadAll(resp.Body)
}

// GetPaginated fetches all pages from a Gitea API list endpoint.
func (c *GiteaClient) GetPaginated(path string) ([]map[string]any, error) {
	var results []map[string]any
	page := 1
	limit := 50

	for {
		sep := "?"
		if strings.Contains(path, "?") {
			sep = "&"
		}
		pagedPath := fmt.Sprintf("%s%spage=%d&limit=%d", path, sep, page, limit)

		resp, err := c.doRequest("GET", pagedPath, nil, false)
		if err != nil {
			return results, err
		}

		body, err := c.readBody(resp)
		if err != nil {
			return results, err
		}

		if resp.StatusCode != 200 {
			return results, fmt.Errorf("HTTP %d on %s: %s", resp.StatusCode, pagedPath, string(body))
		}

		var data []map[string]any
		if err := json.Unmarshal(body, &data); err != nil {
			return results, err
		}

		if len(data) == 0 {
			break
		}

		results = append(results, data...)

		if len(data) < limit {
			break
		}
		page++
	}

	return results, nil
}

// --- User operations ---

func (c *GiteaClient) CreateUser(username string, spec map[string]any) error {
	if err := c.checkWrite(); err != nil {
		return err
	}

	payload := map[string]any{
		"login_name":           username,
		"username":             username,
		"email":                spec["email"],
		"full_name":            mapGetString(spec, "full_name", ""),
		"password":             generateTempPassword(),
		"must_change_password": true,
		"send_notify":          true,
	}

	resp, err := c.doRequest("POST", "/api/v1/admin/users", payload, true)
	if err != nil {
		return err
	}
	body, _ := c.readBody(resp)

	if resp.StatusCode == 201 {
		fmt.Printf("  [API] User %s created successfully.\n", username)
		return nil
	}
	return fmt.Errorf("creating user %s: HTTP %d: %s", username, resp.StatusCode, string(body))
}

func (c *GiteaClient) DeleteUser(username string) error {
	if err := c.checkWrite(); err != nil {
		return err
	}

	resp, err := c.doRequest("DELETE", "/api/v1/admin/users/"+username, nil, true)
	if err != nil {
		return err
	}
	body, _ := c.readBody(resp)

	if resp.StatusCode == 200 || resp.StatusCode == 204 {
		fmt.Printf("  [API] User %s deleted.\n", username)
		return nil
	}
	return fmt.Errorf("deleting user %s: HTTP %d: %s", username, resp.StatusCode, string(body))
}

func (c *GiteaClient) UpdateUser(username string, changed map[string]any) error {
	if err := c.checkWrite(); err != nil {
		return err
	}

	payload := map[string]any{"login_name": username, "source_id": 0}
	for _, key := range []string{"email", "full_name", "active"} {
		if v, ok := changed[key]; ok {
			payload[key] = v
		}
	}

	resp, err := c.doRequest("PATCH", "/api/v1/admin/users/"+username, payload, true)
	if err != nil {
		return err
	}
	body, _ := c.readBody(resp)

	if resp.StatusCode == 200 {
		fmt.Printf("  [API] User %s updated.\n", username)
		return nil
	}
	return fmt.Errorf("updating user %s: HTTP %d: %s", username, resp.StatusCode, string(body))
}

// --- Organization operations ---

func (c *GiteaClient) CreateOrganization(name string, spec map[string]any) error {
	if err := c.checkWrite(); err != nil {
		return err
	}

	payload := map[string]any{
		"username":    name,
		"full_name":   mapGetString(spec, "full_name", ""),
		"description": mapGetString(spec, "description", ""),
	}

	resp, err := c.doRequest("POST", "/api/v1/orgs", payload, true)
	if err != nil {
		return err
	}
	body, _ := c.readBody(resp)

	if resp.StatusCode == 201 {
		fmt.Printf("  [API] Organization %s created.\n", name)
		return nil
	}
	return fmt.Errorf("creating org %s: HTTP %d: %s", name, resp.StatusCode, string(body))
}

func (c *GiteaClient) UpdateOrganization(name string, changed map[string]any) error {
	if err := c.checkWrite(); err != nil {
		return err
	}

	payload := map[string]any{}
	for _, key := range []string{"description", "full_name"} {
		if v, ok := changed[key]; ok {
			payload[key] = v
		}
	}
	if len(payload) == 0 {
		return nil
	}

	resp, err := c.doRequest("PATCH", "/api/v1/orgs/"+name, payload, true)
	if err != nil {
		return err
	}
	body, _ := c.readBody(resp)

	if resp.StatusCode == 200 {
		fmt.Printf("  [API] Organization %s updated.\n", name)
		return nil
	}
	return fmt.Errorf("updating org %s: HTTP %d: %s", name, resp.StatusCode, string(body))
}

// --- Team operations ---

func (c *GiteaClient) CreateTeam(orgName, teamName string, spec map[string]any) (int64, error) {
	if err := c.checkWrite(); err != nil {
		return 0, err
	}

	payload := map[string]any{
		"name":                       teamName,
		"permission":                 mapGetString(spec, "permission", "read"),
		"includes_all_repositories":  mapGetBool(spec, "includes_all_repositories", false),
		"can_create_org_repo":        mapGetBool(spec, "can_create_org_repo", false),
	}
	if um, ok := spec["units_map"]; ok {
		payload["units_map"] = um
	}

	resp, err := c.doRequest("POST", fmt.Sprintf("/api/v1/orgs/%s/teams", orgName), payload, true)
	if err != nil {
		return 0, err
	}
	body, _ := c.readBody(resp)

	if resp.StatusCode == 201 {
		fmt.Printf("  [API] Team %s (%s) created.\n", teamName, orgName)
		var result map[string]any
		if err := json.Unmarshal(body, &result); err == nil {
			if id, ok := result["id"].(float64); ok {
				return int64(id), nil
			}
		}
		return 0, nil
	}
	return 0, fmt.Errorf("creating team %s: HTTP %d: %s", teamName, resp.StatusCode, string(body))
}

func (c *GiteaClient) DeleteTeam(orgName, teamName string) error {
	teamID, err := c.FindTeamID(orgName, teamName)
	if err != nil {
		return err
	}
	if teamID == 0 {
		return fmt.Errorf("team %s not found in %s", teamName, orgName)
	}

	if err := c.checkWrite(); err != nil {
		return err
	}

	resp, err := c.doRequest("DELETE", fmt.Sprintf("/api/v1/teams/%d", teamID), nil, true)
	if err != nil {
		return err
	}
	body, _ := c.readBody(resp)

	if resp.StatusCode == 200 || resp.StatusCode == 204 {
		fmt.Printf("  [API] Team %s deleted.\n", teamName)
		return nil
	}
	return fmt.Errorf("deleting team %s: HTTP %d: %s", teamName, resp.StatusCode, string(body))
}

func (c *GiteaClient) UpdateTeam(orgName, teamName string, changed map[string]any) error {
	if err := c.checkWrite(); err != nil {
		return err
	}

	teamID, err := c.FindTeamID(orgName, teamName)
	if err != nil {
		return err
	}
	if teamID == 0 {
		return fmt.Errorf("team %s not found in %s", teamName, orgName)
	}

	payload := map[string]any{}
	for _, key := range []string{"permission", "includes_all_repositories", "can_create_org_repo"} {
		if v, ok := changed[key]; ok {
			payload[key] = v
		}
	}
	if um, ok := changed["units_map"]; ok {
		payload["units_map"] = um
	}
	if len(payload) == 0 {
		return nil
	}

	resp, err := c.doRequest("PATCH", fmt.Sprintf("/api/v1/teams/%d", teamID), payload, true)
	if err != nil {
		return err
	}
	body, _ := c.readBody(resp)

	if resp.StatusCode == 200 {
		fmt.Printf("  [API] Team %s (%s) updated.\n", teamName, orgName)
		return nil
	}
	return fmt.Errorf("updating team %s: HTTP %d: %s", teamName, resp.StatusCode, string(body))
}

func (c *GiteaClient) FindTeamID(orgName, teamName string) (int64, error) {
	resp, err := c.doRequest("GET", fmt.Sprintf("/api/v1/orgs/%s/teams", orgName), nil, false)
	if err != nil {
		return 0, err
	}
	body, _ := c.readBody(resp)

	if resp.StatusCode != 200 {
		return 0, fmt.Errorf("listing teams for %s: HTTP %d", orgName, resp.StatusCode)
	}

	var teams []map[string]any
	if err := json.Unmarshal(body, &teams); err != nil {
		return 0, err
	}

	normalizedTarget := strings.ToLower(strings.ReplaceAll(teamName, " ", "-"))
	for _, t := range teams {
		name, _ := t["name"].(string)
		if strings.ToLower(strings.ReplaceAll(name, " ", "-")) == normalizedTarget {
			if id, ok := t["id"].(float64); ok {
				return int64(id), nil
			}
		}
	}
	return 0, nil
}

func (c *GiteaClient) AddTeamMember(teamID int64, username string) error {
	if err := c.checkWrite(); err != nil {
		return err
	}

	resp, err := c.doRequest("PUT", fmt.Sprintf("/api/v1/teams/%d/members/%s", teamID, username), nil, true)
	if err != nil {
		return err
	}
	body, _ := c.readBody(resp)

	if resp.StatusCode == 200 || resp.StatusCode == 204 {
		fmt.Printf("  [API] User %s added to team %d.\n", username, teamID)
		return nil
	}
	return fmt.Errorf("adding %s to team %d: HTTP %d: %s", username, teamID, resp.StatusCode, string(body))
}

func (c *GiteaClient) RemoveTeamMember(teamID int64, username string) error {
	if err := c.checkWrite(); err != nil {
		return err
	}

	resp, err := c.doRequest("DELETE", fmt.Sprintf("/api/v1/teams/%d/members/%s", teamID, username), nil, true)
	if err != nil {
		return err
	}
	body, _ := c.readBody(resp)

	if resp.StatusCode == 200 || resp.StatusCode == 204 {
		fmt.Printf("  [API] User %s removed from team %d.\n", username, teamID)
		return nil
	}
	return fmt.Errorf("removing %s from team %d: HTTP %d: %s", username, teamID, resp.StatusCode, string(body))
}

// --- Repository operations ---

func (c *GiteaClient) CreateOrgRepo(orgName, repoName string, spec map[string]any) error {
	if err := c.checkWrite(); err != nil {
		return err
	}

	payload := map[string]any{
		"name":           repoName,
		"private":        mapGetBool(spec, "private", true),
		"description":    mapGetString(spec, "description", ""),
		"default_branch": mapGetString(spec, "default_branch", "main"),
	}

	resp, err := c.doRequest("POST", fmt.Sprintf("/api/v1/orgs/%s/repos", orgName), payload, true)
	if err != nil {
		return err
	}
	body, _ := c.readBody(resp)

	if resp.StatusCode == 201 {
		fmt.Printf("  [API] Repository %s/%s created.\n", orgName, repoName)
		return nil
	}
	return fmt.Errorf("creating repo %s/%s: HTTP %d: %s", orgName, repoName, resp.StatusCode, string(body))
}

func (c *GiteaClient) DeleteOrgRepo(orgName, repoName string) error {
	if err := c.checkWrite(); err != nil {
		return err
	}

	resp, err := c.doRequest("DELETE", fmt.Sprintf("/api/v1/repos/%s/%s", orgName, repoName), nil, true)
	if err != nil {
		return err
	}
	body, _ := c.readBody(resp)

	if resp.StatusCode == 200 || resp.StatusCode == 204 {
		fmt.Printf("  [API] Repository %s/%s deleted.\n", orgName, repoName)
		return nil
	}
	return fmt.Errorf("deleting repo %s/%s: HTTP %d: %s", orgName, repoName, resp.StatusCode, string(body))
}

func (c *GiteaClient) CreateUserRepo(username, repoName string, spec map[string]any) error {
	if err := c.checkWrite(); err != nil {
		return err
	}

	payload := map[string]any{
		"name":           repoName,
		"private":        mapGetBool(spec, "private", true),
		"description":    mapGetString(spec, "description", ""),
		"default_branch": mapGetString(spec, "default_branch", "main"),
	}

	resp, err := c.doRequest("POST", fmt.Sprintf("/api/v1/admin/users/%s/repos", username), payload, true)
	if err != nil {
		return err
	}
	body, _ := c.readBody(resp)

	if resp.StatusCode == 201 {
		fmt.Printf("  [API] Repository %s/%s created.\n", username, repoName)
		return nil
	}
	return fmt.Errorf("creating repo %s/%s: HTTP %d: %s", username, repoName, resp.StatusCode, string(body))
}

func (c *GiteaClient) DeleteUserRepo(username, repoName string) error {
	if err := c.checkWrite(); err != nil {
		return err
	}

	resp, err := c.doRequest("DELETE", fmt.Sprintf("/api/v1/repos/%s/%s", username, repoName), nil, true)
	if err != nil {
		return err
	}
	body, _ := c.readBody(resp)

	if resp.StatusCode == 200 || resp.StatusCode == 204 {
		fmt.Printf("  [API] Repository %s/%s deleted.\n", username, repoName)
		return nil
	}
	return fmt.Errorf("deleting repo %s/%s: HTTP %d: %s", username, repoName, resp.StatusCode, string(body))
}

func (c *GiteaClient) UpdateRepo(owner, repoName string, changed map[string]any) error {
	if err := c.checkWrite(); err != nil {
		return err
	}

	payload := map[string]any{}
	for _, key := range []string{"description", "private", "default_branch"} {
		if v, ok := changed[key]; ok {
			payload[key] = v
		}
	}
	if len(payload) == 0 {
		return nil
	}

	resp, err := c.doRequest("PATCH", fmt.Sprintf("/api/v1/repos/%s/%s", owner, repoName), payload, true)
	if err != nil {
		return err
	}
	body, _ := c.readBody(resp)

	if resp.StatusCode == 200 {
		fmt.Printf("  [API] Repository %s/%s updated.\n", owner, repoName)
		return nil
	}
	return fmt.Errorf("updating repo %s/%s: HTTP %d: %s", owner, repoName, resp.StatusCode, string(body))
}

// --- Helpers ---

func mapGetString(m map[string]any, key, fallback string) string {
	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return fallback
}

func mapGetBool(m map[string]any, key string, fallback bool) bool {
	if v, ok := m[key]; ok {
		if b, ok := v.(bool); ok {
			return b
		}
	}
	return fallback
}

func generateTempPassword() string {
	const chars = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789!@#$%"
	b := make([]byte, 24)
	for i := range b {
		n, _ := rand.Int(rand.Reader, big.NewInt(int64(len(chars))))
		b[i] = chars[n.Int64()]
	}
	return string(b)
}
