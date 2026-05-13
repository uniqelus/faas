package pkgconfig_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	pkgconfig "github.com/uniqelus/faas/pkg/config"
)

type TestConfig struct {
	Port     int    `yaml:"port" env:"PORT"`
	Host     string `yaml:"host" env:"HOST"`
	Database struct {
		Host     string `yaml:"host" env:"DATABASE_HOST"`
		Port     int    `yaml:"port" env:"DATABASE_PORT"`
		User     string `yaml:"user" env:"DATABASE_USER"`
		Password string `yaml:"password" env:"DATABASE_PASSWORD"`
		Database string `yaml:"database" env:"DATABASE_DATABASE"`
	} `yaml:"database"`
}

func TestReadFromFile(t *testing.T) {
	cfg, err := pkgconfig.ReadFromFile[TestConfig]("testdata/config.yaml")
	if err != nil {
		t.Fatalf("failed to read config file: %v", err)
	}

	assert.NoError(t, err)
	assert.Equal(t, cfg.Port, 8080)
	assert.Equal(t, cfg.Host, "localhost")
	assert.Equal(t, cfg.Database.Host, "localhost")
	assert.Equal(t, cfg.Database.Port, 5432)
	assert.Equal(t, cfg.Database.User, "postgres")
	assert.Equal(t, cfg.Database.Password, "postgres")
	assert.Equal(t, cfg.Database.Database, "postgres")
}

func TestReadFromEnv(t *testing.T) {
	t.Setenv("PORT", "8080")
	t.Setenv("HOST", "localhost")
	t.Setenv("DATABASE_HOST", "localhost")
	t.Setenv("DATABASE_PORT", "5432")
	t.Setenv("DATABASE_USER", "postgres")
	t.Setenv("DATABASE_PASSWORD", "postgres")
	t.Setenv("DATABASE_DATABASE", "postgres")

	cfg, err := pkgconfig.ReadFromEnv[TestConfig]()
	if err != nil {
		t.Fatalf("failed to read config from environment: %v", err)
	}

	assert.NoError(t, err)
	assert.Equal(t, cfg.Port, 8080)
	assert.Equal(t, cfg.Host, "localhost")
	assert.Equal(t, cfg.Database.Host, "localhost")
	assert.Equal(t, cfg.Database.Port, 5432)
	assert.Equal(t, cfg.Database.User, "postgres")
	assert.Equal(t, cfg.Database.Password, "postgres")
	assert.Equal(t, cfg.Database.Database, "postgres")
}
