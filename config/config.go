package config

import (
	"gopkg.in/yaml.v2"
	"io/ioutil"
	"os"
	"path/filepath"
)

// Bot represents telegram bot parameters.
type Bot struct {
	Token  string `yaml:"token"`
	User   string `yaml:"user"`
	ChatID int64  `yaml:"chat_id"`
}

type Sqlite struct {
	Datasource string `yaml:"datasource"`
}

// Config represents parent config group.
type Config struct {
	Bot    Bot    `yaml:"bot"`
	Sqlite Sqlite `yaml:"sqlite"`
}

// GetConfig returns config.
func GetConfig(cfgPath string) (*Config, error) {
	filename, err := filepath.Abs(cfgPath)
	if err != nil {
		return nil, err
	}
	yamlFile, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil, err
	}
	var config Config
	if err = yaml.Unmarshal(yamlFile, &config); err != nil {
		return nil, err
	}
	if config.Bot.Token == "" {
		config.Bot.Token = os.Getenv("BOT_TOKEN")
	}
	return &config, nil
}
