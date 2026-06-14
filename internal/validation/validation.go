package validation

import (
	"fmt"
	"os/exec"
	"regexp"
	"strings"
)

// reIdentifier — допустимое имя shell-функции/переменной: буква или подчёркивание
// в начале, далее буквы, цифры, подчёркивания. Используется для имён префиксов
// (становятся именами zsh-функций) и глобальных переменных (export NAME=…).
var reIdentifier = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)

// reCommandName — имя команды (ветка case в диспетчере). Помимо символов
// идентификатора разрешаем дефис: типичные имена вроде `db-migrate`.
var reCommandName = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_-]*$`)

// reFlagName — имя флага. Допускаем ведущие дефисы (`--force`, `-f`), но требуем
// далее буквенно-цифровой символ.
var reFlagName = regexp.MustCompile(`^-{0,2}[a-zA-Z0-9][a-zA-Z0-9_-]*$`)

// ValidateIdentifier проверяет, что имя пригодно как имя shell-функции или
// переменной (префикс, глобальная переменная).
func ValidateIdentifier(name string) error {
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("имя не может быть пустым")
	}
	if !reIdentifier.MatchString(name) {
		return fmt.Errorf("недопустимое имя %q: разрешены буквы, цифры и _, первый символ — буква или _", name)
	}
	return nil
}

// ValidateCommandName проверяет имя команды (разрешён дефис).
func ValidateCommandName(name string) error {
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("имя не может быть пустым")
	}
	if !reCommandName.MatchString(name) {
		return fmt.Errorf("недопустимое имя команды %q: разрешены буквы, цифры, _ и -", name)
	}
	return nil
}

// ValidateFlagName проверяет имя флага (допускает ведущие дефисы).
func ValidateFlagName(name string) error {
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("имя не может быть пустым")
	}
	if !reFlagName.MatchString(name) {
		return fmt.Errorf("недопустимое имя флага %q: например --force, -f, build", name)
	}
	return nil
}

// CheckCondition проверяет синтаксис контекстного условия, оборачивая его в
// `if <cond>; then :; fi` и прогоняя через `bash -n`. Кривое условие иначе
// ломает справку и автодополнение всего префикса.
func CheckCondition(cond string) error {
	cond = strings.TrimSpace(cond)
	if cond == "" {
		return nil
	}
	return CheckSyntax("if " + cond + "; then :; fi")
}

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
