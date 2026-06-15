package compiler

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

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
	if !strings.Contains(out, `if [[ -f docker-compose.yml ]]; then __cmds+="deploy "`) {
		t.Errorf("условие должно гейтить команду в bash-автодополнении:\n%s", out)
	}
}
