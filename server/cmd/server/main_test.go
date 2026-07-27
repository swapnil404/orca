package main

import (
	"log/slog"
	"testing"
)

func TestLoadConfig(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://localhost/orca")
	t.Setenv("ORCA_PORT", "9090")
	t.Setenv("ORCA_SERVER_URL", "wss://orca.example/agent")
	t.Setenv("ORCA_LOG_LEVEL", "debug")

	configuration, err := loadConfig()
	if err != nil {
		t.Fatalf("loadConfig() error = %v", err)
	}
	if configuration.databaseURL != "postgres://localhost/orca" {
		t.Errorf("databaseURL = %q", configuration.databaseURL)
	}
	if configuration.port != 9090 {
		t.Errorf("port = %d", configuration.port)
	}
	if configuration.serverURL != "wss://orca.example/agent" {
		t.Errorf("serverURL = %q", configuration.serverURL)
	}
	if configuration.logLevel != slog.LevelDebug {
		t.Errorf("logLevel = %v", configuration.logLevel)
	}
}

func TestLoadConfigRejectsInvalidPort(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://localhost/orca")
	t.Setenv("ORCA_PORT", "not-a-port")

	if _, err := loadConfig(); err == nil {
		t.Fatal("loadConfig() error = nil, want invalid port error")
	}
}
