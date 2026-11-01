package tui

import (
	"fmt"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"

	"cli-manager/internal/compiler"
	"cli-manager/internal/models"
)

// --- Наполнение списков ---

func (m *model) refreshPrefixList() {
	items := make([]list.Item, len(m.cfg.Prefixes))
	for i, p := range m.cfg.Prefixes {
		items[i] = prefixItem{prefix: p}
	}
	m.prefixesList.SetItems(items)
}

func (m *model) refreshCommandList() {
	cmds := *m.childrenPtr()
	items := make([]list.Item, len(cmds))
	for i, c := range cmds {
		items[i] = commandItem{cmd: c}
	}
	m.commandsList.SetItems(items)
}

func (m *model) refreshFlagList() {
	c := m.commandAt(m.flagCmdPath)
	if c == nil {
		return
	}
	items := make([]list.Item, len(c.Flags))
	for i, f := range c.Flags {
		items[i] = flagItem{flag: f}
	}
	m.flagsList.SetItems(items)
}

func (m *model) refreshVariableList() {
	keys := sortedKeys(m.cfg.Variables)
	items := make([]list.Item, len(keys))
	for i, k := range keys {
		items[i] = variableItem{key: k, value: m.cfg.Variables[k]}
	}
	m.variablesList.SetItems(items)
}

// refreshList пересобирает список, соответствующий экрану state.
func (m *model) refreshList(state sessionState) {
	switch state {
	case statePrefixList:
		m.refreshPrefixList()
	case stateCommandList:
		m.refreshCommandList()
	case stateFlagList:
		m.refreshFlagList()
	case stateVariableList:
		m.refreshVariableList()
	}
}

// activeList возвращает список текущего экрана или nil, если экран — не список.
func (m *model) activeList() *list.Model {
	switch m.state {
	case statePrefixList:
		return &m.prefixesList
	case stateCommandList:
		return &m.commandsList
	case stateFlagList:
		return &m.flagsList
	case stateVariableList:
		return &m.variablesList
	}
	return nil
}

// --- Главное меню ---

func (m model) updateMainMenu(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch key.String() {
	case "up", "k":
		if m.menuIndex > 0 {
			m.menuIndex--
		}
	case "down", "j":
		if m.menuIndex < len(m.menuItems())-1 {
			m.menuIndex++
		}
	case "q":
		return m, tea.Quit
	case "?":
		m.openHelp()
	case "enter":
		switch m.menuIndex {
		case menuPrefixes:
			m.refreshPrefixList()
			m.state = statePrefixList
		case menuVariables:
			m.refreshVariableList()
			m.state = stateVariableList
		case menuShell:
			m.toggleShell()
		case menuPreview:
			m.openPreview(fmt.Sprintf("Предпросмотр %s · весь скрипт", m.shellName()),
				compiler.For(m.cfg.Shell).Render(m.cfg))
		}
	}
	return m, nil
}

// --- Клавиши экранов-списков ---
// Обработчики возвращают (cmd, handled). Необработанная клавиша уходит в сам
// list.Model (навигация).

func (m *model) updateListKey(key tea.KeyMsg) (tea.Cmd, bool) {
	// Отмена и справка доступны во всех списках.
	switch key.String() {
	case "u":
		m.undo()
		return nil, true
	case "?":
		m.openHelp()
		return nil, true
	}
	switch m.state {
	case statePrefixList:
		return m.updatePrefixList(key)
	case stateCommandList:
		return m.updateCommandList(key)
	case stateFlagList:
		return m.updateFlagList(key)
	case stateVariableList:
		return m.updateVariableList(key)
	}
	return nil, false
}

func (m *model) updatePrefixList(key tea.KeyMsg) (tea.Cmd, bool) {
	item, selected := m.prefixesList.SelectedItem().(prefixItem)
	switch key.String() {
	case "esc":
		m.state = stateMainMenu
		return nil, true
	case "n":
		return m.openCreate(statePrefixForm), true
	case "e":
		if selected {
			m.newFormBuffers()
			m.editIdx = m.prefixesList.Index()
			m.fb.name = item.prefix.Name
			m.fb.description = item.prefix.Description
			return m.startForm(statePrefixForm, modeEdit), true
		}
	case "d":
		if selected {
			m.askDelete(fmt.Sprintf("префикс '%s' (команд: %d)",
				item.prefix.Name, len(item.prefix.Commands)))
		}
		return nil, true
	case "p":
		if selected {
			m.previewPrefix(item.prefix)
		}
		return nil, true
	case "K", "shift+up":
		m.movePrefix(-1)
		return nil, true
	case "J", "shift+down":
		m.movePrefix(1)
		return nil, true
	case "i":
		m.importAliases()
		return nil, true
	case "enter":
		if selected {
			m.activePrefixIdx = m.prefixesList.Index()
			m.cmdPath = nil
			m.refreshCommandList()
			m.commandsList.Select(0)
			m.state = stateCommandList
		}
		return nil, true
	}
	return nil, false
}

func (m *model) updateCommandList(key tea.KeyMsg) (tea.Cmd, bool) {
	item, selected := m.commandsList.SelectedItem().(commandItem)
	idx := m.commandsList.Index()
	switch key.String() {
	case "esc":
		// На уровень выше: к родительской команде, а с верхнего уровня — к префиксам.
		if !m.leaveSubcommands() {
			m.refreshPrefixList()
			m.state = statePrefixList
		}
		return nil, true
	case "n":
		return m.openCreate(stateCommandForm), true
	case "e":
		if selected {
			m.newFormBuffers()
			m.editIdx = idx
			m.fb.loadCommand(item.cmd)
			return m.startForm(stateCommandForm, modeEdit), true
		}
	case "d":
		if selected {
			m.askDelete(fmt.Sprintf("команду '%s'", item.cmd.Name))
		}
		return nil, true
	case "p":
		m.previewPrefix(*m.activePrefix())
		return nil, true
	case "K", "shift+up":
		m.moveCommand(-1)
		return nil, true
	case "J", "shift+down":
		m.moveCommand(1)
		return nil, true
	case "f":
		if selected {
			m.openFlags(idx)
		}
		return nil, true
	case "s", "right":
		// Явно войти в подкоманды (в т.ч. чтобы начать вложенность у листа).
		if selected {
			m.enterSubcommands(idx)
		}
		return nil, true
	case "enter":
		// Ветвь — вглубь, лист — к его флагам.
		if selected {
			if item.cmd.IsBranch() {
				m.enterSubcommands(idx)
			} else {
				m.openFlags(idx)
			}
		}
		return nil, true
	}
	return nil, false
}

func (m *model) updateFlagList(key tea.KeyMsg) (tea.Cmd, bool) {
	item, selected := m.flagsList.SelectedItem().(flagItem)
	switch key.String() {
	case "esc":
		m.refreshCommandList()
		m.state = stateCommandList
		return nil, true
	case "n":
		return m.openCreate(stateFlagForm), true
	case "e":
		if selected {
			m.newFormBuffers()
			m.editIdx = m.flagsList.Index()
			m.fb.loadFlag(item.flag)
			return m.startForm(stateFlagForm, modeEdit), true
		}
	case "d":
		if selected {
			m.askDelete(fmt.Sprintf("флаг '%s'", item.flag.Name))
		}
		return nil, true
	case "K", "shift+up":
		m.moveFlag(-1)
		return nil, true
	case "J", "shift+down":
		m.moveFlag(1)
		return nil, true
	}
	return nil, false
}

func (m *model) updateVariableList(key tea.KeyMsg) (tea.Cmd, bool) {
	item, selected := m.variablesList.SelectedItem().(variableItem)
	switch key.String() {
	case "esc":
		m.state = stateMainMenu
		return nil, true
	case "n":
		return m.openCreate(stateVariableForm), true
	case "e":
		if selected {
			m.newFormBuffers()
			m.editKey = item.key
			m.fb.name = item.key
			m.fb.value = item.value
			return m.startForm(stateVariableForm, modeEdit), true
		}
	case "d":
		if selected {
			m.askDelete(fmt.Sprintf("переменную '%s'", item.key))
		}
		return nil, true
	}
	return nil, false
}

// openCreate открывает пустую форму создания элемента.
func (m *model) openCreate(state sessionState) tea.Cmd {
	m.newFormBuffers()
	return m.startForm(state, modeCreate)
}

// askDelete включает режим подтверждения удаления выбранного элемента.
func (m *model) askDelete(label string) {
	m.confirmDelete = true
	m.deleteLabel = label
}

// previewPrefix открывает предпросмотр сгенерированного кода префикса.
func (m *model) previewPrefix(p models.Prefix) {
	content := compiler.For(m.cfg.Shell).RenderPrefix(p)
	m.openPreview(fmt.Sprintf("Предпросмотр %s · префикс '%s'", m.shellName(), p.Name), content)
}

// openHelp показывает полный список клавиш в окне предпросмотра.
func (m *model) openHelp() {
	m.openPreview(helpTitle, helpText)
}

// openPreview наполняет viewport содержимым и переключается на экран
// предпросмотра, запоминая текущий экран для возврата по Esc.
func (m *model) openPreview(title, content string) {
	m.previewTitle = title
	if m.previewReady {
		m.previewVP.SetContent(content)
		m.previewVP.GotoTop()
	}
	m.previousState = m.state
	m.state = statePreview
}
