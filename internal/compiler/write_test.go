package compiler

import (
	"os"
	"os/exec"
	"strings"
	"testing"

	"cli-manager/internal/models"
)

// TestGenerateRejectsBrokenScript — команда с синтаксической ошибкой в run не
// должна затирать рабочий скрипт: Generate возвращает ошибку, файл прежний.
func TestGenerateRejectsBrokenScript(t *testing.T) {
	for _, c := range []Compiler{ZshCompiler{}, BashCompiler{}} {
		shell := "zsh"
		if _, ok := c.(BashCompiler); ok {
			shell = "bash"
		}
		t.Run(shell, func(t *testing.T) {
			if _, err := exec.LookPath(shell); err != nil {
				t.Skip(shell + " не установлен")
			}
			t.Setenv("HOME", t.TempDir())

			good := &models.Config{Prefixes: []models.Prefix{{Name: "av", Commands: []models.Command{
				{Name: "hi", Run: "echo hi"},
			}}}}
			if err := c.Generate(good); err != nil {
				t.Fatalf("корректный конфиг должен генерироваться: %v", err)
			}
			before, _ := os.ReadFile(c.OutputPath())

			bad := &models.Config{Prefixes: []models.Prefix{{Name: "av", Commands: []models.Command{
				{Name: "hi", Run: "if then fi ("},
			}}}}
			err := c.Generate(bad)
			if err == nil || !strings.Contains(err.Error(), "прежний файл сохранён") {
				t.Fatalf("ожидалась ошибка проверки синтаксиса, получено %v", err)
			}
			after, _ := os.ReadFile(c.OutputPath())
			if string(after) != string(before) {
				t.Error("при ошибке синтаксиса скрипт на диске не должен меняться")
			}
		})
	}
}

// TestPrefixSurvivesSameNamedAlias — префикс с именем существующего alias не
// должен ломать загрузку скрипта: остальные префиксы определяются, а сам
// префикс доступен через \name.
func TestPrefixSurvivesSameNamedAlias(t *testing.T) {
	for _, c := range []Compiler{ZshCompiler{}, BashCompiler{}} {
		shell, flags := "zsh", []string{"-c"}
		if _, ok := c.(BashCompiler); ok {
			shell, flags = "bash", []string{"-O", "expand_aliases", "-c"}
		}
		t.Run(shell, func(t *testing.T) {
			if _, err := exec.LookPath(shell); err != nil {
				t.Skip(shell + " не установлен")
			}
			t.Setenv("HOME", t.TempDir())
			cfg := &models.Config{Prefixes: []models.Prefix{
				{Name: "gs", Commands: []models.Command{{Name: "hi", Run: "echo GS_HI"}}},
				{Name: "other", Commands: []models.Command{{Name: "hi", Run: "echo OTHER_HI"}}},
			}}
			if err := c.Generate(cfg); err != nil {
				t.Fatal(err)
			}
			script := "alias gs='echo ALIAS'\nsource " + c.OutputPath() + "\nother hi\n\\gs hi\n"
			out, err := exec.Command(shell, append(flags, script)...).CombinedOutput()
			if err != nil || !strings.Contains(string(out), "OTHER_HI") || !strings.Contains(string(out), "GS_HI") {
				t.Fatalf("скрипт должен загружаться при alias с тем же именем: err=%v\n%s", err, out)
			}
		})
	}
}
