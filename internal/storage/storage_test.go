package storage

import (
	"os"
	"path/filepath"
	"strings"
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

func TestSaveConfigBackup(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	c1 := &models.Config{Variables: map[string]string{}, Prefixes: []models.Prefix{{Name: "one"}}}
	if err := SaveConfig(c1); err != nil {
		t.Fatalf("SaveConfig c1: %v", err)
	}
	// При первом сохранении исходного файла не было — бэкап не создаётся.
	if _, err := os.Stat(ConfigPath() + ".bak"); err == nil {
		t.Error("при первом сохранении .bak не должен создаваться")
	}

	c2 := &models.Config{Variables: map[string]string{}, Prefixes: []models.Prefix{{Name: "two"}}}
	if err := SaveConfig(c2); err != nil {
		t.Fatalf("SaveConfig c2: %v", err)
	}
	bak, err := os.ReadFile(ConfigPath() + ".bak")
	if err != nil {
		t.Fatalf("ожидался .bak после второго сохранения: %v", err)
	}
	if !strings.Contains(string(bak), "one") {
		t.Errorf(".bak должен хранить предыдущую версию (one), получено:\n%s", bak)
	}
}

func TestCloneConfigIsDeep(t *testing.T) {
	orig := &models.Config{
		Variables: map[string]string{"A": "1"},
		Prefixes:  []models.Prefix{{Name: "p", Commands: []models.Command{{Name: "c"}}}},
	}
	clone, err := CloneConfig(orig)
	if err != nil {
		t.Fatalf("CloneConfig: %v", err)
	}
	clone.Prefixes[0].Name = "changed"
	clone.Variables["A"] = "2"
	if orig.Prefixes[0].Name != "p" {
		t.Error("клон делит срез Prefixes с оригиналом")
	}
	if orig.Variables["A"] != "1" {
		t.Error("клон делит карту Variables с оригиналом")
	}
}

func TestConfigPath(t *testing.T) {
	t.Setenv("HOME", "/tmp/home")
	if ConfigPath() != filepath.Join("/tmp/home", ".config", "cli-manager", "config.yaml") {
		t.Errorf("неверный путь: %s", ConfigPath())
	}
}
