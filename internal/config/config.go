package config

import (
	"fmt"

	"github.com/joho/godotenv"
	"github.com/kelseyhightower/envconfig"
)

type Config struct {
	AppConfig     *AppConfig
	AIConfig      *AIConfig
	BrowserConfig *BrowserConfig
}

type AppConfig struct {
	LogLevel string `envconfig:"LOG_LEVEL" default:"info"`
	Debug    bool   `envconfig:"DEBUG" default:"false"`
}

type AIConfig struct {
	Provider string `envconfig:"AI_PROVIDER" default:"anthropic"`
	APIKey   string `envconfig:"AI_API_KEY" required:"true"`
	Model    string `envconfig:"AI_MODEL" default:"claude-sonnet-4-20250514"`
}

type BrowserConfig struct {
	Headless       bool   `envconfig:"BROWSER_HEADLESS" default:"false"`
	SlowMo         int    `envconfig:"BROWSER_SLOW_MO" default:"100"`
	Timeout        int    `envconfig:"BROWSER_TIMEOUT" default:"30000"`
	UserDataDir    string `envconfig:"BROWSER_USER_DATA_DIR" default:"./browser-data"`
	UseScreenshots bool   `envconfig:"BROWSER_USE_SCREENSHOTS" default:"true"`
}

func GetConfig() (*Config, error) {
	_ = godotenv.Load()

	var conf Config

	if err := envconfig.Process("", &conf); err != nil {
		return nil, fmt.Errorf("read config from env vars: %w", err)
	}

	return &conf, nil
}
