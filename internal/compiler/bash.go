package compiler

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"cli-manager/internal/models"
)

// BashCompiler генерирует Bash-совместимый скрипт с функциями-префиксами и
// автодополнением (complete -F / compgen).
type BashCompiler struct{}

// убеждаемся, что интерфейс реализован.
var _ Compiler = BashCompiler{}

// OutputPath возвращает путь к генерируемому скрипту
// (~/.local/share/cli-manager/generated.bash).
func (BashCompiler) OutputPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "share", "cli-manager", "generated.bash")
}

// Generate собирает скрипт и записывает его на диск.
func (b BashCompiler) Generate(cfg *models.Config) error {
	script := b.render(cfg)

	path := b.OutputPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(script), 0o644)
}

// Render возвращает полный текст скрипта без записи на диск (для предпросмотра).
func (b BashCompiler) Render(cfg *models.Config) string {
	return b.render(cfg)
}

// RenderPrefix возвращает сгенерированный фрагмент скрипта для одного префикса.
func (b BashCompiler) RenderPrefix(p models.Prefix) string {
	var sb strings.Builder
	b.renderPrefix(&sb, p)
	return sb.String()
}

// render формирует текст скрипта в памяти.
func (bc BashCompiler) render(cfg *models.Config) string {
	var b strings.Builder

	b.WriteString("#!/usr/bin/env bash\n")
	b.WriteString("# ─────────────────────────────────────────────\n")
	b.WriteString("# Сгенерировано cli-manager. Не редактируйте вручную.\n")
	b.WriteString("# ─────────────────────────────────────────────\n\n")

	renderVariables(&b, cfg.Variables)

	for _, p := range cfg.Prefixes {
		bc.renderPrefix(&b, p)
	}

	return b.String()
}

// renderPrefix генерирует шапку, справку, функцию-диспетчер (общие) и
// bash-специфичное автодополнение.
func (bc BashCompiler) renderPrefix(b *strings.Builder, p models.Prefix) {
	renderPrefixHeader(b, p)
	renderHelp(b, p)
	renderDispatcher(b, p)
	bc.renderCompletion(b, p)
}

// renderCompletion генерирует bash-функцию автодополнения (через compgen) и её
// регистрацию через complete. Команды с condition включаются в список дополнений
// только если условие истинно в текущем контексте.
func (BashCompiler) renderCompletion(b *strings.Builder, p models.Prefix) {
	fmt.Fprintf(b, "_%s_completions() {\n", p.Name)
	b.WriteString("    local cur=\"${COMP_WORDS[COMP_CWORD]}\"\n\n")

	// Первый аргумент — имя команды (с учётом контекстных условий).
	b.WriteString("    if [[ $COMP_CWORD -eq 1 ]]; then\n")
	b.WriteString("        local __cmds=\"\"\n")
	for _, c := range p.Commands {
		if c.Condition != "" {
			fmt.Fprintf(b, "        if %s; then __cmds+=%s; fi\n", c.Condition, shQuote(c.Name+" "))
		} else {
			fmt.Fprintf(b, "        __cmds+=%s\n", shQuote(c.Name+" "))
		}
	}
	b.WriteString("        COMPREPLY=( $(compgen -W \"$__cmds\" -- \"$cur\") )\n")
	b.WriteString("        return 0\n")
	b.WriteString("    fi\n\n")

	// Дальше — флаги/файлы конкретной команды.
	b.WriteString("    case \"${COMP_WORDS[1]}\" in\n")
	for _, c := range p.Commands {
		if len(c.Flags) == 0 && !c.AcceptsArgs {
			continue
		}
		fmt.Fprintf(b, "        %s)\n", shCasePattern(c.Name))
		if len(c.Flags) > 0 {
			names := make([]string, len(c.Flags))
			for i, f := range c.Flags {
				names[i] = f.Name
			}
			fmt.Fprintf(b, "            COMPREPLY=( $(compgen -W %s -- \"$cur\") )\n",
				shQuote(strings.Join(names, " ")))
		}
		if c.AcceptsArgs {
			b.WriteString("            COMPREPLY+=( $(compgen -f -- \"$cur\") )\n")
		}
		b.WriteString("            ;;\n")
	}
	b.WriteString("    esac\n")
	b.WriteString("}\n")

	// Регистрируем автодополнение, если доступна команда complete.
	fmt.Fprintf(b, "if type complete >/dev/null 2>&1; then complete -F _%s_completions %s; fi\n\n",
		p.Name, p.Name)
}
