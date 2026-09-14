package config

import (
	"fmt"
	"os"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	DNSControl DNSControlConfig `yaml:"dns_control"`
	Agent      AgentConfig      `yaml:"agent"`
	Edge       EdgeConfig       `yaml:"edge"`
}

type DNSControlConfig struct {
	URL   string `yaml:"url"`
	Token string `yaml:"token"`
}

type AgentConfig struct {
	ID       string         `yaml:"id"`
	Interval time.Duration  `yaml:"interval"`
	Capacity CapacityConfig `yaml:"capacity"`
}

type CapacityConfig struct {
	SNI   int `yaml:"sni"`
	Route int `yaml:"route"`
}

type EdgeConfig struct {
	Addresses []string `yaml:"addresses"`
}

func Load(filename string) (Config, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return Config{}, fmt.Errorf(
			"read config %q: %w",
			filename,
			err,
		)
	}

	var cfg Config

	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf(
			"parse config %q: %w",
			filename,
			err,
		)
	}

	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}

	return cfg, nil
}

func (c Config) Validate() error {
	c.DNSControl.URL = strings.TrimRight(
		strings.TrimSpace(c.DNSControl.URL),
		"/",
	)

	if c.DNSControl.URL == "" {
		return fmt.Errorf("dns_control.url is required")
	}

	if c.Agent.ID == "" {
		return fmt.Errorf("agent.id is required")
	}

	if c.Agent.Interval <= 0 {
		return fmt.Errorf("agent.interval must be greater than zero")
	}

	if len(c.Edge.Addresses) == 0 {
		return fmt.Errorf(
			"edge.addresses must contain at least one address",
		)
	}

	for i, address := range c.Edge.Addresses {
		address = strings.TrimSpace(address)

		if address == "" {
			return fmt.Errorf(
				"edge.addresses[%d] cannot be empty",
				i,
			)
		}
	}

	return nil
}
