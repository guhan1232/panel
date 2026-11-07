// Package config 面板配置：默认自动生成 config.yaml（含随机安全入口）。
package config

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// TLSConfig TLS 配置；cert/key 为空时自动生成自签证书。
type TLSConfig struct {
	Enabled bool   `yaml:"enabled"`
	Cert    string `yaml:"cert"`
	Key     string `yaml:"key"`
}

type ServerConfig struct {
	Listen   string    `yaml:"listen"`
	Entrance string    `yaml:"entrance"`
	TLS      TLSConfig `yaml:"tls"`
}

type Config struct {
	Server   ServerConfig `yaml:"server"`
	DataDir  string       `yaml:"data_dir"`
	LogDir   string       `yaml:"log_dir"`
	LogLevel string       `yaml:"log_level"`
}

func randomToken(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// Load 读取配置；文件不存在时生成默认配置并保存，返回的 cfg 同时带兜底默认值。
func Load(path string) (*Config, error) {
	cfg := &Config{}
	if data, err := os.ReadFile(path); err == nil {
		if err := yaml.Unmarshal(data, cfg); err != nil {
			return nil, fmt.Errorf("解析配置 %s: %w", path, err)
		}
	} else if !os.IsNotExist(err) {
		return nil, err
	} else {
		cfg = defaultConfig()
		if err := cfg.Save(path); err != nil {
			return nil, err
		}
	}
	applyDefaults(cfg)
	return cfg, nil
}

func defaultConfig() *Config {
	return &Config{
		Server: ServerConfig{
			Listen:   ":8443",
			Entrance: randomToken(4), // 8 位随机安全入口
		},
		DataDir:  "data",
		LogDir:   "logs",
		LogLevel: "info",
	}
}

func applyDefaults(cfg *Config) {
	if cfg.Server.Listen == "" {
		cfg.Server.Listen = ":8443"
	}
	if cfg.DataDir == "" {
		cfg.DataDir = "data"
	}
	if cfg.LogDir == "" {
		cfg.LogDir = "logs"
	}
	if cfg.LogLevel == "" {
		cfg.LogLevel = "info"
	}
}

func (c *Config) Save(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := yaml.Marshal(c)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}
