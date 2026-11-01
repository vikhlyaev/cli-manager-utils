package compiler

import (
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
