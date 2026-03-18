package main

import (
	"fmt"
	"os"
	"path/filepath"
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
}

type ConfigFileLocations struct {
	GraphjinConfigFilePath string // stored at /config/dev.yaml
	QueriesFolderPath      string // stored at /queries
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

	return &Config{
		ConfigFileLocations: ConfigFileLocations{
			GraphjinConfigFilePath: graphjinConfigPath,
			QueriesFolderPath:      queriesFolderPath,
		},
	}, nil
}
