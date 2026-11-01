package shellrc

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstallIdempotent(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	rc := filepath.Join(home, ".zshrc")
	script := filepath.Join(home, ".local/share/cli-manager/generated.zsh")
	if err := os.WriteFile(rc, []byte("autoload -Uz compinit && compinit"), 0o644); err != nil {
		t.Fatal(err)
	}

	if ok, _ := IsInstalled(rc, script); ok {
		t.Fatal("до установки скрипт не должен считаться подключённым")
	}
	changed, err := Install(rc, script)
	if err != nil || !changed {
		t.Fatalf("Install: changed=%v err=%v", changed, err)
	}
	changed, err = Install(rc, script)
	if err != nil || changed {
		t.Fatalf("повторный Install должен быть no-op: changed=%v err=%v", changed, err)
	}

	data, _ := os.ReadFile(rc)
	s := string(data)
	if strings.Count(s, beginMarker) != 1 {
		t.Errorf("блок должен быть ровно один:\n%s", s)
	}
	if !strings.HasPrefix(s, "autoload -Uz compinit && compinit\n\n") {
		t.Errorf("прежнее содержимое должно сохраниться:\n%s", s)
	}
	if !strings.Contains(s, `source "$HOME/.local/share/cli-manager/generated.zsh"`) {
		t.Errorf("путь должен быть через $HOME:\n%s", s)
	}
	if bak, err := os.ReadFile(rc + ".bak"); err != nil || string(bak) != "autoload -Uz compinit && compinit" {
		t.Errorf("бэкап: %q, %v", bak, err)
	}
	if ok, _ := IsInstalled(rc, script); !ok {
		t.Error("после установки скрипт должен считаться подключённым")
	}
}

func TestInstallCreatesMissingRC(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	rc := filepath.Join(home, ".bashrc")
	if _, err := Install(rc, filepath.Join(home, ".local/share/cli-manager/generated.bash")); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(rc)
	if !strings.HasPrefix(string(data), beginMarker) {
		t.Errorf("новый rc должен начинаться с блока:\n%s", data)
	}
	if _, err := os.Stat(rc + ".bak"); !os.IsNotExist(err) {
		t.Error("для нового файла бэкап не нужен")
	}
}

func TestManualSourceDetected(t *testing.T) {
	cases := map[string]bool{
		"source ~/.local/share/cli-manager/generated.zsh":   true,
		". $HOME/.local/share/cli-manager/generated.zsh":    true,
		"# source ~/.local/share/cli-manager/generated.zsh": false,
		"source ~/other.zsh":                                false,
	}
	for rc, want := range cases {
		if got := mentions(rc, "/h/.local/share/cli-manager/generated.zsh"); got != want {
			t.Errorf("mentions(%q) = %v, want %v", rc, got, want)
		}
	}
}
