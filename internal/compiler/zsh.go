package compiler

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
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

	z.renderVariables(&b, cfg.Variables)

	for _, p := range cfg.Prefixes {
		z.renderPrefix(&b, p)
	}

	return b.String()
}

// renderVariables экспортирует глобальные переменные. Ключи сортируются для
// детерминированного вывода.
func (ZshCompiler) renderVariables(b *strings.Builder, vars map[string]string) {
	if len(vars) == 0 {
		return
	}

	keys := make([]string, 0, len(vars))
	for k := range vars {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	b.WriteString("# --- Глобальные переменные ---\n")
	for _, k := range keys {
		fmt.Fprintf(b, "export %s=%s\n", k, zshQuote(expandTilde(vars[k])))
	}
	b.WriteString("\n")
}

// renderPrefix генерирует функцию-диспетчер и функцию автодополнения.
func (z ZshCompiler) renderPrefix(b *strings.Builder, p models.Prefix) {
	b.WriteString("# ─────────────────────────────────────────────\n")
	if p.Description != "" {
		fmt.Fprintf(b, "# Префикс: %s — %s\n", p.Name, p.Description)
	} else {
		fmt.Fprintf(b, "# Префикс: %s\n", p.Name)
	}
	b.WriteString("# ─────────────────────────────────────────────\n\n")

	z.renderHelp(b, p)

	// Определяет, перехватывает ли сам префикс ввод `help` под справку. Если
	// среди команд есть собственная команда с именем "help", не затеняем её.
	hasHelpCmd := false
	for _, c := range p.Commands {
		if c.Name == "help" {
			hasHelpCmd = true
			break
		}
	}
	helpPattern := `""|-h|--help`
	if !hasHelpCmd {
		helpPattern = `""|-h|--help|help`
	}

	// Функция-диспетчер.
	fmt.Fprintf(b, "%s() {\n", p.Name)
	b.WriteString("    local __cmd=\"$1\"\n")
	b.WriteString("    shift 2>/dev/null\n\n")
	b.WriteString("    case \"$__cmd\" in\n")

	// Пустой ввод или запрос справки — показываем список всех команд.
	fmt.Fprintf(b, "        %s)\n", helpPattern)
	fmt.Fprintf(b, "            __%s_help\n", p.Name)
	b.WriteString("            return 0\n")
	b.WriteString("            ;;\n")

	for _, c := range p.Commands {
		z.renderCommand(b, c)
	}

	b.WriteString("        *)\n")
	fmt.Fprintf(b, "            echo \"%s: неизвестная команда: $__cmd\" >&2\n", p.Name)
	fmt.Fprintf(b, "            __%s_help >&2\n", p.Name)
	b.WriteString("            return 1\n")
	b.WriteString("            ;;\n")
	b.WriteString("    esac\n")
	b.WriteString("}\n\n")

	z.renderCompletion(b, p)
}

// renderHelp генерирует функцию __<name>_help, печатающую заголовок префикса и
// список всех команд с описаниями. Команды с condition попадают в справку только
// если их условие истинно в текущем контексте (как и в автодополнении).
func (ZshCompiler) renderHelp(b *strings.Builder, p models.Prefix) {
	fmt.Fprintf(b, "__%s_help() {\n", p.Name)

	if p.Description != "" {
		fmt.Fprintf(b, "    echo %s\n", zshQuote(p.Name+" — "+p.Description))
	} else {
		fmt.Fprintf(b, "    echo %s\n", zshQuote(p.Name))
	}
	b.WriteString("    echo \"\"\n")
	fmt.Fprintf(b, "    echo %s\n", zshQuote("Использование: "+p.Name+" <команда> [аргументы]"))

	if len(p.Commands) > 0 {
		// Ширина колонки имён команд и (отдельно) флагов — по самому длинному
		// имени, для стабильного выравнивания независимо от контекстных условий.
		cmdWidth, flagWidth := 0, 0
		for _, c := range p.Commands {
			if l := len([]rune(c.Name)); l > cmdWidth {
				cmdWidth = l
			}
			for _, f := range c.Flags {
				if l := len([]rune(f.Name)); l > flagWidth {
					flagWidth = l
				}
			}
		}
		cmdFormat := fmt.Sprintf("  %%-%ds  %%s\\n", cmdWidth)
		flagFormat := fmt.Sprintf("      %%-%ds  %%s\\n", flagWidth)

		b.WriteString("    echo \"\"\n")
		fmt.Fprintf(b, "    echo %s\n", zshQuote("Команды:"))
		for _, c := range p.Commands {
			cmdLine := fmt.Sprintf("    printf %s %s %s\n",
				zshQuote(cmdFormat), zshQuote(c.Name), zshQuote(c.Description))
			// Строки флагов печатаются с дополнительным отступом под командой.
			var flagLines strings.Builder
			for _, f := range c.Flags {
				fmt.Fprintf(&flagLines, "    printf %s %s %s\n",
					zshQuote(flagFormat), zshQuote(f.Name), zshQuote(f.Description))
			}
			block := cmdLine + flagLines.String()
			if c.Condition != "" {
				// Всю группу (команда + её флаги) гейтим условием — как и в
				// автодополнении: нет смысла показывать флаги недоступной команды.
				fmt.Fprintf(b, "    if %s; then\n%s    fi\n", c.Condition, block)
			} else {
				b.WriteString(block)
			}
		}
	}

	b.WriteString("}\n\n")
}

// renderCommand генерирует одну ветку case для команды.
func (ZshCompiler) renderCommand(b *strings.Builder, c models.Command) {
	fmt.Fprintf(b, "        %s)\n", zshCasePattern(c.Name))
	if c.Description != "" {
		fmt.Fprintf(b, "            # %s\n", c.Description)
	}

	if len(c.Flags) > 0 {
		// Команда с флагами: первый аргумент трактуется как флаг.
		b.WriteString("            local __flag=\"$1\"\n")
		b.WriteString("            case \"$__flag\" in\n")
		for _, f := range c.Flags {
			fmt.Fprintf(b, "                %s)\n", zshCasePattern(f.Name))
			b.WriteString("                    shift\n")
			if f.Description != "" {
				fmt.Fprintf(b, "                    # %s\n", f.Description)
			}
			writeIndentedBody(b, f.Run, "                    ")
			b.WriteString("                    ;;\n")
		}
		// Ветка по умолчанию — основное действие команды.
		b.WriteString("                *)\n")
		writeIndentedBody(b, commandBody(c), "                    ")
		b.WriteString("                    ;;\n")
		b.WriteString("            esac\n")
	} else {
		writeIndentedBody(b, commandBody(c), "            ")
	}

	b.WriteString("            ;;\n")
}

// renderCompletion генерирует функцию автодополнения и её регистрацию.
// Команды с condition включаются в список дополнений только если условие
// истинно в текущем контексте.
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
				c.Condition, zshQuote(entry))
		} else {
			fmt.Fprintf(b, "            __commands+=(%s)\n", zshQuote(entry))
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
		fmt.Fprintf(b, "                %s)\n", zshCasePattern(c.Name))
		if len(c.Flags) > 0 {
			b.WriteString("                    local -a __flags\n")
			for _, f := range c.Flags {
				entry := fmt.Sprintf("%s:%s", f.Name, f.Description)
				fmt.Fprintf(b, "                    __flags+=(%s)\n", zshQuote(entry))
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

// commandBody возвращает тело команды, при необходимости дополняя его пробросом
// позиционных аргументов "$@".
func commandBody(c models.Command) string {
	body := c.Run
	if c.AcceptsArgs && !strings.Contains(body, "$@") && !strings.Contains(body, "$*") {
		body = strings.TrimRight(body, "\n") + " \"$@\""
	}
	return body
}

// writeIndentedBody печатает многострочный bash-код с заданным отступом.
func writeIndentedBody(b *strings.Builder, body, indent string) {
	body = strings.TrimRight(body, "\n")
	if body == "" {
		fmt.Fprintf(b, "%s:\n", indent) // no-op команда, чтобы ветка была валидной
		return
	}
	for _, line := range strings.Split(body, "\n") {
		fmt.Fprintf(b, "%s%s\n", indent, line)
	}
}

// expandTilde заменяет ведущую тильду на $HOME, чтобы путь корректно
// раскрывался внутри двойных кавычек (где tilde-expansion не работает).
func expandTilde(s string) string {
	if s == "~" {
		return "$HOME"
	}
	if strings.HasPrefix(s, "~/") {
		return "$HOME/" + s[2:]
	}
	return s
}

// zshQuote оборачивает значение в двойные кавычки, экранируя спецсимволы, но
// сохраняя подстановку переменных (${VAR}).
func zshQuote(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `"`, `\"`, "`", "\\`")
	return `"` + r.Replace(s) + `"`
}

// zshCasePattern экранирует имя для использования в качестве паттерна case.
func zshCasePattern(name string) string {
	// Имена команд/флагов состоят из безопасных символов; кавычим на всякий случай.
	return "'" + strings.ReplaceAll(name, "'", `'\''`) + "'"
}
