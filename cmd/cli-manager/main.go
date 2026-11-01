package main

import (
	"fmt"
	"os"

	"cli-manager/internal/compiler"
	"cli-manager/internal/storage"
	"cli-manager/internal/tui"
	"cli-manager/internal/validation"
)

func main() {
	cfg, err := storage.LoadConfig()
	if err != nil {
		fmt.Fprintln(os.Stderr, "Ошибка загрузки конфигурации:", err)
		os.Exit(1)
	}

	for _, p := range validation.ValidateConfig(cfg) {
		fmt.Fprintln(os.Stderr, "Предупреждение: конфиг:", p)
	}

	// Регенерируем скрипт при каждом запуске, чтобы улучшения компилятора
	// (новые возможности генерации) попадали в generated.zsh даже без правок
	// конфигурации. Ошибку не считаем фатальной — TUI всё равно запускаем.
	if err := compiler.For(cfg.Shell).Generate(cfg); err != nil {
		fmt.Fprintln(os.Stderr, "Предупреждение: не удалось перегенерировать скрипт:", err)
	}

	tui.Run(cfg)
}
