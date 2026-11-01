package compiler

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"

	"cli-manager/internal/fsutil"
)

// writeScript атомарно записывает сгенерированный скрипт, предварительно
// проверив его синтаксис целевой оболочкой (`<shell> -n`). Если скрипт битый,
// прежний файл остаётся на месте: одна ошибочная команда не должна ломать
// загрузку всей оболочки через `source`.
func writeScript(path, shell, script string) error {
	return fsutil.WriteFileAtomic(path, []byte(script), 0o644, func(tmp string) error {
		if err := checkScript(shell, tmp); err != nil {
			// В выводе парсера — имя временного файла; показываем настоящее.
			return fmt.Errorf("%s", strings.ReplaceAll(err.Error(), tmp, filepath.Base(path)))
		}
		return nil
	})
}

// checkScript прогоняет файл через `<shell> -n`. Если оболочки нет в системе,
// проверять нечем — запись не блокируем.
func checkScript(shell, path string) error {
	if _, err := exec.LookPath(shell); err != nil {
		return nil
	}
	out, err := exec.Command(shell, "-n", path).CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if msg == "" {
			msg = err.Error()
		}
		return fmt.Errorf("скрипт не прошёл проверку %s -n, прежний файл сохранён: %s", shell, msg)
	}
	return nil
}
