package compiler

import (
	"fmt"
	"sort"
	"strings"

	"cli-manager/internal/models"
)

// Этот файл содержит части генерации, идентичные для всех POSIX-совместимых
// оболочек (zsh/bash): экспорт переменных, функция-диспетчер, справка и ветки
// команд. Различается между оболочками только блок автодополнения и shebang —
// они живут в zsh.go / bash.go.

// renderVariables экспортирует глобальные переменные. Ключи сортируются для
// детерминированного вывода.
func renderVariables(b *strings.Builder, vars map[string]string) {
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
		fmt.Fprintf(b, "export %s=%s\n", k, shQuote(expandTilde(vars[k])))
	}
	b.WriteString("\n")
}

// renderPrefixHeader печатает шапку-комментарий префикса.
func renderPrefixHeader(b *strings.Builder, p models.Prefix) {
	b.WriteString("# ─────────────────────────────────────────────\n")
	if p.Description != "" {
		fmt.Fprintf(b, "# Префикс: %s — %s\n", p.Name, p.Description)
	} else {
		fmt.Fprintf(b, "# Префикс: %s\n", p.Name)
	}
	b.WriteString("# ─────────────────────────────────────────────\n\n")
}

// renderDispatcher генерирует функцию-диспетчер: case по первому аргументу с
// ветками команд и справкой. Синтаксис (local/shift/case/printf) совместим с
// zsh и bash.
func renderDispatcher(b *strings.Builder, p models.Prefix) {
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
		renderCommand(b, c)
	}

	b.WriteString("        *)\n")
	fmt.Fprintf(b, "            echo \"%s: неизвестная команда: $__cmd\" >&2\n", p.Name)
	fmt.Fprintf(b, "            __%s_help >&2\n", p.Name)
	b.WriteString("            return 1\n")
	b.WriteString("            ;;\n")
	b.WriteString("    esac\n")
	b.WriteString("}\n\n")
}

// renderHelp генерирует функцию __<name>_help, печатающую заголовок префикса и
// список всех команд с описаниями. Команды с condition попадают в справку только
// если их условие истинно в текущем контексте (как и в автодополнении).
func renderHelp(b *strings.Builder, p models.Prefix) {
	fmt.Fprintf(b, "__%s_help() {\n", p.Name)

	if p.Description != "" {
		fmt.Fprintf(b, "    echo %s\n", shQuote(p.Name+" — "+p.Description))
	} else {
		fmt.Fprintf(b, "    echo %s\n", shQuote(p.Name))
	}
	b.WriteString("    echo \"\"\n")
	fmt.Fprintf(b, "    echo %s\n", shQuote("Использование: "+p.Name+" <команда> [аргументы]"))

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
		fmt.Fprintf(b, "    echo %s\n", shQuote("Команды:"))
		for _, c := range p.Commands {
			cmdLine := fmt.Sprintf("    printf %s %s %s\n",
				shQuote(cmdFormat), shQuote(c.Name), shQuote(c.Description))
			// Строки флагов печатаются с дополнительным отступом под командой.
			var flagLines strings.Builder
			for _, f := range c.Flags {
				fmt.Fprintf(&flagLines, "    printf %s %s %s\n",
					shQuote(flagFormat), shQuote(f.Name), shQuote(f.Description))
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
func renderCommand(b *strings.Builder, c models.Command) {
	fmt.Fprintf(b, "        %s)\n", shCasePattern(c.Name))
	if c.Description != "" {
		fmt.Fprintf(b, "            # %s\n", c.Description)
	}

	if len(c.Flags) > 0 {
		// Команда с флагами: первый аргумент трактуется как флаг.
		b.WriteString("            local __flag=\"$1\"\n")
		b.WriteString("            case \"$__flag\" in\n")
		for _, f := range c.Flags {
			fmt.Fprintf(b, "                %s)\n", shCasePattern(f.Name))
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

// shQuote оборачивает значение в двойные кавычки, экранируя спецсимволы, но
// сохраняя подстановку переменных (${VAR}). Совместимо с zsh и bash.
func shQuote(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `"`, `\"`, "`", "\\`")
	return `"` + r.Replace(s) + `"`
}

// shCasePattern экранирует имя для использования в качестве паттерна case.
func shCasePattern(name string) string {
	// Имена команд/флагов состоят из безопасных символов; кавычим на всякий случай.
	return "'" + strings.ReplaceAll(name, "'", `'\''`) + "'"
}
