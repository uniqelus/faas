package pkgconfig

import (
	"fmt"

	"github.com/ilyakaznacheev/cleanenv"
)

func ReadConfig[Config any](path string) (*Config, error) {
	if path != "" {
		return ReadFromFile[Config](path)
	}

	return ReadFromEnv[Config]()
}

func ReadFromFile[Config any](path string) (*Config, error) {
	var cfg Config
	if err := cleanenv.ReadConfig(path, &cfg); err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	return &cfg, nil
}

func ReadFromEnv[Config any]() (*Config, error) {
	var cfg Config
	if err := cleanenv.ReadEnv(&cfg); err != nil {
		return nil, fmt.Errorf("failed to read config from environment: %w", err)
	}

	return &cfg, nil
}
