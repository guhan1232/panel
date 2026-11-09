// Package config 面板配置：默认自动生成 config.yaml（含随机安全入口）。
package config

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"math/big"
	"net"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// 端口随机化范围：面板默认监听端口在此区间内随机挑选。
const (
	PortMin = 10000
	PortMax = 30000
)

// randomFreePort 在 [PortMin, PortMax] 中随机挑选一个当前空闲的 TCP 端口。
func randomFreePort() (int, error) {
	span := big.NewInt(int64(PortMax - PortMin + 1))
	for i := 0; i < 50; i++ {
		n, err := rand.Int(rand.Reader, span)
		if err != nil {
			return 0, err
		}
		port := PortMin + int(n.Int64())
		l, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
		if err == nil {
			_ = l.Close()
			return port, nil
		}
	}
	return 0, fmt.Errorf("%d-%d 范围内未找到空闲端口", PortMin, PortMax)
}

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
	Path     string       `yaml:"-"` // 配置文件路径（Load 时记录，面板备份用）
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
	listenWasEmpty := cfg.Server.Listen == ""
	applyDefaults(cfg)
	// listen 原为空时把生成的随机端口持久化，避免每次重启端口漂移
	if listenWasEmpty && cfg.Server.Listen != "" {
		if err := cfg.Save(path); err != nil {
			return nil, err
		}
	}
	cfg.Path = path
	return cfg, nil
}

func defaultConfig() *Config {
	// 面板端口在 [10000, 30000] 中随机开放；探测失败兜底 24443
	port, err := randomFreePort()
	if err != nil {
		port = 24443
	}
	return &Config{
		Server: ServerConfig{
			Listen:   fmt.Sprintf(":%d", port),
			Entrance: randomToken(4), // 8 位随机安全入口
		},
		DataDir:  "data",
		LogDir:   "logs",
		LogLevel: "info",
	}
}

func applyDefaults(cfg *Config) {
	if cfg.Server.Listen == "" {
		// 未指定端口时在随机区间内挑选（每次生成可能不同，建议固定端口时显式配置）
		if port, err := randomFreePort(); err == nil {
			cfg.Server.Listen = fmt.Sprintf(":%d", port)
		} else {
			cfg.Server.Listen = ":24443"
		}
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
