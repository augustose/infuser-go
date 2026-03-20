package setup

import (
	"bufio"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"

	"github.com/augustose/infuser-go/internal/api"
	"github.com/augustose/infuser-go/internal/config"
	"github.com/joho/godotenv"
)

var nonAlphaNum = regexp.MustCompile(`[^A-Z0-9]+`)

// RunAddServer guides the user through interactively adding a new server to servers.yaml.
func RunAddServer() (*config.ServerConfig, error) {
	reader := bufio.NewReader(os.Stdin)

	fmt.Println()
	fmt.Println("=== Add New Server ===")
	fmt.Println()

	// Step 1: Server name and URL
	name := promptString(reader, "Server name: ")
	if name == "" {
		return nil, fmt.Errorf("server name cannot be empty")
	}

	url := promptString(reader, "Server URL: ")
	if url == "" {
		return nil, fmt.Errorf("server URL cannot be empty")
	}
	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		return nil, fmt.Errorf("URL must start with http:// or https://")
	}
	url = strings.TrimRight(url, "/")

	fmt.Println()

	// Step 2: Test raw connection (no auth)
	fmt.Printf("[1/4] Testing connection to %s...\n", url)
	version, err := testConnection(url)
	if err != nil {
		return nil, fmt.Errorf("connection failed: %w", err)
	}
	if version != "" {
		fmt.Printf("  Connected (%s)\n", version)
	} else {
		fmt.Println("  Connected")
	}
	fmt.Println()

	// Step 3: Token setup
	fmt.Println("[2/4] Token setup")
	readToken := promptString(reader, "  Enter API token: ")
	if readToken == "" {
		return nil, fmt.Errorf("token cannot be empty")
	}

	// Generate default env var name from server name
	envPrefix := nonAlphaNum.ReplaceAllString(strings.ToUpper(name), "_")
	defaultReadEnv := envPrefix + "_READ_TOKEN"

	readEnvName := promptStringDefault(reader, fmt.Sprintf("  Token name for .env file [%s]: ", defaultReadEnv), defaultReadEnv)

	// Check .env for duplicates
	_ = godotenv.Load()
	fmt.Printf("  Checking .env for existing %s... ", readEnvName)
	if existing := os.Getenv(readEnvName); existing != "" {
		fmt.Println("already exists!")
		fmt.Println("  WARNING: This env var already has a value. It will be overwritten in .env.")
		if !askYesNo("  Continue?", false) {
			return nil, fmt.Errorf("aborted by user")
		}
	} else {
		fmt.Println("not found, OK.")
	}

	// Write token
	var writeToken string
	var writeEnvName string
	fmt.Println()
	if askYesNo("  Use same token for writes?", false) {
		writeToken = readToken
		writeEnvName = envPrefix + "_WRITE_TOKEN"
	} else {
		writeToken = promptString(reader, "  Enter write token (or leave empty to skip): ")
		if writeToken != "" {
			defaultWriteEnv := envPrefix + "_WRITE_TOKEN"
			writeEnvName = promptStringDefault(reader, fmt.Sprintf("  Write token name for .env [%s]: ", defaultWriteEnv), defaultWriteEnv)
		}
	}

	// Save tokens to .env
	if err := appendToEnvFile(readEnvName, readToken); err != nil {
		return nil, fmt.Errorf("saving read token to .env: %w", err)
	}
	fmt.Printf("  Saved %s to .env\n", readEnvName)

	if writeToken != "" && writeEnvName != "" {
		if err := appendToEnvFile(writeEnvName, writeToken); err != nil {
			return nil, fmt.Errorf("saving write token to .env: %w", err)
		}
		fmt.Printf("  Saved %s to .env\n", writeEnvName)
	}

	// Reload .env so tokens are available for verification
	_ = godotenv.Overload()
	os.Setenv(readEnvName, readToken)
	if writeToken != "" && writeEnvName != "" {
		os.Setenv(writeEnvName, writeToken)
	}

	fmt.Println()

	// Step 4: Verify tokens
	fmt.Println("[3/4] Verifying token...")
	configDir := fmt.Sprintf("infuser-config/%s", strings.ToLower(name))
	stateFile := fmt.Sprintf(".infuser_state_%s.json", strings.ToLower(name))

	cfg := &config.ServerConfig{
		Name:        name,
		URL:         url,
		ReadToken:   readToken,
		WriteToken:  writeToken,
		AllowWrites: writeToken != "",
		ConfigDir:   configDir,
		StateFile:   stateFile,
	}

	client := api.NewClient(cfg)
	_, readOK, writeOK, err := client.Ping()
	if err != nil {
		return nil, fmt.Errorf("verification failed: %w", err)
	}

	if readOK {
		fmt.Println("  Read token: OK")
	} else {
		fmt.Println("  Read token: FAILED")
		return nil, fmt.Errorf("read token is invalid for %s", url)
	}

	if writeToken != "" {
		if writeOK {
			fmt.Println("  Write token: OK")
		} else {
			fmt.Println("  Write token: FAILED (writes will be disabled)")
			cfg.AllowWrites = false
		}
	}

	fmt.Println()

	// Step 5: Save to servers.yaml
	fmt.Println("[4/4] Saving configuration...")

	entry := config.ServerEntry{
		Name:          name,
		URL:           url,
		ReadTokenEnv:  readEnvName,
		AllowWrites:   cfg.AllowWrites,
		ConfigDir:     configDir,
		StateFile:     stateFile,
	}
	if writeEnvName != "" {
		entry.WriteTokenEnv = writeEnvName
	}

	if err := config.AppendServerToYAML(entry); err != nil {
		return nil, fmt.Errorf("saving to servers.yaml: %w", err)
	}

	fmt.Printf("  Added \"%s\" to servers.yaml\n", name)
	fmt.Printf("  Config dir: %s/\n", configDir)
	fmt.Printf("  State file: %s\n", stateFile)
	fmt.Println()
	fmt.Println("  Server added! Returning to menu...")
	fmt.Println()

	return cfg, nil
}

func promptString(reader *bufio.Reader, prompt string) string {
	fmt.Print(prompt)
	input, _ := reader.ReadString('\n')
	return strings.TrimSpace(input)
}

func promptStringDefault(reader *bufio.Reader, prompt, defaultVal string) string {
	fmt.Print(prompt)
	input, _ := reader.ReadString('\n')
	input = strings.TrimSpace(input)
	if input == "" {
		return defaultVal
	}
	return input
}

func testConnection(baseURL string) (string, error) {
	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}

	resp, err := client.Get(baseURL + "/api/v1/version")
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("HTTP %d from %s", resp.StatusCode, baseURL)
	}

	var data map[string]any
	if err := json.Unmarshal(body, &data); err == nil {
		if v, ok := data["version"].(string); ok {
			return v, nil
		}
	}

	return "", nil
}

func appendToEnvFile(key, value string) error {
	// Ensure existing file ends with a newline before appending
	if data, err := os.ReadFile(".env"); err == nil && len(data) > 0 && data[len(data)-1] != '\n' {
		f, err := os.OpenFile(".env", os.O_APPEND|os.O_WRONLY, 0600)
		if err != nil {
			return err
		}
		_, _ = f.WriteString("\n")
		f.Close()
	}

	f, err := os.OpenFile(".env", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = fmt.Fprintf(f, "%s=\"%s\"\n", key, value)
	return err
}
