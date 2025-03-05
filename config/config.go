package config

import (
	"os"
	"time"

	"github.com/sirupsen/logrus"
	"gopkg.in/yaml.v3"
)

// CollectorConfig is the base configuration for all collectors
type CollectorConfig struct {
	ScrapeTime time.Duration `yaml:"scrapeTime"`
}

// IsEnabled returns if the collector is enabled
func (c *CollectorConfig) IsEnabled() bool {
	return c.ScrapeTime.Seconds() > 0
}

// Config is the root configuration
type Config struct {
	Logger *logrus.Logger

	Azure struct {
		// List of tenant IDs
		Tenants []string `yaml:"tenants"`
	} `yaml:"azure"`

	Collector struct {
		General                  CollectorConfig `yaml:"general"`
		Users                    CollectorConfig `yaml:"users"`
		Devices                  CollectorConfig `yaml:"devices"`
		Applications             CollectorConfig `yaml:"applications"`
		ServicePrincipals        CollectorConfig `yaml:"servicePrincipals"`
		Groups                   CollectorConfig `yaml:"groups"`
		ConditionalAccessPolicies CollectorConfig `yaml:"conditionalAccessPolicies"`
		DirectoryRoles           CollectorConfig `yaml:"directoryRoles"`
	} `yaml:"collectors"`
}

// NewConfig creates a new configuration
func NewConfig(logger *logrus.Logger) *Config {
	return &Config{
		Logger: logger,
	}
}

// LoadConfigFile loads the config file
func (c *Config) LoadConfigFile(path string) error {
	c.Logger.Infof("loading configuration from %s", path)

	ymlBytes, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	return yaml.Unmarshal(ymlBytes, c)
}
