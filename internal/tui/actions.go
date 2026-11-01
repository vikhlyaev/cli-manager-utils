package tui

import (
	"fmt"
	"slices"

	"github.com/charmbracelet/bubbles/list"
	"gopkg.in/yaml.v3"

	"cli-manager/internal/compiler"
	"cli-manager/internal/importer"
	"cli-manager/internal/models"
	"cli-manager/internal/storage"
	"cli-manager/internal/validation"
)

// Все изменения конфигурации идут через mutate: снимок для undo → изменение →
// сохранение на диск и перегенерация скрипта.

func (m *model) setStatus(msg string) { m.status, m.statusErr = msg, false }
func (m *model) setError(msg string)  { m.status, m.statusErr = msg, true }

// mutate применяет изменение fn к конфигурации с поддержкой undo и сохраняет.
func (m *model) mutate(fn func()) {
	m.mutateIf(func() bool { fn(); return true })
}

// mutateIf — как mutate, но fn сообщает, изменила ли она что-то. Если нет,
// снимок снимается с undo-стека и сохранения не происходит.
func (m *model) mutateIf(fn func() bool) {
	m.pushUndo()
	if !fn() {
		m.undoStack = m.undoStack[:len(m.undoStack)-1]
		return
	}
	m.persist()
}

// persist сохраняет конфигурацию и регенерирует скрипт.
func (m *model) persist() {
	if m.saveAndGenerate() {
		m.setStatus("Сохранено и скомпилировано ✓")
	}
}

// saveAndGenerate пишет конфиг и скрипт; при ошибке выставляет статус и
// возвращает false.
func (m *model) saveAndGenerate() bool {
	if err := storage.SaveConfig(m.cfg); err != nil {
		m.setError("Ошибка сохранения: " + err.Error())
		return false
	}
	if err := compiler.For(m.cfg.Shell).Generate(m.cfg); err != nil {
		m.setError("Ошибка генерации " + m.shellName() + ": " + err.Error())
		return false
	}
	return true
}

// pushUndo кладёт снимок текущей конфигурации в undo-стек.
func (m *model) pushUndo() {
	snap, err := storage.CloneConfig(m.cfg)
	if err != nil {
		return
	}
	m.undoStack = append(m.undoStack, snap)
	if len(m.undoStack) > undoLimit {
		m.undoStack = m.undoStack[len(m.undoStack)-undoLimit:]
	}
}

// undo откатывает последнюю мутацию: восстанавливает снимок, обновляет экраны,
// сохраняет его на диск и перегенерирует скрипт.
func (m *model) undo() {
	if len(m.undoStack) == 0 {
		m.setStatus("Нечего отменять")
		return
	}
	prev := m.cfg
	m.cfg = m.undoStack[len(m.undoStack)-1]
	m.undoStack = m.undoStack[:len(m.undoStack)-1]
	m.clampAndRefresh(prev)
	if m.saveAndGenerate() {
		m.setStatus("Отменено ✓")
	}
}

// clampAndRefresh приводит активный префикс и пути к актуальным границам после
// замены конфигурации prev на m.cfg (undo) и пересобирает списки. Если открытый
// экран показывал исчезнувший узел, поднимается на уровень, который ещё есть.
func (m *model) clampAndRefresh(prev *models.Config) {
	m.refreshPrefixList()
	m.refreshVariableList()

	idx, ok := m.findActivePrefix(prev)
	if !ok {
		m.activePrefixIdx = 0
		m.cmdPath, m.flagCmdPath = nil, nil
		if m.state == stateCommandList || m.state == stateFlagList {
			m.state = statePrefixList
		}
		return
	}

	m.activePrefixIdx = idx
	m.cmdPath = m.validPathPrefix(m.cmdPath)
	m.refreshCommandList()
	if m.commandAt(m.flagCmdPath) != nil {
		m.refreshFlagList()
		return
	}
	m.flagCmdPath = nil
	if m.state == stateFlagList {
		m.state = stateCommandList
	}
}

// findActivePrefix ищет в m.cfg префикс, который был активным в prev: сначала
// по имени (его индекс мог сдвинуться), а если имя исчезло, но число префиксов
// не изменилось — по прежнему индексу (undo переименования). Иначе префикс
// удалён, и показывать его команды нельзя.
func (m *model) findActivePrefix(prev *models.Config) (int, bool) {
	ps := m.cfg.Prefixes
	if !inRange(m.activePrefixIdx, len(prev.Prefixes)) {
		return 0, false
	}
	name := prev.Prefixes[m.activePrefixIdx].Name
	if i := slices.IndexFunc(ps, func(p models.Prefix) bool { return p.Name == name }); i >= 0 {
		return i, true
	}
	if len(ps) == len(prev.Prefixes) {
		return m.activePrefixIdx, true
	}
	return 0, false
}

// toggleShell переключает целевую оболочку zsh↔bash и пересохраняет
// конфигурацию (скрипт перегенерируется уже выбранным компилятором).
func (m *model) toggleShell() {
	if m.shellName() == "zsh" {
		m.cfg.Shell = "bash"
	} else {
		m.cfg.Shell = "zsh"
	}
	m.persist()
	m.checkSourced()
}

// performDelete удаляет выбранный элемент после подтверждения. Пока ждём
// подтверждения, навигация заблокирована, поэтому индекс списка — тот самый.
func (m *model) performDelete() {
	m.confirmDelete, m.deleteLabel = false, ""

	switch m.state {
	case statePrefixList:
		if i := m.prefixesList.GlobalIndex(); inRange(i, len(m.cfg.Prefixes)) {
			m.mutate(func() { m.cfg.Prefixes = slices.Delete(m.cfg.Prefixes, i, i+1) })
		}
	case stateCommandList:
		children := m.childrenPtr()
		if i := m.commandsList.GlobalIndex(); inRange(i, len(*children)) {
			m.mutate(func() { *children = slices.Delete(*children, i, i+1) })
		}
	case stateFlagList:
		if cmd := m.commandAt(m.flagCmdPath); cmd != nil {
			if i := m.flagsList.GlobalIndex(); inRange(i, len(cmd.Flags)) {
				m.mutate(func() { cmd.Flags = slices.Delete(cmd.Flags, i, i+1) })
			}
		}
	case stateVariableList:
		if item, ok := m.variablesList.SelectedItem().(variableItem); ok {
			m.mutate(func() { delete(m.cfg.Variables, item.key) })
		}
	}
	m.refreshList(m.state)
}

// --- Перестановка ---
// Порядок влияет на справку и автодополнение. Выделение следует за элементом.

func (m *model) movePrefix(delta int) {
	m.moveItem(&m.prefixesList, len(m.cfg.Prefixes), delta,
		swapper(m.cfg.Prefixes), m.refreshPrefixList)
}

func (m *model) moveCommand(delta int) {
	cs := *m.childrenPtr()
	m.moveItem(&m.commandsList, len(cs), delta, swapper(cs), m.refreshCommandList)
}

func (m *model) moveFlag(delta int) {
	if cmd := m.commandAt(m.flagCmdPath); cmd != nil {
		m.moveItem(&m.flagsList, len(cmd.Flags), delta, swapper(cmd.Flags), m.refreshFlagList)
	}
}

// moveItem переставляет выбранный в l элемент (из n) на delta позиций.
func (m *model) moveItem(l *list.Model, n, delta int, swap func(i, j int), refresh func()) {
	i := l.GlobalIndex()
	j := i + delta
	if !inRange(i, n) || !inRange(j, n) {
		return
	}
	m.mutate(func() { swap(i, j) })
	refresh()
	l.Select(j)
}

func swapper[T any](s []T) func(i, j int) {
	return func(i, j int) { s[i], s[j] = s[j], s[i] }
}

// --- Импорт ---

// importAliases читает alias из ~/.zshrc и добавляет их командами в префикс-
// контейнер importedPrefix. Невалидные имена и уже существующие команды
// пропускаются; пустой контейнер не создаётся.
func (m *model) importAliases() {
	aliases, err := importer.LoadAliases()
	if err != nil {
		m.setError("Импорт: не удалось прочитать ~/.zshrc: " + err.Error())
		return
	}

	idx := slices.IndexFunc(m.cfg.Prefixes, func(p models.Prefix) bool {
		return p.Name == importedPrefix
	})
	existing := map[string]bool{}
	if idx >= 0 {
		for _, c := range m.cfg.Prefixes[idx].Commands {
			existing[c.Name] = true
		}
	}

	var toAdd []models.Command
	skipped := 0
	for _, a := range aliases {
		if validation.ValidateCommandName(a.Name) != nil || existing[a.Name] {
			skipped++
			continue
		}
		existing[a.Name] = true
		toAdd = append(toAdd, models.Command{
			Name: a.Name, Run: a.Value, Description: "импорт из ~/.zshrc",
		})
	}

	if len(toAdd) == 0 {
		m.setStatus(fmt.Sprintf("Импорт: нечего добавлять (пропущено %d)", skipped))
		return
	}

	m.mutate(func() {
		if idx < 0 {
			m.cfg.Prefixes = append(m.cfg.Prefixes, models.Prefix{
				Name: importedPrefix, Description: "Импортированные алиасы из ~/.zshrc",
			})
			idx = len(m.cfg.Prefixes) - 1
		}
		m.cfg.Prefixes[idx].Commands = append(m.cfg.Prefixes[idx].Commands, toAdd...)
	})
	m.refreshPrefixList()
	if !m.statusErr { // не затираем ошибку сохранения
		m.setStatus(fmt.Sprintf("Импорт: добавлено %d, пропущено %d", len(toAdd), skipped))
	}
}

// --- Дублирование ---

// duplicate вставляет сразу после выбранного элемента его глубокую копию с
// уникальным именем (<имя>-copy, <имя>-copy2, …) и выделяет копию.
func (m *model) duplicate() {
	switch m.state {
	case statePrefixList:
		ps := m.cfg.Prefixes
		i := m.prefixesList.GlobalIndex()
		if !inRange(i, len(ps)) {
			return
		}
		c := deepCopy(ps[i])
		// Имя префикса — имя функции, дефис недопустим.
		c.Name = uniqueName(c.Name, "_copy", func(n string) bool {
			return slices.ContainsFunc(ps, func(p models.Prefix) bool { return p.Name == n })
		})
		m.mutate(func() { m.cfg.Prefixes = slices.Insert(m.cfg.Prefixes, i+1, c) })
		m.afterDuplicate(&m.prefixesList, m.refreshPrefixList, i+1, "префикса", c.Name)

	case stateCommandList:
		children := m.childrenPtr()
		i := m.commandsList.GlobalIndex()
		if !inRange(i, len(*children)) {
			return
		}
		c := deepCopy((*children)[i])
		c.Name = uniqueName(c.Name, "-copy", func(n string) bool {
			return slices.ContainsFunc(*children, func(x models.Command) bool { return x.Name == n })
		})
		m.mutate(func() { *children = slices.Insert(*children, i+1, c) })
		m.afterDuplicate(&m.commandsList, m.refreshCommandList, i+1, "команды", c.Name)

	case stateFlagList:
		cmd := m.commandAt(m.flagCmdPath)
		if cmd == nil {
			return
		}
		i := m.flagsList.GlobalIndex()
		if !inRange(i, len(cmd.Flags)) {
			return
		}
		f := cmd.Flags[i]
		f.Name = uniqueName(f.Name, "-copy", func(n string) bool {
			return slices.ContainsFunc(cmd.Flags, func(x models.Flag) bool { return x.Name == n })
		})
		m.mutate(func() { cmd.Flags = slices.Insert(cmd.Flags, i+1, f) })
		m.afterDuplicate(&m.flagsList, m.refreshFlagList, i+1, "флага", f.Name)
	}
}

func (m *model) afterDuplicate(l *list.Model, refresh func(), idx int, what, name string) {
	refresh()
	l.Select(idx)
	if !m.statusErr {
		m.setStatus(fmt.Sprintf("Создана копия %s: %s", what, name))
	}
}

// uniqueName подбирает свободное имя base+suffix, base+suffix+"2", …
func uniqueName(base, suffix string, taken func(string) bool) string {
	name := base + suffix
	for n := 2; taken(name); n++ {
		name = fmt.Sprintf("%s%s%d", base, suffix, n)
	}
	return name
}

// deepCopy возвращает независимую копию значения (через YAML round-trip):
// у копии команды свои срезы флагов и подкоманд.
func deepCopy[T any](v T) T {
	var c T
	data, err := yaml.Marshal(v)
	if err != nil {
		return v
	}
	if err := yaml.Unmarshal(data, &c); err != nil {
		return v
	}
	return c
}

// --- Выключение ---

// toggleDisabled включает/выключает выбранный префикс, команду или флаг.
// Выключенный элемент остаётся в конфиге, но не попадает в скрипт.
func (m *model) toggleDisabled() {
	var (
		flag *bool
		l    *list.Model
		name string
	)
	switch m.state {
	case statePrefixList:
		if i := m.prefixesList.GlobalIndex(); inRange(i, len(m.cfg.Prefixes)) {
			flag, l, name = &m.cfg.Prefixes[i].Disabled, &m.prefixesList, m.cfg.Prefixes[i].Name
		}
	case stateCommandList:
		children := *m.childrenPtr()
		if i := m.commandsList.GlobalIndex(); inRange(i, len(children)) {
			flag, l, name = &children[i].Disabled, &m.commandsList, children[i].Name
		}
	case stateFlagList:
		if cmd := m.commandAt(m.flagCmdPath); cmd != nil {
			if i := m.flagsList.GlobalIndex(); inRange(i, len(cmd.Flags)) {
				flag, l, name = &cmd.Flags[i].Disabled, &m.flagsList, cmd.Flags[i].Name
			}
		}
	}
	if flag == nil {
		return
	}
	idx := l.GlobalIndex()
	m.mutate(func() { *flag = !*flag })
	m.refreshList(m.state)
	l.Select(idx)
	if m.statusErr {
		return
	}
	if *flag {
		m.setStatus(fmt.Sprintf("'%s' выключен — в скрипт не попадает", name))
	} else {
		m.setStatus(fmt.Sprintf("'%s' включён", name))
	}
}
