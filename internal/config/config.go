package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/joho/godotenv"
	"gopkg.in/yaml.v3"
)

type ServerConfig struct {
	Name        string
	URL         string
	ReadToken   string
	WriteToken  string
	AllowWrites bool
	ConfigDir   string
	StateFile   string
}

type serversFile struct {
	Servers []serverEntry `yaml:"servers"`
}

type serverEntry struct {
	Name          string `yaml:"name"`
	URL           string `yaml:"url"`
	ReadTokenEnv  string `yaml:"read_token_env"`
	WriteTokenEnv string `yaml:"write_token_env"`
	AllowWrites   bool   `yaml:"allow_writes"`
	ConfigDir     string `yaml:"config_dir"`
	StateFile     string `yaml:"state_file"`
}

// LoadServers reads servers.yaml if it exists, otherwise falls back to .env vars.
// It returns one or more ServerConfig instances.
func LoadServers() ([]ServerConfig, error) {
	// Load .env if present (makes env vars available for token resolution)
	_ = godotenv.Load()

	if data, err := os.ReadFile("servers.yaml"); err == nil {
		return loadFromYAML(data)
	}

	return loadFromEnv()
}

func loadFromYAML(data []byte) ([]ServerConfig, error) {
	var sf serversFile
	if err := yaml.Unmarshal(data, &sf); err != nil {
		return nil, fmt.Errorf("parsing servers.yaml: %w", err)
	}

	if len(sf.Servers) == 0 {
		return nil, fmt.Errorf("servers.yaml contains no server entries")
	}

	configs := make([]ServerConfig, 0, len(sf.Servers))
	for _, s := range sf.Servers {
		cfg := ServerConfig{
			Name:        s.Name,
			URL:         strings.TrimRight(s.URL, "/"),
			ReadToken:   os.Getenv(s.ReadTokenEnv),
			WriteToken:  os.Getenv(s.WriteTokenEnv),
			AllowWrites: s.AllowWrites,
			ConfigDir:   s.ConfigDir,
			StateFile:   s.StateFile,
		}

		if cfg.ConfigDir == "" {
			cfg.ConfigDir = filepath.Join("infuser-config", s.Name)
		}
		if cfg.StateFile == "" {
			cfg.StateFile = fmt.Sprintf(".infuser_state_%s.json", s.Name)
		}
		if cfg.ReadToken == "" {
			fmt.Printf("WARNING: %s read token env var %q is empty\n", s.Name, s.ReadTokenEnv)
		}

		configs = append(configs, cfg)
	}

	return configs, nil
}

func loadFromEnv() ([]ServerConfig, error) {
	url := strings.TrimRight(os.Getenv("GITEA_URL"), "/")
	if url == "" {
		return nil, fmt.Errorf("no servers.yaml found and GITEA_URL is not set")
	}

	readToken := os.Getenv("GITEA_READ_TOKEN")
	if readToken == "" {
		readToken = os.Getenv("GITEA_TOKEN")
	}

	allowWrites := strings.ToLower(os.Getenv("GITEA_ALLOW_WRITES"))

	cfg := ServerConfig{
		Name:        "default",
		URL:         url,
		ReadToken:   readToken,
		WriteToken:  os.Getenv("GITEA_WRITE_TOKEN"),
		AllowWrites: allowWrites == "true" || allowWrites == "1" || allowWrites == "yes",
		ConfigDir:   "infuser-config",
		StateFile:   ".infuser_state.json",
	}

	if cfg.ReadToken == "" {
		fmt.Println("WARNING: GITEA_READ_TOKEN is not set. Read operations will fail.")
	}

	return []ServerConfig{cfg}, nil
}
