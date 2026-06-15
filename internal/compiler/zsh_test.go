package compiler

import (
	"strings"
	"testing"

	"cli-manager/internal/models"
)

func sampleConfig() *models.Config {
	return &models.Config{
		Variables: map[string]string{"VAULT_DIR": "~/Documents/vault"},
		Prefixes: []models.Prefix{{
			Name:        "av",
			Description: "тулкит",
			Commands: []models.Command{
				{Name: "note", Run: `echo "$@" >> ${VAULT_DIR}/inbox.md`, AcceptsArgs: true},
				{Name: "deploy", Run: "docker-compose up -d", Condition: "[[ -f docker-compose.yml ]]",
					Flags: []models.Flag{{Name: "--force", Run: "docker-compose up -d --build"}}},
			},
		}},
	}
}

func TestRenderTildeExpansion(t *testing.T) {
	out := ZshCompiler{}.render(sampleConfig())
	if !strings.Contains(out, `export VAULT_DIR="$HOME/Documents/vault"`) {
		t.Errorf("ожидалось разворачивание тильды в $HOME, получено:\n%s", out)
	}
}

func TestRenderConditionGatesCompletion(t *testing.T) {
	out := ZshCompiler{}.render(sampleConfig())
	if !strings.Contains(out, `if [[ -f docker-compose.yml ]]; then __c+=("deploy:"); fi`) {
		t.Errorf("условие должно оборачивать добавление команды в автодополнение:\n%s", out)
	}
	// команда без условия добавляется безусловно
	if !strings.Contains(out, `__c+=("note:")`) {
		t.Errorf("команда без условия должна добавляться безусловно:\n%s", out)
	}
}

func TestRenderCompdefGuarded(t *testing.T) {
	out := ZshCompiler{}.render(sampleConfig())
	if !strings.Contains(out, "if typeset -f compdef >/dev/null; then compdef _av_completions av; fi") {
		t.Errorf("регистрация compdef должна быть защищена проверкой:\n%s", out)
	}
}

func TestCommandBodyAppendsArgs(t *testing.T) {
	// accepts_args без явного $@ -> аргументы пробрасываются
	got := commandBody(models.Command{Run: "cat", AcceptsArgs: true})
	if got != `cat "$@"` {
		t.Errorf("ожидалось добавление \"$@\", получено: %q", got)
	}
	// если $@ уже есть — не дублируем
	got = commandBody(models.Command{Run: `echo "$@"`, AcceptsArgs: true})
	if got != `echo "$@"` {
		t.Errorf("не должно дублировать $@, получено: %q", got)
	}
	// без accepts_args — без изменений
	got = commandBody(models.Command{Run: "ls", AcceptsArgs: false})
	if got != "ls" {
		t.Errorf("без accepts_args тело не меняется, получено: %q", got)
	}
}
