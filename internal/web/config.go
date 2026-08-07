package web

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"
)

const (
	DefaultHost       = "127.0.0.1"
	DefaultPort       = 9615
	DefaultDaemonPort = 50051
)

type Config struct {
	Host          string
	Port          int
	Token         string
	AllowRemote   bool
	ReadOnly      bool
	SecureCookies bool
	AssetDir      string
	DevReload     bool
	SessionTTL    time.Duration
}

type runtimeConfig struct {
	Config
	tokenGenerated bool
}

func normalizeConfig(config Config) (runtimeConfig, error) {
	if strings.TrimSpace(config.Host) == "" {
		config.Host = DefaultHost
	}
	if config.Port <= 0 || config.Port > 65535 {
		return runtimeConfig{}, fmt.Errorf("invalid web port: %d", config.Port)
	}
	if !config.AllowRemote && !isLoopbackHost(config.Host) {
		return runtimeConfig{}, errors.New("refusing to bind web server to a non-loopback host without --allow-remote")
	}
	if config.SessionTTL <= 0 {
		config.SessionTTL = 12 * time.Hour
	}

	runtime := runtimeConfig{Config: config}
	runtime.Token = strings.TrimSpace(config.Token)
	if runtime.Token == "" {
		token, err := generateToken()
		if err != nil {
			return runtimeConfig{}, err
		}
		runtime.Token = token
		runtime.tokenGenerated = true
	}
	return runtime, nil
}

func isLoopbackHost(host string) bool {
	trimmed := strings.Trim(host, "[]")
	if strings.EqualFold(trimmed, "localhost") {
		return true
	}
	ip := net.ParseIP(trimmed)
	return ip != nil && ip.IsLoopback()
}

func generateToken() (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}
