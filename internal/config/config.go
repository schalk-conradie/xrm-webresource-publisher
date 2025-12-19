package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Environment represents a Dynamics 365 environment
type Environment struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

// Binding maps a local file to a web resource
type Binding struct {
	Environment      string `json:"environment"`
	LocalPath        string `json:"localPath"`
	WebResourceName  string `json:"webResourceName"`
	WebResourceID    string `json:"webResourceId"`
	LastKnownVersion string `json:"lastKnownVersion"`
	AutoPublish      bool   `json:"autoPublish"`
}

// Config represents the application configuration
type Config struct {
	CurrentEnvironment string        `json:"currentEnvironment"`
	Environments       []Environment `json:"environments"`
	PublisherPrefix    string        `json:"publisherPrefix"`
	Bindings           []Binding     `json:"bindings"`
}

var configDir string
var configPath string

func init() {
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	configDir = filepath.Join(home, ".d365tui")
	configPath = filepath.Join(configDir, "config.json")
}

// GetConfigDir returns the configuration directory path
func GetConfigDir() string {
	return configDir
}

// Load reads the config from disk or returns defaults
func Load() (*Config, error) {
	if err := os.MkdirAll(configDir, 0700); err != nil {
		return nil, err
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return &Config{
				CurrentEnvironment: "",
				Environments:       []Environment{},
				PublisherPrefix:    "new",
				Bindings:           []Binding{},
			}, nil
		}
		return nil, err
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return &Config{
			CurrentEnvironment: "",
			Environments:       []Environment{},
			PublisherPrefix:    "new",
			Bindings:           []Binding{},
		}, nil
	}

	return &cfg, nil
}

// Save writes the config to disk
func (c *Config) Save() error {
	if err := os.MkdirAll(configDir, 0700); err != nil {
		return err
	}

	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(configPath, data, 0600)
}

// ValidateEnvironmentURL checks if the URL is a valid Dynamics 365 URL
func ValidateEnvironmentURL(url string) error {
	if !strings.HasPrefix(url, "https://") {
		return errors.New("URL must start with https://")
	}

	pattern := `^https://[a-zA-Z0-9-]+\.crm[0-9]*\.dynamics\.com$`
	matched, err := regexp.MatchString(pattern, url)
	if err != nil {
		return err
	}
	if !matched {
		return errors.New("URL must be a valid Dynamics 365 URL (e.g., https://myorg.crm.dynamics.com)")
	}

	return nil
}

// AddEnvironment adds a new environment
func (c *Config) AddEnvironment(name, url string) error {
	name = strings.TrimSpace(name)
	url = strings.TrimSpace(url)

	if name == "" {
		return errors.New("environment name cannot be empty")
	}

	for _, env := range c.Environments {
		if env.Name == name {
			return errors.New("environment with this name already exists")
		}
	}

	if err := ValidateEnvironmentURL(url); err != nil {
		return err
	}

	c.Environments = append(c.Environments, Environment{Name: name, URL: url})
	return c.Save()
}

// UpdateEnvironment updates an existing environment
func (c *Config) UpdateEnvironment(oldName, newName, newURL string) error {
	newName = strings.TrimSpace(newName)
	newURL = strings.TrimSpace(newURL)

	if newName == "" {
		return errors.New("environment name cannot be empty")
	}

	if err := ValidateEnvironmentURL(newURL); err != nil {
		return err
	}

	if oldName != newName {
		for _, env := range c.Environments {
			if env.Name == newName {
				return errors.New("environment with this name already exists")
			}
		}
	}

	for i, env := range c.Environments {
		if env.Name == oldName {
			c.Environments[i].Name = newName
			c.Environments[i].URL = newURL

			if oldName != newName {
				for j, b := range c.Bindings {
					if b.Environment == oldName {
						c.Bindings[j].Environment = newName
					}
				}
				if c.CurrentEnvironment == oldName {
					c.CurrentEnvironment = newName
				}
			}

			return c.Save()
		}
	}

	return errors.New("environment not found")
}

// DeleteEnvironment removes an environment and its bindings
func (c *Config) DeleteEnvironment(name string) error {
	found := false
	newEnvs := make([]Environment, 0, len(c.Environments))
	for _, env := range c.Environments {
		if env.Name == name {
			found = true
			continue
		}
		newEnvs = append(newEnvs, env)
	}

	if !found {
		return errors.New("environment not found")
	}

	c.Environments = newEnvs

	newBindings := make([]Binding, 0, len(c.Bindings))
	for _, b := range c.Bindings {
		if b.Environment != name {
			newBindings = append(newBindings, b)
		}
	}
	c.Bindings = newBindings

	if c.CurrentEnvironment == name {
		c.CurrentEnvironment = ""
	}

	return c.Save()
}

// GetEnvironment returns the environment by name
func (c *Config) GetEnvironment(name string) *Environment {
	for i := range c.Environments {
		if c.Environments[i].Name == name {
			return &c.Environments[i]
		}
	}
	return nil
}

// GetBindingsForEnvironment returns bindings for a specific environment
func (c *Config) GetBindingsForEnvironment(envName string) []Binding {
	var result []Binding
	for _, b := range c.Bindings {
		if b.Environment == envName {
			result = append(result, b)
		}
	}
	return result
}

// AddBinding adds or updates a binding
func (c *Config) AddBinding(binding Binding) error {
	for i, b := range c.Bindings {
		if b.Environment == binding.Environment && b.WebResourceID == binding.WebResourceID {
			c.Bindings[i] = binding
			return c.Save()
		}
	}
	c.Bindings = append(c.Bindings, binding)
	return c.Save()
}

// GetBinding finds a binding by environment and web resource ID
func (c *Config) GetBinding(envName, webResourceID string) *Binding {
	for i := range c.Bindings {
		if c.Bindings[i].Environment == envName && c.Bindings[i].WebResourceID == webResourceID {
			return &c.Bindings[i]
		}
	}
	return nil
}

// UpdateBindingVersion updates the version of a binding
func (c *Config) UpdateBindingVersion(envName, webResourceID, version string) error {
	for i := range c.Bindings {
		if c.Bindings[i].Environment == envName && c.Bindings[i].WebResourceID == webResourceID {
			c.Bindings[i].LastKnownVersion = version
			return c.Save()
		}
	}
	return errors.New("binding not found")
}
