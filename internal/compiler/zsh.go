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

// renderPrefix генерирует шапку, дерево справок, рекурсивный диспетчер (общие) и
// zsh-специфичное автодополнение.
func (z ZshCompiler) renderPrefix(b *strings.Builder, p models.Prefix) {
	renderPrefixHeader(b, p)
	title := p.Name
	if p.Description != "" {
		title = p.Name + " — " + p.Description
	}
	renderHelpFns(b, "__"+p.Name, p.Name, title, p.Commands)
	renderDispatcher(b, p)
	z.renderCompletion(b, p)
}

// renderCompletion генерирует функцию автодополнения через обход дерева команд.
// Текущий уровень определяется путём уже введённых слов ($words[2,CURRENT-1]),
// поэтому поддерживается произвольная вложенность подкоманд. Команды с condition
// предлагаются только если условие истинно.
func (ZshCompiler) renderCompletion(b *strings.Builder, p models.Prefix) {
	fmt.Fprintf(b, "_%s_completions() {\n", p.Name)
	b.WriteString("    local __path=\"${(j: :)words[2,CURRENT-1]}\"\n")
	b.WriteString("    case \"$__path\" in\n")
	renderCompletionTree(b, p.Commands, "", zshEmitter{})
	b.WriteString("    esac\n")
	b.WriteString("}\n")

	// Регистрируем автодополнение только если система completion загружена
	// (compdef доступна) — иначе source в неинтерактивном zsh упадёт с ошибкой.
	fmt.Fprintf(b, "if typeset -f compdef >/dev/null; then compdef _%s_completions %s; fi\n\n",
		p.Name, p.Name)
}

// zshEmitter выводит кандидатов автодополнения средствами zsh (_describe/_files).
type zshEmitter struct{}

func (zshEmitter) emitCommands(b *strings.Builder, path string, cmds []models.Command) {
	fmt.Fprintf(b, "        %s)\n", shCasePattern(path))
	b.WriteString("            local -a __c\n")
	for _, c := range cmds {
		entry := fmt.Sprintf("%s:%s", c.Name, c.Description)
		if c.Condition != "" {
			fmt.Fprintf(b, "            if %s; then __c+=(%s); fi\n", c.Condition, shQuote(entry))
		} else {
			fmt.Fprintf(b, "            __c+=(%s)\n", shQuote(entry))
		}
	}
	b.WriteString("            _describe -t commands 'команда' __c\n")
	b.WriteString("            ;;\n")
}

func (zshEmitter) emitLeaf(b *strings.Builder, path string, c models.Command) {
	fmt.Fprintf(b, "        %s)\n", shCasePattern(path))
	if len(c.Flags) > 0 {
		b.WriteString("            local -a __f\n")
		for _, f := range c.Flags {
			entry := fmt.Sprintf("%s:%s", f.Name, f.Description)
			fmt.Fprintf(b, "            __f+=(%s)\n", shQuote(entry))
		}
		b.WriteString("            _describe -t flags 'флаг' __f\n")
	}
	if c.AcceptsArgs {
		b.WriteString("            _files\n")
	}
	b.WriteString("            ;;\n")
}
