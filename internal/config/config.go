package config

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// Config contains runtime configuration for the example service.
type Config struct {
	Port               string
	LogLevel           string
	ClearnodeWSURL     string
	DemoPrivateKey     string
	SQLitePath         string
	StoreName          string
	StoreAppID         string
	StoreAppPrivateKey string
	BlockchainRPCURLs  map[string]string
	HomeBlockchains    map[string]uint64
}

// Load reads process env and an optional .env file into a validated Config.
func Load(dotenvPath string) (*Config, error) {
	if err := loadDotEnv(dotenvPath); err != nil {
		return nil, fmt.Errorf("failed to load dotenv: %w", err)
	}

	cfg := &Config{
		Port:               getEnv("PORT", "8080"),
		LogLevel:           getEnv("LOG_LEVEL", "info"),
		ClearnodeWSURL:     os.Getenv("CLEARNODE_WS_URL"),
		DemoPrivateKey:     os.Getenv("DEMO_PRIVATE_KEY"),
		SQLitePath:         getEnv("SQLITE_PATH", "./data/nitrolite-store-example.db"),
		StoreName:          getEnv("STORE_NAME", "Nitrolite App Session Store"),
		StoreAppID:         getEnv("STORE_APP_ID", "default"),
		StoreAppPrivateKey: strings.TrimSpace(os.Getenv("STORE_APP_PRIVATE_KEY")),
	}

	if err := parseJSONEnv("BLOCKCHAIN_RPC_URLS", &cfg.BlockchainRPCURLs); err != nil {
		return nil, err
	}
	if err := parseJSONEnv("HOME_BLOCKCHAINS", &cfg.HomeBlockchains); err != nil {
		return nil, err
	}

	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

// Validate checks required configuration.
func (c *Config) Validate() error {
	switch {
	case c.ClearnodeWSURL == "":
		return fmt.Errorf("missing CLEARNODE_WS_URL")
	case c.DemoPrivateKey == "":
		return fmt.Errorf("missing DEMO_PRIVATE_KEY")
	case strings.TrimSpace(c.SQLitePath) == "":
		return fmt.Errorf("missing SQLITE_PATH")
	case strings.TrimSpace(c.StoreName) == "":
		return fmt.Errorf("missing STORE_NAME")
	case strings.TrimSpace(c.StoreAppID) == "":
		return fmt.Errorf("missing STORE_APP_ID")
	case len(c.BlockchainRPCURLs) == 0:
		return fmt.Errorf("missing BLOCKCHAIN_RPC_URLS")
	case len(c.HomeBlockchains) == 0:
		return fmt.Errorf("missing HOME_BLOCKCHAINS")
	}

	return nil
}

func parseJSONEnv[T any](key string, target *T) error {
	raw := os.Getenv(key)
	if raw == "" {
		return fmt.Errorf("missing %s", key)
	}
	if err := json.Unmarshal([]byte(raw), target); err != nil {
		return fmt.Errorf("invalid %s: %w", key, err)
	}
	return nil
}

func getEnv(key string, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func loadDotEnv(path string) error {
	if path == "" {
		return nil
	}

	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		key, value, ok := strings.Cut(line, "=")
		if !ok {
			return fmt.Errorf("invalid dotenv line: %q", line)
		}

		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if os.Getenv(key) != "" {
			continue
		}
		if err := os.Setenv(key, value); err != nil {
			return fmt.Errorf("failed to set %s from dotenv: %w", key, err)
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("failed to scan dotenv: %w", err)
	}

	return nil
}
