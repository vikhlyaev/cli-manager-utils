package compiler

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"cli-manager/internal/models"
)

// ZshCompiler генерирует ZSH-совместимый скрипт с функциями-префиксами и
// автодополнением (compdef).
type ZshCompiler struct{}

// убеждаемся, что интерфейс реализован.
var _ Compiler = ZshCompiler{}

// OutputPath возвращает путь к генерируемому скрипту
// (~/.local/share/cli-manager/generated.zsh).
func (ZshCompiler) OutputPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "share", "cli-manager", "generated.zsh")
}

// Generate собирает скрипт и записывает его на диск.
func (z ZshCompiler) Generate(cfg *models.Config) error {
	script := z.render(cfg)

	path := z.OutputPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(script), 0o644)
}

// Render возвращает полный текст скрипта без записи на диск (для предпросмотра).
func (z ZshCompiler) Render(cfg *models.Config) string {
	return z.render(cfg)
}

// RenderPrefix возвращает сгенерированный фрагмент скрипта для одного префикса
// (функция-диспетчер, справка и автодополнение) — для предпросмотра в TUI.
func (z ZshCompiler) RenderPrefix(p models.Prefix) string {
	var b strings.Builder
	z.renderPrefix(&b, p)
	return b.String()
}

// render формирует текст скрипта в памяти (вынесено отдельно для тестируемости).
func (z ZshCompiler) render(cfg *models.Config) string {
	var b strings.Builder

	b.WriteString("#!/usr/bin/env zsh\n")
	b.WriteString("# ─────────────────────────────────────────────\n")
	b.WriteString("# Сгенерировано cli-manager. Не редактируйте вручную.\n")
	b.WriteString("# ─────────────────────────────────────────────\n\n")

	renderVariables(&b, cfg.Variables)

	for _, p := range cfg.Prefixes {
		z.renderPrefix(&b, p)
	}

	return b.String()
}

// renderPrefix генерирует шапку, справку, функцию-диспетчер (общие) и
// zsh-специфичное автодополнение.
func (z ZshCompiler) renderPrefix(b *strings.Builder, p models.Prefix) {
	renderPrefixHeader(b, p)
	renderHelp(b, p)
	renderDispatcher(b, p)
	z.renderCompletion(b, p)
}

// renderCompletion генерирует функцию автодополнения (через _arguments/_describe)
// и её регистрацию через compdef. Команды с condition включаются в список
// дополнений только если условие истинно в текущем контексте.
func (ZshCompiler) renderCompletion(b *strings.Builder, p models.Prefix) {
	fmt.Fprintf(b, "_%s_completions() {\n", p.Name)
	b.WriteString("    local curcontext=\"$curcontext\" state line\n")
	b.WriteString("    _arguments -C \\\n")
	b.WriteString("        '1: :->cmds' \\\n")
	b.WriteString("        '*::arg:->args'\n\n")
	b.WriteString("    case \"$state\" in\n")

	// Список команд (с учётом контекстных условий).
	b.WriteString("        cmds)\n")
	b.WriteString("            local -a __commands\n")
	for _, c := range p.Commands {
		entry := fmt.Sprintf("%s:%s", c.Name, c.Description)
		if c.Condition != "" {
			fmt.Fprintf(b, "            if %s; then __commands+=(%s); fi\n",
				c.Condition, shQuote(entry))
		} else {
			fmt.Fprintf(b, "            __commands+=(%s)\n", shQuote(entry))
		}
	}
	b.WriteString("            _describe -t commands 'команда' __commands\n")
	b.WriteString("            ;;\n")

	// Автодополнение аргументов/флагов конкретной команды.
	b.WriteString("        args)\n")
	b.WriteString("            case \"$line[1]\" in\n")
	for _, c := range p.Commands {
		if len(c.Flags) == 0 && !c.AcceptsArgs {
			continue
		}
		fmt.Fprintf(b, "                %s)\n", shCasePattern(c.Name))
		if len(c.Flags) > 0 {
			b.WriteString("                    local -a __flags\n")
			for _, f := range c.Flags {
				entry := fmt.Sprintf("%s:%s", f.Name, f.Description)
				fmt.Fprintf(b, "                    __flags+=(%s)\n", shQuote(entry))
			}
			b.WriteString("                    _describe -t flags 'флаг' __flags\n")
		}
		if c.AcceptsArgs {
			b.WriteString("                    _files\n")
		}
		b.WriteString("                    ;;\n")
	}
	b.WriteString("            esac\n")
	b.WriteString("            ;;\n")
	b.WriteString("    esac\n")
	b.WriteString("}\n")

	// Регистрируем автодополнение только если система completion загружена
	// (compdef доступна) — иначе source в неинтерактивном zsh упадёт с ошибкой.
	fmt.Fprintf(b, "if typeset -f compdef >/dev/null; then compdef _%s_completions %s; fi\n\n",
		p.Name, p.Name)
}
