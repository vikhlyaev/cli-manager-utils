package cli

import (
	"bufio"
	"fmt"
	"os"
	"runtime"
	"strings"

	"cli-manager/internal/compiler"
	"cli-manager/internal/importer"
	"cli-manager/internal/shellrc"
	"cli-manager/internal/storage"
)

// runInit подключает сгенерированный скрипт к rc-файлу выбранной оболочки.
//
//	init          показать изменения и спросить подтверждение
//	init --yes    без вопроса
//	init --print  только напечатать блок (для ручного подключения)
func runInit(args []string, s IO) int {
	var yes, printOnly bool
	for _, a := range args {
		switch a {
		case "-y", "--yes":
			yes = true
		case "--print":
			printOnly = true
		default:
			fmt.Fprintf(s.Err, "init: неизвестный флаг %q (есть --yes, --print)\n", a)
			return 2
		}
	}

	cfg, err := storage.LoadConfig()
	if err != nil {
		fmt.Fprintln(s.Err, "Ошибка загрузки конфигурации:", err)
		return 1
	}
	c := compiler.For(cfg.Shell)
	script := c.OutputPath()
	block := shellrc.Block(script)

	if printOnly {
		fmt.Fprint(s.Out, block)
		return 0
	}

	rc := importer.RCPath(shellOf(cfg.Shell))
	if ok, err := shellrc.IsInstalled(rc, script); err != nil {
		fmt.Fprintln(s.Err, "Не удалось прочитать", rc+":", err)
		return 1
	} else if ok {
		fmt.Fprintln(s.Out, "Скрипт уже подключён в", rc)
		return 0
	}

	if !yes {
		fmt.Fprintf(s.Out, "В конец %s будет добавлено:\n\n%s\nПродолжить? [y/N] ", rc, block)
		answer, _ := bufio.NewReader(s.In).ReadString('\n')
		if a := strings.ToLower(strings.TrimSpace(answer)); a != "y" && a != "yes" && a != "д" && a != "да" {
			fmt.Fprintln(s.Out, "Отменено.")
			return 1
		}
	}

	_, statErr := os.Stat(rc)
	hadRC := statErr == nil
	if _, err := shellrc.Install(rc, script); err != nil {
		fmt.Fprintln(s.Err, "Не удалось изменить", rc+":", err)
		return 1
	}
	// Сразу создаём скрипт, чтобы новый терминал не ссылался на пустоту.
	if err := c.Generate(cfg); err != nil {
		fmt.Fprintln(s.Err, "Предупреждение: не удалось сгенерировать скрипт:", err)
	}
	if hadRC {
		fmt.Fprintf(s.Out, "Готово: скрипт подключён в %s (прежняя версия — %s.bak).\n", rc, rc)
	} else {
		fmt.Fprintf(s.Out, "Готово: создан %s с подключением скрипта.\n", rc)
	}
	if shellOf(cfg.Shell) == "bash" && runtime.GOOS == "darwin" {
		fmt.Fprintf(s.Out, "На macOS терминал читает ~/.bash_profile — убедитесь, что в нём есть: %s\n", bashrcSourceLine)
	}
	fmt.Fprintf(s.Out, "Откройте новый терминал или выполните: source %s\n", rc)
	return 0
}

// shellOf нормализует имя оболочки из конфига ("zsh" по умолчанию).
func shellOf(shell string) string {
	if strings.ToLower(strings.TrimSpace(shell)) == "bash" {
		return "bash"
	}
	return "zsh"
}
