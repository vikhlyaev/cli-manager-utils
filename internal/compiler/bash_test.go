package compiler

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"cli-manager/internal/models"
)

// nestedConfig — конфигурация с вложенными подкомандами (av db migrate up).
func nestedConfig() *models.Config {
	return &models.Config{
		Prefixes: []models.Prefix{{
			Name:        "av",
			Description: "тулкит",
			Commands: []models.Command{
				{Name: "note", Run: "echo note", AcceptsArgs: true},
				{Name: "db", Description: "база", Subcommands: []models.Command{
					{Name: "migrate", Subcommands: []models.Command{
						{Name: "up", Run: "echo up", Flags: []models.Flag{{Name: "--force", Run: "echo up-force"}}},
						{Name: "down", Run: "echo down"},
					}},
					{Name: "seed", Run: "echo seed", Condition: "[[ -f seeds.sql ]]"},
				}},
			},
		}},
	}
}

func syntaxOK(t *testing.T, shell, ext, script string) {
	t.Helper()
	if _, err := exec.LookPath(shell); err != nil {
		t.Skipf("%s недоступен", shell)
	}
	f := filepath.Join(t.TempDir(), "generated."+ext)
	if err := os.WriteFile(f, []byte(script), 0o644); err != nil {
		t.Fatal(err)
	}
	if combined, err := exec.Command(shell, "-n", f).CombinedOutput(); err != nil {
		t.Fatalf("%s -n отверг скрипт: %v\n%s\n---\n%s", shell, err, combined, script)
	}
}

func TestNestedRenderSyntaxValid(t *testing.T) {
	syntaxOK(t, "bash", "bash", BashCompiler{}.render(nestedConfig()))
	syntaxOK(t, "zsh", "zsh", ZshCompiler{}.render(nestedConfig()))
}

func TestNestedDispatchAndHelp(t *testing.T) {
	out := ZshCompiler{}.render(nestedConfig())
	// help-функции генерируются для каждого уровня-ветви
	for _, fn := range []string{"__av_help()", "__av_db_help()", "__av_db_migrate_help()"} {
		if !strings.Contains(out, fn) {
			t.Errorf("ожидалась help-функция %s:\n%s", fn, out)
		}
	}
	// вложенный диспетчер использует уникальные переменные уровней
	if !strings.Contains(out, `local __arg2="$1"`) || !strings.Contains(out, `local __arg3="$1"`) {
		t.Errorf("ожидались переменные уровней __arg2/__arg3:\n%s", out)
	}
	// автодополнение содержит ветки по путям вложенных уровней
	if !strings.Contains(out, `'db migrate')`) || !strings.Contains(out, `'db migrate up')`) {
		t.Errorf("ожидались ветки автодополнения по путям 'db migrate' и 'db migrate up':\n%s", out)
	}
}

func TestBashNestedExecution(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash недоступен")
	}
	f := filepath.Join(t.TempDir(), "g.bash")
	if err := os.WriteFile(f, []byte(BashCompiler{}.render(nestedConfig())), 0o644); err != nil {
		t.Fatal(err)
	}
	run := func(args string) string {
		o, _ := exec.Command("bash", "-c", "source "+f+"; "+args).CombinedOutput()
		return strings.TrimSpace(string(o))
	}

	cases := map[string]string{
		"av db migrate up":         "up",
		"av db migrate down":       "down",
		"av db migrate up --force": "up-force",
		"av note hello":            "note hello", // accepts_args -> проброс "$@"
	}
	for cmd, want := range cases {
		if got := run(cmd); got != want {
			t.Errorf("%q => %q, ожидалось %q", cmd, got, want)
		}
	}

	// Ветвь без подкоманды печатает справку своего уровня.
	if got := run("av db"); !strings.Contains(got, "Использование: av db") {
		t.Errorf("`av db` без подкоманды должно печатать справку уровня, получено %q", got)
	}
}

func TestForSelectsCompiler(t *testing.T) {
	if _, ok := For("bash").(BashCompiler); !ok {
		t.Error(`For("bash") должен возвращать BashCompiler`)
	}
	if _, ok := For("zsh").(ZshCompiler); !ok {
		t.Error(`For("zsh") должен возвращать ZshCompiler`)
	}
	if _, ok := For("").(ZshCompiler); !ok {
		t.Error(`For("") должен возвращать ZshCompiler по умолчанию`)
	}
	if _, ok := For("FISH").(ZshCompiler); !ok {
		t.Error("неизвестная оболочка должна давать ZshCompiler")
	}
}

func TestBashRenderSyntaxValid(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash недоступен")
	}
	out := BashCompiler{}.render(sampleConfig())
	f := filepath.Join(t.TempDir(), "generated.bash")
	if err := os.WriteFile(f, []byte(out), 0o644); err != nil {
		t.Fatal(err)
	}
	if combined, err := exec.Command("bash", "-n", f).CombinedOutput(); err != nil {
		t.Fatalf("bash -n отверг сгенерированный скрипт: %v\n%s\n---\n%s", err, combined, out)
	}
}

func TestZshRenderSyntaxValid(t *testing.T) {
	if _, err := exec.LookPath("zsh"); err != nil {
		t.Skip("zsh недоступен")
	}
	out := ZshCompiler{}.render(sampleConfig())
	f := filepath.Join(t.TempDir(), "generated.zsh")
	if err := os.WriteFile(f, []byte(out), 0o644); err != nil {
		t.Fatal(err)
	}
	if combined, err := exec.Command("zsh", "-n", f).CombinedOutput(); err != nil {
		t.Fatalf("zsh -n отверг сгенерированный скрипт: %v\n%s\n---\n%s", err, combined, out)
	}
}

func TestBashCompletionRegistration(t *testing.T) {
	out := BashCompiler{}.render(sampleConfig())
	if !strings.Contains(out, "complete -F _av_completions av") {
		t.Errorf("ожидалась регистрация complete:\n%s", out)
	}
	if !strings.Contains(out, "compgen -W") {
		t.Errorf("ожидался compgen для списка команд:\n%s", out)
	}
	// Контекстное условие гейтит добавление команды в автодополнение.
	if !strings.Contains(out, `if [[ -f docker-compose.yml ]]; then __c+="deploy "; fi`) {
		t.Errorf("условие должно гейтить команду в bash-автодополнении:\n%s", out)
	}
}
