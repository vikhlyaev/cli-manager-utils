package compiler

import "cli-manager/internal/models"

// Compiler — интерфейс генератора shell-кода. Архитектурный задел: помимо
// ZshCompiler можно добавить BashCompiler, FishCompiler и т.д.
type Compiler interface {
	// Generate генерирует итоговый скрипт из конфигурации и сохраняет его
	// в соответствующее для оболочки место.
	Generate(cfg *models.Config) error
}
