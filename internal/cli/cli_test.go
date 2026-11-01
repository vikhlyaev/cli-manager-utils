package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"cli-manager/internal/models"
	"cli-manager/internal/storage"
)

// run выполняет подкоманду с изолированным HOME и возвращает код и вывод.
func run(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	var out, errb bytes.Buffer
	code := Run(args, IO{In: strings.NewReader(""), Out: &out, Err: &errb})
	return code, out.String(), errb.String()
}

func TestHelpAndUnknown(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	for _, a := range []string{"help", "-h", "--help"} {
		code, out, _ := run(t, a)
		if code != 0 || !strings.Contains(out, "generate") {
			t.Errorf("%s: code=%d out=%q", a, code, out)
		}
	}
	code, _, errOut := run(t, "nope")
	if code != 2 || !strings.Contains(errOut, "неизвестная команда") {
		t.Errorf("nope: code=%d err=%q", code, errOut)
	}
}

func TestVersion(t *testing.T) {
	code, out, _ := run(t, "--version")
	if code != 0 || !strings.HasPrefix(out, "cli-manager ") {
		t.Errorf("code=%d out=%q", code, out)
	}
}

func TestPathFollowsShell(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	_, out, _ := run(t, "path")
	if strings.TrimSpace(out) != filepath.Join(home, ".local/share/cli-manager/generated.zsh") {
		t.Errorf("zsh path = %q", out)
	}
	if err := storage.SaveConfig(&models.Config{Shell: "bash"}); err != nil {
		t.Fatal(err)
	}
	_, out, _ = run(t, "path")
	if !strings.HasSuffix(strings.TrimSpace(out), "generated.bash") {
		t.Errorf("bash path = %q", out)
	}
}

func TestGenerate(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	cfg := &models.Config{Prefixes: []models.Prefix{{
		Name: "av", Commands: []models.Command{{Name: "hi", Run: "echo hi"}},
	}}}
	if err := storage.SaveConfig(cfg); err != nil {
		t.Fatal(err)
	}
	code, out, errOut := run(t, "generate")
	if code != 0 {
		t.Fatalf("code=%d err=%q", code, errOut)
	}
	if !strings.Contains(out, "generated.zsh") {
		t.Errorf("out=%q", out)
	}
	data, err := os.ReadFile(filepath.Join(home, ".local/share/cli-manager/generated.zsh"))
	if err != nil || !strings.Contains(string(data), "function av {") {
		t.Errorf("скрипт не записан: %v", err)
	}
}

func TestInit(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	rc := filepath.Join(home, ".zshrc")

	// Отказ на вопрос — файл не трогаем.
	var out, errb bytes.Buffer
	code := Run([]string{"init"}, IO{In: strings.NewReader("n\n"), Out: &out, Err: &errb})
	if code != 1 {
		t.Fatalf("отказ: code=%d", code)
	}
	if _, err := os.Stat(rc); !os.IsNotExist(err) {
		t.Fatal("после отказа rc не должен создаваться")
	}

	// Согласие — блок добавлен и скрипт сгенерирован.
	out.Reset()
	code = Run([]string{"init"}, IO{In: strings.NewReader("y\n"), Out: &out, Err: &errb})
	if code != 0 {
		t.Fatalf("согласие: code=%d err=%s", code, errb.String())
	}
	data, _ := os.ReadFile(rc)
	if !strings.Contains(string(data), "generated.zsh") {
		t.Errorf("rc без подключения:\n%s", data)
	}
	if _, err := os.Stat(filepath.Join(home, ".local/share/cli-manager/generated.zsh")); err != nil {
		t.Errorf("скрипт не создан: %v", err)
	}

	// Повторный запуск — ничего не меняет.
	code, out2, _ := run(t, "init", "--yes")
	if code != 0 || !strings.Contains(out2, "уже подключён") {
		t.Errorf("повтор: code=%d out=%q", code, out2)
	}

	code, block, _ := run(t, "init", "--print")
	if code != 0 || !strings.Contains(block, "# >>> cli-manager >>>") {
		t.Errorf("--print: code=%d out=%q", code, block)
	}
}

func TestDoctor(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if err := storage.SaveConfig(&models.Config{Prefixes: []models.Prefix{{
		Name: "zz_cli_manager_test", Commands: []models.Command{{Name: "hi", Run: "echo hi"}},
	}}}); err != nil {
		t.Fatal(err)
	}

	// Чистый HOME: скрипта нет и он не подключён.
	code, out, _ := run(t, "doctor")
	if code != 1 || !strings.Contains(out, "cli-manager generate") || !strings.Contains(out, "cli-manager init") {
		t.Fatalf("ожидались ошибки: code=%d\n%s", code, out)
	}

	rc := filepath.Join(home, ".zshrc")
	if err := os.WriteFile(rc, []byte("autoload -Uz compinit && compinit\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if code, _, errOut := run(t, "init", "--yes"); code != 0 {
		t.Fatalf("init: %s", errOut)
	}
	code, out, _ = run(t, "doctor")
	if code != 0 || strings.Contains(out, "✗") {
		t.Fatalf("после init всё должно быть исправно: code=%d\n%s", code, out)
	}
	if !strings.Contains(out, "compinit найден") {
		t.Errorf("нет проверки compinit:\n%s", out)
	}
}

// TestDoctorCompinitDetection — compinit в комментарии не считается, а
// фреймворк вроде oh-my-zsh — считается.
func TestDoctorCompinitDetection(t *testing.T) {
	cases := map[string]string{
		"# autoload -Uz compinit && compinit\n": "нет compinit",
		"source $ZSH/oh-my-zsh.sh\n":            "compinit найден",
		"autoload -Uz compinit && compinit\n":   "compinit найден",
	}
	for rcText, want := range cases {
		home := t.TempDir()
		t.Setenv("HOME", home)
		rc := filepath.Join(home, ".zshrc")
		if err := os.WriteFile(rc, []byte(rcText), 0o644); err != nil {
			t.Fatal(err)
		}
		if code, _, errOut := run(t, "init", "--yes"); code != 0 {
			t.Fatalf("init: %s", errOut)
		}
		_, out, _ := run(t, "doctor")
		if !strings.Contains(out, want) {
			t.Errorf("rc %q: ожидалось %q в:\n%s", rcText, want, out)
		}
	}
}

func TestUIInHelpAndBadFlag(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	if _, out, _ := run(t, "help"); !strings.Contains(out, "ui") {
		t.Errorf("в справке нет ui: %q", out)
	}
	if code, _, _ := run(t, "ui", "--no-such-flag"); code != 2 {
		t.Errorf("неизвестный флаг ui: code=%d", code)
	}
}
