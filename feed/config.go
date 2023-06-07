package feed

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	ShowDescriptions bool          `yaml:"showDescriptions"`
	MaxBadgeSize     int           `yaml:"maxBadgeSize"`
	Sources          []Source      `yaml:"sources"`
	Subreddits       []string      `yaml:"subreddits"`
	PollInterval     time.Duration `yaml:"pollInterval"`
	ShowHelp         bool          `yaml:"showHelp"`
}

var DefaultConfig = Config{
	ShowDescriptions: true,
	MaxBadgeSize:     16,
	ShowHelp:         true,
	PollInterval:     time.Minute,
	Sources: []Source{
		{
			Name:       "BBC",
			Background: "#930000",
			Foreground: "#e4e6e9",
			Url:        "http://feeds.bbci.co.uk/news/world/rss.xml",
			MaxAge:     4 * time.Hour,
		},
		{
			Name:       "Hacker News",
			Background: "#cc5200",
			Foreground: "#e4e6e9",
			Url:        "https://hnrss.org/newest?points=20",
		},
		{
			Name:       "lobste.rs",
			Background: "#5e0000",
			Foreground: "#ffffff",
			Url:        "https://lobste.rs/rss",
		},
		{
			Name:       "Register",
			Url:        "https://www.theregister.com/security/headlines.atom",
			Background: "#ff581a",
			Foreground: "#ffffff",
		},
	},
	Subreddits: []string{
		"programming",
		"linux",
	},
}

func LoadConfig(fromFile bool) (*Config, error) {

	if fromFile {
		homedir, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("failed to get home directory: %w", err)
		}

		configPath := filepath.Join(homedir, ".config", "happen.yaml")

		if data, err := os.ReadFile(configPath); err == nil {
			config := DefaultConfig
			err := yaml.Unmarshal(data, &config)
			if err != nil {
				return nil, fmt.Errorf("failed to unmarshal config: %w", err)
			}
			config.Init()
			return &config, nil
		}

		if err := os.MkdirAll(filepath.Dir(configPath), 0700); err != nil {
			return nil, fmt.Errorf("failed to create config directory: %w", err)
		}

		data, err := yaml.Marshal(DefaultConfig)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal default config: %w", err)
		}

		if err := os.WriteFile(configPath, data, 0600); err != nil {
			return nil, fmt.Errorf("failed to write default config: %w", err)
		}
	}

	config := DefaultConfig
	config.Init()

	return &config, nil
}

func (c *Config) Init() {
	for _, subreddit := range c.Subreddits {
		c.Sources = append(c.Sources, Source{
			Name:       "r/" + subreddit,
			Background: "#ff581a",
			Foreground: "#e4e6e9",
			Url:        fmt.Sprintf("https://www.reddit.com/r/%s/.rss", subreddit),
			MaxAge:     24 * time.Hour,
		})
	}
}
