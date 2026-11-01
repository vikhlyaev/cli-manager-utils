// Package cli реализует неинтерактивные подкоманды cli-manager (generate, path,
// version, …). Без аргументов бинарь открывает TUI — это решает main.
package cli

import (
	"fmt"
	"io"

	"cli-manager/internal/buildinfo"
	"cli-manager/internal/compiler"
	"cli-manager/internal/storage"
	"cli-manager/internal/validation"
)

// IO — потоки ввода-вывода подкоманды (подменяются в тестах).
type IO struct {
	In  io.Reader
	Out io.Writer
	Err io.Writer
}

// command — одна подкоманда: имя, краткое описание и обработчик, который
// возвращает код выхода.
type command struct {
	name    string
	aliases []string
	summary string
	run     func(args []string, s IO) int
}

func commands() []command {
	return []command{
		{name: "generate", summary: "перегенерировать скрипт из конфига", run: runGenerate},
		{name: "path", summary: "напечатать путь к сгенерированному скрипту", run: runPath},
		{name: "version", aliases: []string{"--version", "-v"}, summary: "показать версию", run: runVersion},
		{name: "help", aliases: []string{"--help", "-h"}, summary: "эта справка", run: runHelp},
	}
}

// Run выполняет подкоманду args[0] и возвращает код выхода. Неизвестная
// подкоманда печатает справку в stderr и возвращает 2.
func Run(args []string, s IO) int {
	if len(args) == 0 {
		return runHelp(nil, s)
	}
	for _, c := range commands() {
		if args[0] == c.name || contains(c.aliases, args[0]) {
			return c.run(args[1:], s)
		}
	}
	fmt.Fprintf(s.Err, "cli-manager: неизвестная команда %q\n\n", args[0])
	printHelp(s.Err)
	return 2
}

func contains(xs []string, x string) bool {
	for _, v := range xs {
		if v == x {
			return true
		}
	}
	return false
}

func runHelp(_ []string, s IO) int {
	printHelp(s.Out)
	return 0
}

func printHelp(w io.Writer) {
	fmt.Fprintln(w, "cli-manager — конструктор составных shell-команд")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Использование:")
	fmt.Fprintln(w, "  cli-manager            открыть интерактивный интерфейс")
	fmt.Fprintln(w, "  cli-manager <команда>")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Команды:")
	for _, c := range commands() {
		fmt.Fprintf(w, "  %-10s %s\n", c.name, c.summary)
	}
}

func runVersion(_ []string, s IO) int {
	fmt.Fprintln(s.Out, "cli-manager", buildinfo.Version)
	return 0
}

func runPath(_ []string, s IO) int {
	cfg, err := storage.LoadConfig()
	if err != nil {
		fmt.Fprintln(s.Err, "Ошибка загрузки конфигурации:", err)
		return 1
	}
	fmt.Fprintln(s.Out, compiler.For(cfg.Shell).OutputPath())
	return 0
}

func runGenerate(_ []string, s IO) int {
	cfg, err := storage.LoadConfig()
	if err != nil {
		fmt.Fprintln(s.Err, "Ошибка загрузки конфигурации:", err)
		return 1
	}
	for _, p := range validation.ValidateConfig(cfg) {
		fmt.Fprintln(s.Err, "Предупреждение: конфиг:", p)
	}
	c := compiler.For(cfg.Shell)
	if err := c.Generate(cfg); err != nil {
		fmt.Fprintln(s.Err, "Ошибка генерации:", err)
		return 1
	}
	fmt.Fprintln(s.Out, "Скрипт обновлён:", c.OutputPath())
	return 0
}
