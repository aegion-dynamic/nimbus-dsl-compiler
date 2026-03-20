package main

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type Config struct {
	PostgresURL         string              `env:"POSTGRES_URL"`
	PostgresUser        string              `env:"POSTGRES_USER"`
	PostgresPassword    string              `env:"POSTGRES_PASSWORD"`
	PostgresDB          string              `env:"POSTGRES_DB"`
	PostgresPort        string              `env:"POSTGRES_PORT"`
	PostgresHost        string              `env:"POSTGRES_HOST"`
	PostgresSSLMode     string              `env:"POSTGRES_SSL_MODE"`
	ConfigFileLocations ConfigFileLocations `env:"CONFIG_FILE_LOCATIONS"`

	// graphjinDev is sourced from config/dev.yaml and used for schema validation.
	graphjinDev graphjinDevConfig
}

type ConfigFileLocations struct {
	GraphjinConfigFilePath string // stored at /config/dev.yaml
	QueriesFolderPath      string // stored at /queries
}

type graphjinDevConfig struct {
	EnableCamelcase bool             `yaml:"enable_camelcase"`
	Production      bool             `yaml:"production"`
	SecretKey       string           `yaml:"secret_key"`
	Database        graphjinDatabase `yaml:"database"`
}

type graphjinDatabase struct {
	Type       string `yaml:"type"`
	Host       string `yaml:"host"`
	Port       int    `yaml:"port"`
	DBName     string `yaml:"dbname"`
	User       string `yaml:"user"`
	Password   string `yaml:"password"`
	Schema     string `yaml:"schema"`
	EnableTLS  bool   `yaml:"enable_tls"`
	ServerName string `yaml:"server_name"`
	ConnString string `yaml:"connection_string"`
}

func LoadConfig(configFolderPath string) (*Config, error) {
	configFolderPath = filepath.Clean(configFolderPath)

	graphjinConfigPath := filepath.Join(configFolderPath, "dev.yaml")
	queriesFolderPath := filepath.Join(configFolderPath, "queries")

	st, err := os.Stat(graphjinConfigPath)
	if err != nil {
		return nil, fmt.Errorf("missing GraphJin config file %q: %w", graphjinConfigPath, err)
	}
	if st.IsDir() {
		return nil, fmt.Errorf("GraphJin config path %q is a directory; expected a file", graphjinConfigPath)
	}

	st, err = os.Stat(queriesFolderPath)
	if err != nil {
		return nil, fmt.Errorf("missing queries folder %q: %w", queriesFolderPath, err)
	}
	if !st.IsDir() {
		return nil, fmt.Errorf("queries folder %q is not a directory", queriesFolderPath)
	}

	// Parse config/dev.yaml so the validator can initialize GraphJin and connect to Postgres.
	devBytes, err := os.ReadFile(graphjinConfigPath)
	if err != nil {
		return nil, fmt.Errorf("reading GraphJin dev.yaml at %q: %w", graphjinConfigPath, err)
	}

	var dev graphjinDevConfig
	if err := yaml.Unmarshal(devBytes, &dev); err != nil {
		return nil, fmt.Errorf("parsing GraphJin dev.yaml at %q: %w", graphjinConfigPath, err)
	}

	return &Config{
		ConfigFileLocations: ConfigFileLocations{
			GraphjinConfigFilePath: graphjinConfigPath,
			QueriesFolderPath:      queriesFolderPath,
		},
		graphjinDev: dev,
	}, nil
}
