package main

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Depth       int    `yaml:"depth"`
	Interval    int    `yaml:"interval"`
	FullPath    bool   `yaml:"full_path"`
	Technical   bool   `yaml:"technical"`
	DefaultPath string `yaml:"default_path"`
}

func configPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "sir", "config.yaml"), nil
}

func loadConfig() Config {
	p, err := configPath()
	if err != nil {
		return Config{}
	}
	data, err := os.ReadFile(p)
	if err != nil {
		return Config{}
	}
	var cfg Config
	_ = yaml.Unmarshal(data, &cfg)
	return cfg
}

func initConfig() error {
	p, err := configPath()
	if err != nil {
		return err
	}
	if _, err := os.Stat(p); err == nil {
		return fmt.Errorf("config already exists at %s", p)
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	sample := `# sir configuration
# All values here are defaults; CLI flags always take precedence.

# depth: how many directory levels to scan for docker-compose files (default: 1)
depth: 1

# interval: TUI auto-refresh interval in seconds (default: 2)
interval: 2

# full_path: show absolute path to compose file instead of basename (default: false)
full_path: false

# technical: show image and ports columns by default (default: false)
technical: false

# default_path: scan this path when no argument is given (default: "")
# default_path: ~/projects
`
	if err := os.WriteFile(p, []byte(sample), 0o644); err != nil {
		return err
	}
	cCyan.Printf("  ✓ Created config at %s\n", p)
	return nil
}
