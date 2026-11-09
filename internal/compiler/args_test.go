package compiler

import (
	"os"
	"os/exec"
	"strings"
	"testing"

	"cli-manager/internal/models"
)

// Сценарий `av mb loc profile`: группа mb делает cd, подкоманда loc
// сначала выполняет её действие и подставляет аргумент в свой run.
func TestSubcommandArgsAndAfterCommand(t *testing.T) {
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
			if err := os.MkdirAll(home+"/mb", 0o755); err != nil {
				t.Fatal(err)
			}
			cfg := &models.Config{Prefixes: []models.Prefix{{Name: "av", Commands: []models.Command{
				{Name: "mb", Run: "cd " + home + "/mb", Subcommands: []models.Command{
					{Name: "loc", Usage: "<файл>", AfterCommand: true, AcceptsArgs: true,
						Run: `echo "LOC=$1 IN=$PWD"`},
					{Name: "two", Usage: "<a> <b> [c]", Run: `echo "TWO=$1,$2"`},
				}},
			}}}}
			if err := c.Generate(cfg); err != nil {
				t.Fatal(err)
			}
			run := func(cmd string) string {
				o, _ := exec.Command(shell, "-c", "source "+c.OutputPath()+"\ncd /\n"+cmd+"; echo RC=$? PWD=$PWD").CombinedOutput()
				return string(o)
			}
			dir := home + "/mb"

			// Аргумент подставлен один раз (без дописанного "$@"), cd выполнен.
			if got := run("av mb loc profile"); !strings.Contains(got, "LOC=profile IN="+dir+"\nRC=0 PWD="+dir) {
				t.Errorf("av mb loc profile:\n%s", got)
			}
			// Группа без подкоманды выполняет своё действие, а не справку.
			if got := run("av mb"); !strings.Contains(got, "RC=0 PWD="+dir) || strings.Contains(got, "Использование") {
				t.Errorf("av mb должно делать cd:\n%s", got)
			}
			// Справка по-прежнему доступна по --help.
			if got := run("av mb --help"); !strings.Contains(got, "Использование: av mb") {
				t.Errorf("av mb --help:\n%s", got)
			}
			// Не хватает аргумента: подсказка, код 2, cd не выполнен.
			got := run("av mb loc")
			if !strings.Contains(got, "av mb loc: не хватает аргументов. Использование: av mb loc <файл>") ||
				!strings.Contains(got, "RC=2 PWD=/\n") || strings.Contains(got, "LOC=") {
				t.Errorf("av mb loc без аргумента:\n%s", got)
			}
			// [c] необязателен, <a> <b> — обязательны.
			if got := run("av mb two x"); !strings.Contains(got, "RC=2") {
				t.Errorf("av mb two x должно требовать 2 аргумента:\n%s", got)
			}
			if got := run("av mb two x y"); !strings.Contains(got, "TWO=x,y\nRC=0") {
				t.Errorf("av mb two x y:\n%s", got)
			}
		})
	}
}

func TestUsesArgs(t *testing.T) {
	for body, want := range map[string]bool{
		`rake loc="$1"`:   true,
		`echo ${2}`:       true,
		`echo "$@"`:       true,
		`echo $*`:         true,
		`echo $#`:         true,
		`echo ${#}`:       true,
		`echo $HOME $PWD`: false,
		`echo $0`:         false,
		`cat`:             false,
	} {
		if got := usesArgs(body); got != want {
			t.Errorf("usesArgs(%q) = %v, ожидалось %v", body, got, want)
		}
	}
}

func TestRequiredArgs(t *testing.T) {
	for usage, want := range map[string]int{
		"":                    0,
		"[строки]":            0,
		"<файл>":              1,
		"<файл> [строки]":     1,
		"<a> <b> [<c>]":       2,
		"<from> <to> [flags]": 2,
	} {
		if got := requiredArgs(usage); got != want {
			t.Errorf("requiredArgs(%q) = %d, ожидалось %d", usage, got, want)
		}
	}
}

// Конфиг со скриншота: у группы mb есть и флаг --profile, и подкоманда loc.
// Раньше флаги ветви отбрасывались, и `av mb --profile` делал только cd.
func TestBranchFlagsWithSubcommands(t *testing.T) {
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
			if err := os.MkdirAll(home+"/mb", 0o755); err != nil {
				t.Fatal(err)
			}
			cfg := &models.Config{Prefixes: []models.Prefix{{Name: "av", Commands: []models.Command{
				{Name: "mb", Run: "cd " + home + "/mb",
					Flags: []models.Flag{
						{Name: "--profile", Description: "Сборка для Profile", AfterCommand: true, Run: `echo "GEN IN=$PWD ARGS=$#"`},
						{Name: "--alone", Run: `echo "ALONE IN=$PWD"`},
					},
					Subcommands: []models.Command{
						{Name: "loc", AcceptsArgs: true, AfterCommand: true, Run: `echo "LOC file=$1 IN=$PWD"`},
					}},
			}}}}
			if err := c.Generate(cfg); err != nil {
				t.Fatal(err)
			}
			run := func(cmd string) string {
				o, _ := exec.Command(shell, "-c", "source "+c.OutputPath()+"\ncd /\n"+cmd+"; echo RC=$?").CombinedOutput()
				return string(o)
			}
			dir := home + "/mb"
			for cmd, want := range map[string]string{
				"av mb --profile":          "GEN IN=" + dir + " ARGS=0\nRC=0",
				"av mb --profile x":        "GEN IN=" + dir + " ARGS=1\nRC=0",
				"av mb --alone":            "ALONE IN=/\nRC=0",
				"av mb loc profile":        "LOC file=profile IN=" + dir + "\nRC=0",
				"av mb; echo PWD=$PWD":     "PWD=" + dir,
				"av mb --help":             "--profile",
				"av --help | grep profile": "--profile",
			} {
				if got := run(cmd); !strings.Contains(got, want) {
					t.Errorf("%s: нет %q в выводе:\n%s", cmd, want, got)
				}
			}
			if out := c.Render(cfg); !strings.Contains(out, "--profile") ||
				!strings.Contains(out[strings.Index(out, "_av_completions()"):], "--profile") {
				t.Errorf("флаг ветви должен быть в автодополнении:\n%s", out)
			}
		})
	}
}
