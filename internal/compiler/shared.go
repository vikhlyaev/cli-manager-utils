package compiler

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"cli-manager/internal/models"
)

// Этот файл содержит части генерации, идентичные для всех POSIX-совместимых
// оболочек (zsh/bash): экспорт переменных, шапку префикса, рекурсивный
// диспетчер, дерево help-функций и общий обход для автодополнения. Различается
// между оболочками только shebang и низкоуровневый вывод кандидатов дополнения
// (compadd/_describe против COMPREPLY/compgen) — см. zsh.go / bash.go.

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

// --- Дерево help-функций ------------------------------------------------------

// renderHelpFns рекурсивно генерирует функцию __<base>_help для узла и
// аналогичные функции для всех его ветвей. fnBase — основа имени функции
// (напр. "__av" или "__av_db"); usage — человекочитаемый путь ("av" / "av db");
// title — заголовок (только для корня), для вложенных уровней пустой.
func renderHelpFns(b *strings.Builder, fnBase, usage, title string, cmds []models.Command, flags []models.Flag) {
	fmt.Fprintf(b, "%s_help() {\n", fnBase)
	if title != "" {
		fmt.Fprintf(b, "    echo %s\n", shLiteral(title))
		b.WriteString("    echo \"\"\n")
	}
	fmt.Fprintf(b, "    echo %s\n", shLiteral("Использование: "+usage+" <команда> [аргументы]"))

	if len(cmds) > 0 {
		// Полностью разворачиваем дерево: показываем все подкоманды и флаги на
		// любой глубине, ничего не пряча. Флаги самой ветви — в конце, с тем
		// же отступом, что и её подкоманды.
		rows := collectHelpRows(cmds, 0, "")
		for _, f := range flags {
			rows = append(rows, helpRow{indent: "  ", name: f.Name, desc: f.Description})
		}

		// Колонку описаний выравниваем по самому длинному «отступ + имя» среди
		// всех строк, чтобы описания на любой глубине стояли единым столбцом.
		maxCol := 0
		for _, r := range rows {
			if l := len([]rune(r.indent)) + len([]rune(r.name)); l > maxCol {
				maxCol = l
			}
		}

		b.WriteString("    echo \"\"\n")
		fmt.Fprintf(b, "    echo %s\n", shLiteral("Команды:"))
		for _, r := range rows {
			// Дополняем пробелами здесь, а не через printf %-Ns: оболочки
			// считают ширину в байтах, и кириллица сбивала бы колонку.
			pad := maxCol - len([]rune(r.indent)) - len([]rune(r.name))
			text := r.indent + r.name + strings.Repeat(" ", pad) + "  " + r.desc
			line := fmt.Sprintf("    printf '%%s\\n' %s\n", shLiteral(strings.TrimRight(text, " ")))
			if r.cond != "" {
				fmt.Fprintf(b, "    if %s; then\n%s    fi\n", r.cond, line)
			} else {
				b.WriteString(line)
			}
		}
	}

	b.WriteString("}\n\n")

	// Рекурсивно генерируем help-функции вложенных ветвей.
	for _, c := range cmds {
		if c.IsBranch() {
			renderHelpFns(b, fnBase+"_"+sanitizeIdent(c.Name), usage+" "+c.Name, "", c.Subcommands, c.Flags)
		}
	}
}

// helpRow — одна строка справки: ведущий отступ, имя элемента, описание и
// (опционально) условие показа, унаследованное от родительских ветвей.
type helpRow struct {
	indent string
	name   string
	desc   string
	cond   string
}

// collectHelpRows рекурсивно разворачивает дерево команд в плоский список строк
// справки. level — глубина (0 для команд верхнего уровня), parentCond —
// накопленное условие показа родителя. Для ветвей вглубь идут подкоманды, для
// любых узлов — их флаги (у ветви — после подкоманд).
func collectHelpRows(cmds []models.Command, level int, parentCond string) []helpRow {
	var rows []helpRow
	indent := strings.Repeat(" ", 2+4*level)
	childIndent := strings.Repeat(" ", 2+4*(level+1))
	for _, c := range cmds {
		cond := combineConditions(parentCond, c.Condition)
		name := c.Name
		if u := strings.TrimSpace(c.Usage); u != "" {
			name += " " + u
		}
		rows = append(rows, helpRow{indent: indent, name: name, desc: c.Description, cond: cond})
		if c.IsBranch() {
			rows = append(rows, collectHelpRows(c.Subcommands, level+1, cond)...)
		}
		for _, f := range c.Flags {
			rows = append(rows, helpRow{indent: childIndent, name: f.Name, desc: f.Description, cond: cond})
		}
	}
	return rows
}

// combineConditions объединяет условие родителя и собственное условие команды
// через "&&"; пустые условия опускаются.
func combineConditions(parent, own string) string {
	parent, own = strings.TrimSpace(parent), strings.TrimSpace(own)
	switch {
	case parent == "":
		return own
	case own == "":
		return parent
	default:
		return parent + " && " + own
	}
}

// --- Рекурсивный диспетчер ----------------------------------------------------

// renderDispatcher генерирует функцию-диспетчер префикса с рекурсивным разбором
// вложенных подкоманд.
func renderDispatcher(b *strings.Builder, p models.Prefix) {
	// Форма `function name {`, а не `name() {`: если у пользователя есть alias с
	// тем же именем, `name() {` раскрывается как alias и ломает весь скрипт.
	fmt.Fprintf(b, "function %s {\n", p.Name)
	renderDispatchLevel(b, p.Commands, nil, 1, "__"+p.Name, p.Name, "", "    ")
	b.WriteString("}\n\n")
}

// renderDispatchLevel генерирует разбор одного уровня: чтение очередного токена,
// case по командам этого уровня со справкой и веткой по умолчанию.
//
//	cmds     — команды этого уровня
//	flags    — флаги узла-родителя (у ветви могут быть и подкоманды, и флаги)
//	depth    — глубина (для уникального имени переменной __arg<depth>)
//	fnBase   — основа имени help-функции уровня
//	usage    — человекочитаемый путь до уровня
//	fallback — действие узла-родителя при нераспознанной подкоманде (может быть "")
//	indent   — отступ операторов внутри функции
func renderDispatchLevel(b *strings.Builder, cmds []models.Command, flags []models.Flag, depth int, fnBase, usage, fallback, indent string) {
	argVar := fmt.Sprintf("__arg%d", depth)
	fmt.Fprintf(b, "%slocal %s=\"$1\"\n", indent, argVar)
	fmt.Fprintf(b, "%sshift 2>/dev/null\n\n", indent)
	fmt.Fprintf(b, "%scase \"$%s\" in\n", indent, argVar)

	// Пустой ввод или запрос справки — показываем справку уровня. Если среди
	// команд есть собственная команда "help", не затеняем её.
	hasHelpCmd := false
	for _, c := range cmds {
		if c.Name == "help" {
			hasHelpCmd = true
			break
		}
	}
	helpPattern := `-h|--help`
	if !hasHelpCmd {
		helpPattern = `-h|--help|help`
	}
	if strings.TrimSpace(fallback) != "" {
		// У группы есть своё действие: без подкоманды выполняем его
		// (`av mb` делает cd), а справка — только по -h/--help.
		fmt.Fprintf(b, "%s    \"\")\n", indent)
		writeIndentedBody(b, fallback, indent+"        ")
		fmt.Fprintf(b, "%s        ;;\n", indent)
	} else {
		helpPattern = `""|` + helpPattern
	}
	fmt.Fprintf(b, "%s    %s)\n", indent, helpPattern)
	fmt.Fprintf(b, "%s        %s_help\n", indent, fnBase)
	fmt.Fprintf(b, "%s        return 0\n", indent)
	fmt.Fprintf(b, "%s        ;;\n", indent)

	for _, c := range cmds {
		renderArm(b, c, depth, fnBase, usage, fallback, indent+"    ")
	}
	// Флаги ветви: токен уже снят shift-ом, поэтому $1 — первый аргумент флага.
	for _, f := range flags {
		fmt.Fprintf(b, "%s    %s)\n", indent, shCasePattern(f.Name))
		if f.Description != "" {
			fmt.Fprintf(b, "%s        # %s\n", indent, f.Description)
		}
		if f.AfterCommand {
			writeRunFirst(b, fallback, indent+"        ")
		}
		writeIndentedBody(b, f.Run, indent+"        ")
		fmt.Fprintf(b, "%s        ;;\n", indent)
	}

	// Ветка по умолчанию: действие родителя (если задано) либо ошибка + справка.
	fmt.Fprintf(b, "%s    *)\n", indent)
	if strings.TrimSpace(fallback) != "" {
		// Возвращаем съеденный токен в позиционные параметры для действия.
		fmt.Fprintf(b, "%s        set -- \"$%s\" \"$@\"\n", indent, argVar)
		writeIndentedBody(b, fallback, indent+"        ")
	} else {
		fmt.Fprintf(b, "%s        echo \"%s: неизвестная команда: $%s\" >&2\n", indent, usage, argVar)
		fmt.Fprintf(b, "%s        %s_help >&2\n", indent, fnBase)
		fmt.Fprintf(b, "%s        return 1\n", indent)
	}
	fmt.Fprintf(b, "%s        ;;\n", indent)
	fmt.Fprintf(b, "%sesac\n", indent)
}

// renderArm генерирует одну ветку case для команды: ветвь рекурсивно разбирает
// подкоманды, лист исполняет тело (с флагами или без). parentRun — действие
// родительской команды (для after_command), может быть "".
func renderArm(b *strings.Builder, c models.Command, depth int, parentFnBase, usage, parentRun, indent string) {
	fmt.Fprintf(b, "%s%s)\n", indent, shCasePattern(c.Name))
	if c.Description != "" {
		fmt.Fprintf(b, "%s    # %s\n", indent, c.Description)
	}
	// Без флагов аргументы проверяем до действия родителя: при ошибке ввода
	// не должно быть побочных эффектов вроде cd.
	simpleLeaf := !c.IsBranch() && len(c.Flags) == 0
	if simpleLeaf {
		writeArgsCheck(b, c, usage+" "+c.Name, indent+"    ")
	}
	if c.AfterCommand {
		writeRunFirst(b, parentRun, indent+"    ")
	}

	if c.IsBranch() {
		fnBase := parentFnBase + "_" + sanitizeIdent(c.Name)
		renderDispatchLevel(b, c.Subcommands, c.Flags, depth+1, fnBase, usage+" "+c.Name, c.Run, indent+"    ")
	} else {
		renderLeafBody(b, c, usage+" "+c.Name, indent+"    ")
	}

	fmt.Fprintf(b, "%s    ;;\n", indent)
}

// writeRunFirst печатает действие, которое нужно выполнить перед основным
// (after_command): в текущей оболочке, чтобы сработал cd; при ошибке
// основное действие не запускается и возвращается её код.
func writeRunFirst(b *strings.Builder, run, indent string) {
	if strings.TrimSpace(run) == "" {
		return
	}
	fmt.Fprintf(b, "%s{\n", indent)
	writeIndentedBody(b, run, indent+"    ")
	fmt.Fprintf(b, "%s} || return $?\n", indent)
}

// writeArgsCheck печатает проверку обязательных аргументов из usage: если их
// меньше, чем <…> в описании, печатает подсказку и возвращает 2.
func writeArgsCheck(b *strings.Builder, c models.Command, path, indent string) {
	n := requiredArgs(c.Usage)
	if n == 0 {
		return
	}
	msg := path + ": не хватает аргументов. Использование: " + path + " " + strings.TrimSpace(c.Usage)
	fmt.Fprintf(b, "%sif (( $# < %d )); then\n", indent, n)
	fmt.Fprintf(b, "%s    echo %s >&2\n", indent, shLiteral(msg))
	fmt.Fprintf(b, "%s    return 2\n", indent)
	fmt.Fprintf(b, "%sfi\n", indent)
}

// requiredArgs считает обязательные аргументы в usage — токены вида <имя>
// вне квадратных скобок ([…] — необязательные, напр. [<строки>]).
func requiredArgs(usage string) int {
	usage = optionalArgRe.ReplaceAllString(usage, "")
	return len(requiredArgRe.FindAllString(usage, -1))
}

var (
	requiredArgRe = regexp.MustCompile(`<[^<>]+>`)
	optionalArgRe = regexp.MustCompile(`\[[^\]]*\]`)
)

// renderLeafBody генерирует тело листовой команды: подменю флагов (первый
// аргумент трактуется как флаг) либо просто действие. path — полный путь
// команды для сообщений ("av mb loc").
func renderLeafBody(b *strings.Builder, c models.Command, path string, indent string) {
	if len(c.Flags) > 0 {
		fmt.Fprintf(b, "%slocal __flag=\"$1\"\n", indent)
		fmt.Fprintf(b, "%scase \"$__flag\" in\n", indent)
		for _, f := range c.Flags {
			fmt.Fprintf(b, "%s    %s)\n", indent, shCasePattern(f.Name))
			fmt.Fprintf(b, "%s        shift\n", indent)
			if f.Description != "" {
				fmt.Fprintf(b, "%s        # %s\n", indent, f.Description)
			}
			if f.AfterCommand {
				writeRunFirst(b, c.Run, indent+"        ")
			}
			writeIndentedBody(b, f.Run, indent+"        ")
			fmt.Fprintf(b, "%s        ;;\n", indent)
		}
		fmt.Fprintf(b, "%s    *)\n", indent)
		writeArgsCheck(b, c, path, indent+"        ")
		writeIndentedBody(b, commandBody(c), indent+"        ")
		fmt.Fprintf(b, "%s        ;;\n", indent)
		fmt.Fprintf(b, "%s    esac\n", indent)
	} else {
		// Проверка аргументов уже напечатана в renderArm.
		writeIndentedBody(b, commandBody(c), indent)
	}
}

// --- Автодополнение (общий обход дерева) --------------------------------------

// completionEmitter абстрагирует вывод кандидатов для конкретной оболочки.
type completionEmitter interface {
	// emitCommands печатает ветку case по пути path, предлагающую дочерние
	// команды cmds (с учётом контекстных условий) и флаги ветви flags.
	emitCommands(b *strings.Builder, path string, cmds []models.Command, flags []models.Flag)
	// emitLeaf печатает ветку case по пути path для листовой команды c
	// (флаги и/или файлы).
	emitLeaf(b *strings.Builder, path string, c models.Command)
}

// renderCompletionTree обходит дерево команд и для каждого узла-ветви печатает
// ветку с дочерними командами, а для каждого листа с флагами/аргументами — ветку
// с его дополнениями. Путь узла (через пробел) служит ключом ветки case.
func renderCompletionTree(b *strings.Builder, cmds []models.Command, flags []models.Flag, path string, e completionEmitter) {
	e.emitCommands(b, path, cmds, flags)
	for _, c := range cmds {
		childPath := c.Name
		if path != "" {
			childPath = path + " " + c.Name
		}
		if c.IsBranch() {
			renderCompletionTree(b, c.Subcommands, c.Flags, childPath, e)
		} else if len(c.Flags) > 0 || c.AcceptsArgs {
			e.emitLeaf(b, childPath, c)
		}
	}
}

// --- Общие низкоуровневые помощники -------------------------------------------

// sanitizeIdent превращает имя команды в безопасный фрагмент имени shell-функции.
func sanitizeIdent(s string) string {
	return strings.NewReplacer("-", "_", " ", "_").Replace(s)
}

// commandBody возвращает тело команды, при необходимости дополняя его пробросом
// позиционных аргументов "$@". Если run сам обращается к аргументам ($1, ${2},
// $@, $*, $#), ничего не дописываем — аргументы уже подставлены, где нужно.
func commandBody(c models.Command) string {
	body := c.Run
	if c.AcceptsArgs && !usesArgs(body) {
		body = strings.TrimRight(body, "\n") + " \"$@\""
	}
	return body
}

// usesArgs сообщает, обращается ли код к позиционным аргументам.
func usesArgs(body string) bool { return positionalRe.MatchString(body) }

var positionalRe = regexp.MustCompile(`\$(\{#?)?[1-9@*#]`)

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

// shLiteral оборачивает текст в одинарные кавычки: никаких подстановок. Для
// справки и описаний в автодополнении — там `$(...)` не должен выполняться.
func shLiteral(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// shCasePattern экранирует имя/путь для использования в качестве паттерна case.
func shCasePattern(name string) string {
	return "'" + strings.ReplaceAll(name, "'", `'\''`) + "'"
}

// --- Выключенные элементы ---------------------------------------------------

// enabledOnly возвращает копию конфигурации без выключенных префиксов, команд и
// флагов. Генерация работает только с ней, поэтому остальной код о Disabled не
// знает.
func enabledOnly(cfg *models.Config) *models.Config {
	out := *cfg
	out.Prefixes = nil
	for _, p := range cfg.Prefixes {
		if !p.Disabled {
			p.Commands = enabledCommands(p.Commands)
			out.Prefixes = append(out.Prefixes, p)
		}
	}
	return &out
}

func enabledCommands(cmds []models.Command) []models.Command {
	var out []models.Command
	for _, c := range cmds {
		if c.Disabled {
			continue
		}
		var flags []models.Flag
		for _, f := range c.Flags {
			if !f.Disabled {
				flags = append(flags, f)
			}
		}
		c.Flags = flags
		if c.IsBranch() {
			c.Subcommands = enabledCommands(c.Subcommands)
			if len(c.Subcommands) == 0 {
				// Все подкоманды выключены: ветвь целиком пропадает, а не
				// превращается в лист с чужими флагами и действием по умолчанию.
				continue
			}
		}
		out = append(out, c)
	}
	return out
}

// disabledPrefixNote — текст предпросмотра выключенного префикса.
const disabledPrefixNote = "# Префикс выключен — в скрипт не попадает.\n"

// --- Авто-перезагрузка ---------------------------------------------------------

// genPlaceholder заменяется на идентификатор поколения скрипта (хеш его текста).
const genPlaceholder = "__CLI_MANAGER_GEN_PLACEHOLDER__"

// StampPath — файл рядом со скриптом, где лежит идентификатор последнего
// записанного поколения. Хук в открытых терминалах сравнивает его со своим.
func StampPath(scriptPath string) string { return scriptPath + ".stamp" }

// renderReloadHook печатает функцию __cli_manager_reload: перед каждым
// приглашением она читает stamp-файл и, если скрипт перегенерирован, снимает
// функции прежних префиксов (чтобы удалённые исчезли) и заново подключает
// скрипт. register — код регистрации хука для конкретной оболочки.
//
// cleanup — код снятия остатков префикса $__p (help-функций и регистрации
// автодополнения), свой для каждой оболочки.
func renderReloadHook(b *strings.Builder, scriptPath string, prefixes []models.Prefix, cleanup, register string) {
	names := make([]string, len(prefixes))
	for i, p := range prefixes {
		names[i] = p.Name
	}
	b.WriteString("# --- Авто-перезагрузка в открытых терминалах ---\n")
	fmt.Fprintf(b, "__CLI_MANAGER_SCRIPT=%s\n", shQuote(scriptPath))
	fmt.Fprintf(b, "__CLI_MANAGER_GEN=%s\n", shQuote(genPlaceholder))
	fmt.Fprintf(b, "__CLI_MANAGER_PREFIXES=(%s)\n", strings.Join(names, " "))
	// Хук сохраняет код возврата последней команды ($?): следующие хуки
	// приглашения (starship, bash-preexec, вывод кода в prompt) должны видеть его.
	b.WriteString(`__cli_manager_reload() {
    local __rc=$? __stamp="$__CLI_MANAGER_SCRIPT.stamp" __gen __p
    [[ -r "$__stamp" && -r "$__CLI_MANAGER_SCRIPT" ]] || return $__rc
    read -r __gen < "$__stamp"
    [[ -z "$__gen" || "$__gen" == "$__CLI_MANAGER_GEN" ]] && return $__rc
    for __p in "${__CLI_MANAGER_PREFIXES[@]}"; do
        unset -f "$__p" "_${__p}_completions" 2>/dev/null
`)
	b.WriteString(cleanup)
	b.WriteString(`    done
    source "$__CLI_MANAGER_SCRIPT"
    return $__rc
}
`)
	b.WriteString(register)
}

// stampScript подставляет в скрипт идентификатор поколения — короткий хеш
// текста. Он детерминирован: одинаковый конфиг даёт одинаковый скрипт, и
// открытые терминалы не перезагружают его зря.
func stampScript(script string) (string, string) {
	sum := sha256.Sum256([]byte(script))
	gen := hex.EncodeToString(sum[:8])
	return strings.Replace(script, genPlaceholder, gen, 1), gen
}
