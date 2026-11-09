package validation

import (
	"fmt"
	"sort"

	"cli-manager/internal/models"
)

// Problem — одна проблема конфигурации: где она (путь в дереве, напр.
// "av db migrate" или "переменная FOO") и что не так.
type Problem struct {
	Path string
	Msg  string
}

func (p Problem) String() string { return p.Path + ": " + p.Msg }

// ValidateConfig проверяет всю конфигурацию целиком — так же строго, как формы
// TUI: формат имён, дубли в пределах родителя и синтаксис условий. Нужна для
// конфигов, отредактированных вручную: иначе битое имя молча уходит в генератор.
func ValidateConfig(cfg *models.Config) []Problem {
	var ps []Problem
	add := func(path, format string, args ...any) {
		ps = append(ps, Problem{Path: path, Msg: fmt.Sprintf(format, args...)})
	}

	for _, k := range sortedVarKeys(cfg.Variables) {
		if err := ValidateIdentifier(k); err != nil {
			add("переменная "+k, "%v", err)
		}
	}

	seen := map[string]bool{}
	for _, p := range cfg.Prefixes {
		path := "префикс " + p.Name
		if err := ValidateIdentifier(p.Name); err != nil {
			add(path, "%v", err)
		}
		if seen[p.Name] {
			add(path, "префикс с таким именем уже есть")
		}
		seen[p.Name] = true
		ps = append(ps, validateCommands(p.Name, p.Commands)...)
	}
	return ps
}

// validateCommands рекурсивно проверяет команды одного уровня; path — путь
// родителя ("av" или "av db").
func validateCommands(path string, cmds []models.Command) []Problem {
	var ps []Problem
	seen := map[string]bool{}
	for _, c := range cmds {
		cp := path + " " + c.Name
		if err := ValidateCommandName(c.Name); err != nil {
			ps = append(ps, Problem{cp, err.Error()})
		}
		if seen[c.Name] {
			ps = append(ps, Problem{cp, "команда с таким именем уже есть на этом уровне"})
		}
		seen[c.Name] = true
		if err := CheckCondition(c.Condition); err != nil {
			ps = append(ps, Problem{cp, "ошибка в условии: " + err.Error()})
		}

		subs := map[string]bool{}
		for _, sc := range c.Subcommands {
			subs[sc.Name] = true
		}
		flags := map[string]bool{}
		for _, f := range c.Flags {
			fp := cp + " " + f.Name
			if err := ValidateFlagName(f.Name); err != nil {
				ps = append(ps, Problem{fp, err.Error()})
			}
			if flags[f.Name] {
				ps = append(ps, Problem{fp, "флаг с таким именем уже есть"})
			}
			if subs[f.Name] {
				ps = append(ps, Problem{fp, "имя флага совпадает с подкомандой — сработает подкоманда"})
			}
			flags[f.Name] = true
		}
		ps = append(ps, validateCommands(cp, c.Subcommands)...)
	}
	return ps
}

func sortedVarKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
