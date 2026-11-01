package validation

import (
	"strings"
	"testing"

	"cli-manager/internal/models"
)

func TestValidateConfigClean(t *testing.T) {
	cfg := &models.Config{
		Variables: map[string]string{"VAULT_DIR": "~/v"},
		Prefixes: []models.Prefix{{
			Name: "av",
			Commands: []models.Command{
				{Name: "note", Run: "echo", Flags: []models.Flag{{Name: "--x"}, {Name: "-y"}}},
				{Name: "db", Subcommands: []models.Command{{Name: "up", Condition: "[[ -f x ]]"}}},
			},
		}},
	}
	if ps := ValidateConfig(cfg); len(ps) != 0 {
		t.Fatalf("ожидался чистый конфиг, получено: %v", ps)
	}
}

func TestValidateConfigProblems(t *testing.T) {
	cfg := &models.Config{
		Variables: map[string]string{"2bad": "x"},
		Prefixes: []models.Prefix{
			{Name: "av", Commands: []models.Command{
				{Name: "note"},
				{Name: "note"},
				{Name: "db", Subcommands: []models.Command{
					{Name: "migrate", Subcommands: []models.Command{
						{Name: "up", Condition: "[[ -f x"},
					}},
				}},
				{Name: "f", Flags: []models.Flag{{Name: "--x"}, {Name: "--x"}, {Name: "bad flag"}}},
			}},
			{Name: "av"},
			{Name: "my cmd"},
		},
	}

	ps := ValidateConfig(cfg)
	var got []string
	for _, p := range ps {
		got = append(got, p.String())
	}
	all := strings.Join(got, "\n")

	wants := []string{
		"переменная 2bad",
		"префикс av: префикс с таким именем уже есть",
		"префикс my cmd",
		"av note: команда с таким именем уже есть",
		"av db migrate up: ошибка в условии",
		"av f --x: флаг с таким именем уже есть",
		"av f bad flag",
	}
	for _, w := range wants {
		if !strings.Contains(all, w) {
			t.Errorf("нет проблемы %q в:\n%s", w, all)
		}
	}
	if len(ps) != len(wants) {
		t.Errorf("ожидалось %d проблем, получено %d:\n%s", len(wants), len(ps), all)
	}
}
