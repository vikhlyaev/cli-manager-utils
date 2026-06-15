package compiler

import (
	"strings"

	"cli-manager/internal/models"
)

// Compiler — интерфейс генератора shell-кода. Помимо ZshCompiler есть
// BashCompiler; конкретный выбирается фабрикой For по настройке Config.Shell.
type Compiler interface {
	// Generate генерирует итоговый скрипт из конфигурации и сохраняет его
	// в соответствующее для оболочки место.
	Generate(cfg *models.Config) error
	// OutputPath возвращает путь, по которому сохраняется сгенерированный скрипт.
	OutputPath() string
	// RenderPrefix возвращает сгенерированный фрагмент для одного префикса
	// (для предпросмотра).
	RenderPrefix(p models.Prefix) string
}

// For возвращает компилятор для указанной оболочки. Неизвестное/пустое значение
// трактуется как zsh (поведение по умолчанию).
func For(shell string) Compiler {
	switch strings.ToLower(strings.TrimSpace(shell)) {
	case "bash":
		return BashCompiler{}
	default:
		return ZshCompiler{}
	}
}
