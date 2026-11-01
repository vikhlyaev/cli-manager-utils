package compiler

import (
	"os"
	"os/exec"
	"strings"
	"testing"

	"cli-manager/internal/models"
)

// TestDisabledSkipped — выключенные префикс, команда (вместе с подкомандами) и
// флаг не попадают ни в диспетчер, ни в справку, ни в автодополнение.
func TestDisabledSkipped(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	cfg := &models.Config{Prefixes: []models.Prefix{
		{Name: "off", Disabled: true, Commands: []models.Command{{Name: "x", Run: "echo OFF_PREFIX"}}},
		{Name: "av", Commands: []models.Command{
			{Name: "on", Run: "echo ON", Flags: []models.Flag{
				{Name: "--keep", Run: "echo KEEP"},
				{Name: "--gone", Run: "echo GONE_FLAG", Disabled: true},
			}},
			{Name: "hidden", Run: "echo HIDDEN", Disabled: true,
				Subcommands: []models.Command{{Name: "deep", Run: "echo DEEP"}}},
		}},
	}}
	for _, c := range []Compiler{ZshCompiler{}, BashCompiler{}} {
		out := c.Render(cfg)
		for _, gone := range []string{"function off", "OFF_PREFIX", "GONE_FLAG", "--gone", "HIDDEN", "hidden", "DEEP"} {
			if strings.Contains(out, gone) {
				t.Errorf("%T: выключенный элемент %q попал в скрипт", c, gone)
			}
		}
		for _, kept := range []string{"function av", "echo ON", "--keep"} {
			if !strings.Contains(out, kept) {
				t.Errorf("%T: включённый элемент %q пропал", c, kept)
			}
		}
		if got := c.RenderPrefix(cfg.Prefixes[0]); got != disabledPrefixNote {
			t.Errorf("%T: предпросмотр выключенного префикса = %q", c, got)
		}
		if strings.Contains(c.RenderPrefix(cfg.Prefixes[1]), "HIDDEN") {
			t.Errorf("%T: предпросмотр префикса показывает выключенную команду", c)
		}
	}
	// Исходная конфигурация не меняется.
	if len(cfg.Prefixes) != 2 || len(cfg.Prefixes[1].Commands) != 2 || len(cfg.Prefixes[1].Commands[0].Flags) != 2 {
		t.Error("фильтрация не должна менять исходный конфиг")
	}
}

// TestUsageInHelp — описание аргументов выводится в справке рядом с именем,
// а колонка описаний выравнивается с его учётом.
func TestUsageInHelp(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	cfg := &models.Config{Prefixes: []models.Prefix{{Name: "av", Commands: []models.Command{
		{Name: "note", Usage: "<текст>", Description: "заметка", Run: "echo"},
		{Name: "ls", Description: "список", Run: "ls"},
	}}}}
	out := ZshCompiler{}.Render(cfg)
	// "note <текст>" — 12 символов; описание ls выровнено по той же колонке.
	if !strings.Contains(out, `printf '%s\n' '  note <текст>  заметка'`) {
		t.Errorf("usage должен идти после имени:\n%s", out)
	}
	if !strings.Contains(out, `printf '%s\n' '  ls            список'`) {
		t.Errorf("колонка описаний должна учитывать usage:\n%s", out)
	}
}

// TestBranchWithAllChildrenDisabled — ветвь, у которой выключены все
// подкоманды, пропадает целиком, а не становится листом со своими флагами.
func TestBranchWithAllChildrenDisabled(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	cfg := &models.Config{Prefixes: []models.Prefix{{Name: "av", Commands: []models.Command{
		{Name: "db", Run: "echo FALLBACK", Flags: []models.Flag{{Name: "--stray", Run: "echo STRAY"}},
			Subcommands: []models.Command{{Name: "up", Run: "echo UP", Disabled: true}}},
		{Name: "ok", Run: "echo OK"},
	}}}}
	out := ZshCompiler{}.Render(cfg)
	for _, gone := range []string{"'db'", "FALLBACK", "--stray", "UP"} {
		if strings.Contains(out, gone) {
			t.Errorf("%q не должно попадать в скрипт", gone)
		}
	}
}

// TestHelpTextIsLiteral — $(...) и $VAR в описаниях и usage печатаются как
// есть и не выполняются при показе справки или автодополнении.
func TestHelpTextIsLiteral(t *testing.T) {
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
			cfg := &models.Config{Prefixes: []models.Prefix{{Name: "av", Description: "it's $HOME",
				Commands: []models.Command{{Name: "x", Usage: "$FILE", Description: "$(echo PWNED) 'q'", Run: "true"}}}}}
			if err := c.Generate(cfg); err != nil {
				t.Fatal(err)
			}
			out, err := exec.Command(shell, "-c", "source "+c.OutputPath()+"; av").CombinedOutput()
			if err != nil {
				t.Fatalf("%v\n%s", err, out)
			}
			for _, want := range []string{"it's $HOME", "x $FILE", "$(echo PWNED) 'q'"} {
				if !strings.Contains(string(out), want) {
					t.Errorf("нет %q в справке:\n%s", want, out)
				}
			}
		})
	}
}

// TestFlagAfterCommand — флаг с after_command сначала выполняет действие
// команды в текущей оболочке (cd остаётся в силе), а при его ошибке не
// запускается; обычный флаг по-прежнему заменяет действие команды.
func TestFlagAfterCommand(t *testing.T) {
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
			cfg := &models.Config{Prefixes: []models.Prefix{{Name: "av", Commands: []models.Command{
				{Name: "vault", Run: "cd " + home + "/vault\necho BASE", Flags: []models.Flag{
					{Name: "--here", Run: "echo HERE=$PWD", AfterCommand: true},
					{Name: "--alone", Run: "echo ALONE"},
				}},
				{Name: "broken", Run: "cd /nonexistent-dir-xyz", Flags: []models.Flag{
					{Name: "--tool", Run: "echo TOOL_RAN", AfterCommand: true},
				}},
			}}}}
			if err := os.MkdirAll(home+"/vault", 0o755); err != nil {
				t.Fatal(err)
			}
			if err := c.Generate(cfg); err != nil {
				t.Fatal(err)
			}
			script := "source " + c.OutputPath() + "\n" +
				"cd /\nav vault --here; echo PWD_AFTER=$PWD\n" +
				"cd /\nav vault --alone; echo PWD_ALONE=$PWD\n" +
				"av broken --tool 2>/dev/null; echo RC=$?\n"
			out, _ := exec.Command(shell, "-c", script).CombinedOutput()
			s := string(out)
			real := home + "/vault"
			for _, want := range []string{"BASE\nHERE=" + real, "PWD_AFTER=" + real, "ALONE\nPWD_ALONE=/\n", "RC=1"} {
				if !strings.Contains(s, want) {
					t.Errorf("нет %q в выводе:\n%s", want, s)
				}
			}
			if strings.Contains(s, "TOOL_RAN") {
				t.Errorf("при ошибке действия команды флаг не должен запускаться:\n%s", s)
			}
		})
	}
}
