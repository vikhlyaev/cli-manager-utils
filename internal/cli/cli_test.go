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
