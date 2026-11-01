package storage

import (
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"cli-manager/internal/fsutil"
	"cli-manager/internal/models"
)

// ConfigPath возвращает путь к файлу конфигурации (~/.config/cli-manager/config.yaml).
func ConfigPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "cli-manager", "config.yaml")
}

// LoadConfig читает конфигурацию с диска. Если файла нет — возвращает пустой,
// но валидный Config (с инициализированной картой переменных).
func LoadConfig() (*models.Config, error) {
	path := ConfigPath()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &models.Config{Variables: map[string]string{}}, nil
		}
		return nil, err
	}

	var cfg models.Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	if cfg.Variables == nil {
		cfg.Variables = map[string]string{}
	}
	return &cfg, nil
}

// SaveConfig сериализует конфигурацию в YAML и атомарно записывает на диск,
// создавая директорию при необходимости. Перед перезаписью предыдущая версия
// файла копируется в config.yaml.bak — простой одноуровневый бэкап.
func SaveConfig(cfg *models.Config) error {
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}

	path := ConfigPath()
	// Бэкап текущего файла перед перезаписью (если он есть). Ошибка бэкапа не
	// мешает сохранению: основной файл всё равно пишется атомарно.
	if existing, err := os.ReadFile(path); err == nil {
		_ = fsutil.WriteFileAtomic(path+".bak", existing, 0o644, nil)
	}

	return fsutil.WriteFileAtomic(path, data, 0o644, nil)
}

// CloneConfig возвращает глубокую копию конфигурации (через YAML round-trip).
// Используется для снимков в undo-стеке: снимок не должен делить срезы/карты с
// рабочей копией.
func CloneConfig(cfg *models.Config) (*models.Config, error) {
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return nil, err
	}
	var c models.Config
	if err := yaml.Unmarshal(data, &c); err != nil {
		return nil, err
	}
	if c.Variables == nil {
		c.Variables = map[string]string{}
	}
	return &c, nil
}
