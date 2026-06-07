package storage

import (
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"

	"cli-manager/internal/models"
)

func TestRoundTrip(t *testing.T) {
	// Подменяем HOME на временную директорию, чтобы не трогать реальный конфиг.
	t.Setenv("HOME", t.TempDir())

	want := &models.Config{
		Variables: map[string]string{"FOO": "bar"},
		Prefixes: []models.Prefix{{
			Name:        "av",
			Description: "demo",
			Commands: []models.Command{{
				Name: "note", Run: "echo hi", AcceptsArgs: true,
				Flags: []models.Flag{{Name: "--x", Run: "echo x"}},
			}},
		}},
	}

	if err := SaveConfig(want); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}

	got, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}

	a, _ := yaml.Marshal(want)
	b, _ := yaml.Marshal(got)
	if string(a) != string(b) {
		t.Errorf("round-trip mismatch:\nwant:\n%s\ngot:\n%s", a, b)
	}
}

func TestLoadMissingReturnsEmpty(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if cfg.Variables == nil {
		t.Error("Variables должна быть инициализирована, а не nil")
	}
	if len(cfg.Prefixes) != 0 {
		t.Errorf("ожидался пустой список префиксов, получено: %d", len(cfg.Prefixes))
	}
}

func TestConfigPath(t *testing.T) {
	t.Setenv("HOME", "/tmp/home")
	if ConfigPath() != filepath.Join("/tmp/home", ".config", "cli-manager", "config.yaml") {
		t.Errorf("неверный путь: %s", ConfigPath())
	}
}
