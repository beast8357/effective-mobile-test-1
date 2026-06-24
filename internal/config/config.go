package config

import (
	"fmt"
	"time"

	"github.com/ilyakaznacheev/cleanenv"
)

type Config struct {
	HTTP     HTTPConfig     `yaml:"http"`
	Database DatabaseConfig `yaml:"database"`
	Log      LogConfig      `yaml:"log"`
}

type HTTPConfig struct {
	Port         string        `yaml:"port"          env:"HTTP_PORT"          env-default:"8080"`
	ReadTimeout  time.Duration `yaml:"read_timeout"  env:"HTTP_READ_TIMEOUT"  env-default:"10s"`
	WriteTimeout time.Duration `yaml:"write_timeout" env:"HTTP_WRITE_TIMEOUT" env-default:"10s"`
}

type DatabaseConfig struct {
	Host     string `yaml:"host"     env:"DB_HOST"     env-default:"localhost"`
	Port     int    `yaml:"port"     env:"DB_PORT"     env-default:"5432"`
	User     string `yaml:"user"     env:"DB_USER"     env-default:"postgres"`
	Password string `yaml:"password" env:"DB_PASSWORD" env-default:"postgres"`
	Name     string `yaml:"name"     env:"DB_NAME"     env-default:"subscriptions"`
	SSLMode  string `yaml:"sslmode"  env:"DB_SSLMODE"  env-default:"disable"`
}

type LogConfig struct {
	Level string `yaml:"level" env:"LOG_LEVEL" env-default:"info"`
}

func (c DatabaseConfig) DSN() string {
	return fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=%s",
		c.User, c.Password, c.Host, c.Port, c.Name, c.SSLMode)
}

// Load reads configuration from the YAML file (if present) and overlays
// any values set via environment variables.
func Load(path string) (*Config, error) {
	var cfg Config
	if path != "" {
		if err := cleanenv.ReadConfig(path, &cfg); err == nil {
			return &cfg, nil
		}
	}
	if err := cleanenv.ReadEnv(&cfg); err != nil {
		return nil, fmt.Errorf("read env: %w", err)
	}
	return &cfg, nil
}
