package cli

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"cli-manager/internal/compiler"
	"cli-manager/internal/importer"
	"cli-manager/internal/models"
	"cli-manager/internal/shellrc"
	"cli-manager/internal/storage"
	"cli-manager/internal/validation"
)

// report накапливает результаты проверок doctor.
type report struct {
	w      io.Writer
	failed bool
}

func (r *report) ok(format string, a ...any)   { fmt.Fprintf(r.w, "✓ "+format+"\n", a...) }
func (r *report) warn(format string, a ...any) { fmt.Fprintf(r.w, "⚠ "+format+"\n", a...) }
func (r *report) fail(format string, a ...any) {
	r.failed = true
	fmt.Fprintf(r.w, "✗ "+format+"\n", a...)
}

// runDoctor проверяет установку: конфиг, скрипт, подключение к оболочке,
// автодополнение и коллизии имён. Код выхода 1, если есть ошибки (✗);
// предупреждения (⚠) на код не влияют.
func runDoctor(_ []string, s IO) int {
	r := &report{w: s.Out}

	cfgPath := storage.ConfigPath()
	cfg, err := storage.LoadConfig()
	if err != nil {
		r.fail("конфиг %s не читается: %v", cfgPath, err)
		return 1
	}
	if _, err := os.Stat(cfgPath); os.IsNotExist(err) {
		r.warn("конфига %s ещё нет — создайте префиксы в TUI (cli-manager)", cfgPath)
	} else {
		r.ok("конфиг: %s", cfgPath)
	}

	if ps := validation.ValidateConfig(cfg); len(ps) > 0 {
		for _, p := range ps {
			r.fail("конфиг: %s", p)
		}
	} else {
		r.ok("конфиг без ошибок (префиксов: %d)", len(cfg.Prefixes))
	}

	shell := shellOf(cfg.Shell)
	if _, err := exec.LookPath(shell); err != nil {
		r.fail("оболочка %s не найдена в PATH", shell)
	} else {
		r.ok("целевая оболочка: %s", shell)
	}

	script := compiler.For(cfg.Shell).OutputPath()
	checkScriptFile(r, cfgPath, script)

	rc := importer.RCPath(shell)
	checkRC(r, rc, script, shell)
	checkCollisions(r, cfg.Prefixes, rc)

	if r.failed {
		return 1
	}
	return 0
}

// checkScriptFile: скрипт существует и не старше конфига.
func checkScriptFile(r *report, cfgPath, script string) {
	si, err := os.Stat(script)
	if err != nil {
		r.fail("скрипт %s не найден — выполните: cli-manager generate", script)
		return
	}
	if ci, err := os.Stat(cfgPath); err == nil && ci.ModTime().After(si.ModTime()) {
		r.warn("скрипт старше конфига (конфиг правили вручную?) — выполните: cli-manager generate")
		return
	}
	r.ok("скрипт: %s", script)
}

// checkRC: скрипт подключён в rc-файле, а для zsh — инициализирован compinit
// и подключение стоит после него (иначе compdef ещё не определена).
func checkRC(r *report, rc, script, shell string) {
	ok, err := shellrc.IsInstalled(rc, script)
	switch {
	case err != nil:
		r.fail("не удалось прочитать %s: %v", rc, err)
		return
	case !ok:
		r.fail("скрипт не подключён в %s — выполните: cli-manager init", rc)
		return
	}
	r.ok("скрипт подключён в %s", rc)

	if shell != "zsh" {
		return
	}
	data, _ := os.ReadFile(rc)
	text := string(data)
	ci := strings.Index(text, "compinit")
	si := strings.Index(text, "cli-manager")
	switch {
	case ci < 0:
		r.warn("в %s нет compinit — автодополнение не заработает (добавьте: autoload -Uz compinit && compinit)", rc)
	case si >= 0 && si < ci:
		r.warn("в %s скрипт подключается раньше compinit — перенесите подключение ниже, иначе нет автодополнения", rc)
	default:
		r.ok("автодополнение: compinit найден")
	}
}

// checkCollisions: имя префикса не перекрывает системную команду или alias.
func checkCollisions(r *report, prefixes []models.Prefix, rc string) {
	aliases := map[string]bool{}
	if as, err := importer.LoadAliasesFrom(rc); err == nil {
		for _, a := range as {
			aliases[a.Name] = true
		}
	}
	clean := true
	for _, p := range prefixes {
		if p.Disabled {
			continue // выключенный префикс ничего не перекрывает
		}
		if aliases[p.Name] {
			clean = false
			r.warn("префикс %s совпадает с alias из %s", p.Name, rc)
		}
		if validation.CheckCollision(p.Name) {
			clean = false
			r.warn("префикс %s перекрывает команду %s", p.Name, commandPath(p.Name))
		}
	}
	if clean {
		r.ok("коллизий имён префиксов нет")
	}
}

func commandPath(name string) string {
	if p, err := exec.LookPath(name); err == nil {
		return p
	}
	return "оболочки"
}
