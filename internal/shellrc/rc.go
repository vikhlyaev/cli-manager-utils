// Package shellrc подключает сгенерированный скрипт к rc-файлу оболочки
// (~/.zshrc / ~/.bashrc): блок между маркерами добавляется один раз.
package shellrc

import (
	"os"
	"path/filepath"
	"strings"

	"cli-manager/internal/fsutil"
)

const (
	beginMarker = "# >>> cli-manager >>>"
	endMarker   = "# <<< cli-manager <<<"
)

// Block возвращает блок подключения скрипта для rc-файла. Путь внутри домашнего
// каталога записывается через $HOME, чтобы rc-файл был переносимым (dotfiles).
func Block(scriptPath string) string {
	p := homeRelative(scriptPath)
	return beginMarker + "\n" +
		"[ -f \"" + p + "\" ] && source \"" + p + "\"\n" +
		endMarker + "\n"
}

// IsInstalled сообщает, подключён ли скрипт в rc-файле: есть блок cli-manager
// или строка, которая упоминает файл скрипта (подключение вручную).
// Отсутствие rc-файла — не ошибка, просто «не подключён».
func IsInstalled(rcPath, scriptPath string) (bool, error) {
	data, err := os.ReadFile(rcPath)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	return mentions(string(data), scriptPath), nil
}

func mentions(rc, scriptPath string) bool {
	if strings.Contains(rc, beginMarker) {
		return true
	}
	// Ручное подключение: `source ~/.local/share/cli-manager/generated.zsh` и т.п.
	tail := filepath.Join(filepath.Base(filepath.Dir(scriptPath)), filepath.Base(scriptPath))
	for _, line := range strings.Split(rc, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "#") {
			continue
		}
		if strings.Contains(line, tail) {
			return true
		}
	}
	return false
}

// Install дописывает блок подключения в конец rc-файла, если скрипт там ещё не
// подключён. Перед изменением прежний файл сохраняется в <rc>.bak. Возвращает
// true, если файл изменён.
func Install(rcPath, scriptPath string) (bool, error) {
	data, err := os.ReadFile(rcPath)
	if err != nil && !os.IsNotExist(err) {
		return false, err
	}
	rc := string(data)
	if mentions(rc, scriptPath) {
		return false, nil
	}
	if err == nil {
		if err := fsutil.WriteFileAtomic(rcPath+".bak", data, 0o644, nil); err != nil {
			return false, err
		}
	}
	if rc != "" && !strings.HasSuffix(rc, "\n") {
		rc += "\n"
	}
	if rc != "" {
		rc += "\n"
	}
	rc += Block(scriptPath)
	if err := fsutil.WriteFileAtomic(rcPath, []byte(rc), 0o644, nil); err != nil {
		return false, err
	}
	return true, nil
}

// homeRelative заменяет домашний каталог в начале пути на $HOME.
func homeRelative(p string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return p
	}
	if p == home {
		return "$HOME"
	}
	if strings.HasPrefix(p, home+string(filepath.Separator)) {
		return "$HOME" + p[len(home):]
	}
	return p
}
