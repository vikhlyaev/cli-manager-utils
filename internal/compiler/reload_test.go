package compiler

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"cli-manager/internal/models"
)

// TestReloadHook — открытая оболочка подхватывает перегенерированный скрипт:
// новая команда появляется, удалённый префикс исчезает, а без изменений хук
// ничего не делает.
func TestReloadHook(t *testing.T) {
	for _, c := range []Compiler{ZshCompiler{}, BashCompiler{}} {
		shell := "zsh"
		if _, ok := c.(BashCompiler); ok {
			shell = "bash"
		}
		t.Run(shell, func(t *testing.T) {
			if _, err := exec.LookPath(shell); err != nil {
				t.Skip(shell + " не установлен")
			}
			home := t.TempDir()
			t.Setenv("HOME", home)
			path := c.OutputPath()

			// Поколение 1: префиксы old и keep.
			v1 := &models.Config{Prefixes: []models.Prefix{
				{Name: "old", Commands: []models.Command{{Name: "hi", Run: "echo OLD"}}},
				{Name: "keep", Commands: []models.Command{{Name: "hi", Run: "echo KEEP1"}}},
			}}
			if err := c.Generate(v1); err != nil {
				t.Fatal(err)
			}
			v1dir := filepath.Join(home, "v1")
			copyGen(t, path, v1dir)

			// Поколение 2: old удалён, keep изменён.
			v2 := &models.Config{Prefixes: []models.Prefix{
				{Name: "keep", Commands: []models.Command{{Name: "hi", Run: "echo KEEP2"}}},
			}}
			if err := c.Generate(v2); err != nil {
				t.Fatal(err)
			}
			v2dir := filepath.Join(home, "v2")
			copyGen(t, path, v2dir)

			base := filepath.Base(path)
			script := strings.Join([]string{
				"cp " + v1dir + "/* " + filepath.Dir(path) + "/",
				"source " + path,
				"__cli_manager_reload", // без изменений — no-op
				"keep hi",
				"cp " + v2dir + "/" + base + " " + v2dir + "/" + base + ".stamp " + filepath.Dir(path) + "/",
				"__cli_manager_reload",
				"keep hi",
				"if type old >/dev/null 2>&1; then echo OLD_STILL_DEFINED; else echo OLD_GONE; fi",
			}, "\n")
			out, err := exec.Command(shell, "-c", script).CombinedOutput()
			s := string(out)
			if err != nil {
				t.Fatalf("ошибка оболочки: %v\n%s", err, s)
			}
			for _, want := range []string{"KEEP1", "KEEP2", "OLD_GONE"} {
				if !strings.Contains(s, want) {
					t.Errorf("нет %q в выводе:\n%s", want, s)
				}
			}
		})
	}
}

// copyGen копирует скрипт и его stamp-файл в каталог dir.
func copyGen(t *testing.T, path, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{path, StampPath(path)} {
		data, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, filepath.Base(p)), data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// TestStampDeterministic — одинаковый конфиг даёт одинаковое поколение, иначе
// терминалы перезагружали бы скрипт после каждого запуска приложения.
func TestStampDeterministic(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	a := ZshCompiler{}.render(sampleConfig())
	b := ZshCompiler{}.render(sampleConfig())
	if a != b {
		t.Fatal("рендер должен быть детерминированным")
	}
	if strings.Contains(a, genPlaceholder) {
		t.Fatal("плейсхолдер поколения должен быть заменён")
	}
}
