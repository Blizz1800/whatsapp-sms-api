package config

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

type Config struct {
	DBType      string
	DatabaseURL string
}

const defaultConfigPath = "app.conf"

func Load() *Config {
	cfg := &Config{DBType: "postgres"}
	parseFile(cfg, defaultConfigPath)
	applyEnvOverrides(cfg)
	return cfg
}

func parseFile(cfg *Config, path string) {
	f, err := os.Open(path)
	if err != nil {
		fmt.Printf("Warning: %s not found, using env/defaults\n", path)
		return
	}
	defer f.Close()

	section := ""
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section = strings.ToLower(strings.TrimSpace(line[1 : len(line)-1]))
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 || section != "database" {
			continue
		}
		switch strings.ToLower(strings.TrimSpace(parts[0])) {
		case "type":
			cfg.DBType = strings.ToLower(strings.TrimSpace(parts[1]))
		case "url":
			cfg.DatabaseURL = strings.TrimSpace(parts[1])
		}
	}
}

func applyEnvOverrides(cfg *Config) {
	if v := os.Getenv("DB_TYPE"); v != "" {
		cfg.DBType = strings.ToLower(v)
	}
	if v := os.Getenv("DATABASE_URL"); v != "" {
		cfg.DatabaseURL = v
	}
}

func (c *Config) Driver() string {
	if c.DBType == "sqlite" {
		return "sqlite3" // registrado por modernc en db.go
	}
	return "pgx"
}

func (c *Config) DSN() string {
	if c.DatabaseURL == "" && c.DBType == "sqlite" {
		return "file:///whatsapp.db"
	}
	return c.DatabaseURL
}
