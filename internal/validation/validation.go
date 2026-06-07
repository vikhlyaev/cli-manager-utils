package validation

import (
	"os/exec"
	"strings"
)

// CheckCollision возвращает true, если в системе уже существует команда с таким
// именем (через `command -v <name>`). Это сигнал о возможном перекрытии
// существующей утилиты.
func CheckCollision(name string) bool {
	name = strings.TrimSpace(name)
	if name == "" {
		return false
	}
	// `command` — встроенная команда оболочки, поэтому запускаем через bash.
	// Имя передаём отдельным аргументом, чтобы избежать инъекций.
	cmd := exec.Command("bash", "-c", `command -v -- "$1" >/dev/null 2>&1`, "bash", name)
	return cmd.Run() == nil
}

// ValidateBash выполняет переданный bash-код и возвращает объединённый вывод
// (stdout + stderr). Используется для UX-тестирования действий (Ctrl+T).
// Если код завершился с ненулевым статусом, возвращается ошибка вместе с
// уже собранным выводом.
func ValidateBash(code string) (string, error) {
	code = strings.TrimSpace(code)
	if code == "" {
		return "", nil
	}
	cmd := exec.Command("bash", "-c", code)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// CheckSyntax выполняет статическую проверку синтаксиса через `bash -n`,
// не запуская код. Возвращает текст ошибки парсера, если он есть.
func CheckSyntax(code string) error {
	code = strings.TrimSpace(code)
	if code == "" {
		return nil
	}
	cmd := exec.Command("bash", "-n", "-c", code)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return &SyntaxError{Output: strings.TrimSpace(string(out))}
	}
	return nil
}

// SyntaxError содержит вывод парсера bash при ошибке синтаксиса.
type SyntaxError struct {
	Output string
}

func (e *SyntaxError) Error() string {
	if e.Output == "" {
		return "синтаксическая ошибка bash"
	}
	return e.Output
}
