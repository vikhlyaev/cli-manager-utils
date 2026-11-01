package importer

import (
	"strings"
	"testing"
)

func TestParseAliases(t *testing.T) {
	src := `
# комментарий
alias ll='ls -la'
alias gs="git status"
alias plain=top
alias -g G='| grep'
alias empty=''
not an alias line
alias gp='git push origin "$@"'
`
	got := ParseAliases(strings.NewReader(src))

	want := map[string]string{
		"ll":    "ls -la",
		"gs":    "git status",
		"plain": "top",
		"G":     "| grep",
		"gp":    `git push origin "$@"`,
	}

	if len(got) != len(want) {
		t.Fatalf("ожидалось %d алиасов, получено %d: %+v", len(want), len(got), got)
	}
	for _, a := range got {
		w, ok := want[a.Name]
		if !ok {
			t.Errorf("неожиданный alias %q", a.Name)
			continue
		}
		if a.Value != w {
			t.Errorf("alias %q: ожидалось %q, получено %q", a.Name, w, a.Value)
		}
	}
}

func TestParseAliasesEmpty(t *testing.T) {
	if got := ParseAliases(strings.NewReader("echo hi\n# nothing\n")); len(got) != 0 {
		t.Errorf("ожидался пустой результат, получено %+v", got)
	}
}

func TestZshrcPathHonorsZDOTDIR(t *testing.T) {
	t.Setenv("ZDOTDIR", "/tmp/zdot")
	if got := ZshrcPath(); got != "/tmp/zdot/.zshrc" {
		t.Errorf("ZshrcPath() = %q", got)
	}
}
