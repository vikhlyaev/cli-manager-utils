// Package importer извлекает определения alias из rc-файлов оболочки, чтобы
// импортировать их в конфигурацию cli-manager.
package importer

import (
	"bufio"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Alias — разобранное определение alias (имя и тело).
type Alias struct {
	Name  string
	Value string
}

// ZshrcPath возвращает путь к .zshrc: в $ZDOTDIR, если он задан, иначе в ~.
func ZshrcPath() string {
	if dir := os.Getenv("ZDOTDIR"); dir != "" {
		return filepath.Join(dir, ".zshrc")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".zshrc")
}

// RCPath возвращает rc-файл интерактивной оболочки: ~/.bashrc для bash,
// иначе ~/.zshrc.
func RCPath(shell string) string {
	if shell == "bash" {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, ".bashrc")
	}
	return ZshrcPath()
}

// LoadAliases читает ~/.zshrc и разбирает определения alias. Отсутствие файла —
// не ошибка: возвращается пустой список.
func LoadAliases() ([]Alias, error) {
	return LoadAliasesFrom(ZshrcPath())
}

// LoadAliasesFrom разбирает alias из rc-файла path. Отсутствие файла — не ошибка.
func LoadAliasesFrom(path string) ([]Alias, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()
	return ParseAliases(f), nil
}

// ParseAliases извлекает определения alias из содержимого rc-файла. Поддерживает
// одинарные/двойные кавычки и значения без кавычек; флаги вида `alias -g` —
// отбрасываются. Строки-комментарии и многострочные определения игнорируются.
func ParseAliases(r io.Reader) []Alias {
	var out []Alias
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if !strings.HasPrefix(line, "alias ") {
			continue
		}
		rest := strings.TrimSpace(strings.TrimPrefix(line, "alias "))

		// Отбрасываем флаги (`-g`, `-s` и т.п.) перед именем.
		for strings.HasPrefix(rest, "-") {
			parts := strings.SplitN(rest, " ", 2)
			if len(parts) < 2 {
				rest = ""
				break
			}
			rest = strings.TrimSpace(parts[1])
		}

		eq := strings.Index(rest, "=")
		if eq <= 0 {
			continue
		}
		name := strings.TrimSpace(rest[:eq])
		value := unquote(strings.TrimSpace(rest[eq+1:]))
		if name == "" || value == "" {
			continue
		}
		out = append(out, Alias{Name: name, Value: value})
	}
	return out
}

// unquote снимает обрамляющие одинарные/двойные кавычки и минимально
// разэкранирует содержимое двойных кавычек.
func unquote(s string) string {
	if len(s) >= 2 {
		first, last := s[0], s[len(s)-1]
		if first == last && (first == '\'' || first == '"') {
			inner := s[1 : len(s)-1]
			if first == '"' {
				inner = strings.ReplaceAll(inner, `\"`, `"`)
			}
			return inner
		}
	}
	return s
}
