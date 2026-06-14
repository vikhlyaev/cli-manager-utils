package tui

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"

	"cli-manager/internal/compiler"
	"cli-manager/internal/models"
	"cli-manager/internal/storage"
	"cli-manager/internal/validation"
)

var (
	docStyle    = lipgloss.NewStyle().Margin(1, 2)
	titleStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("205")).Bold(true).Padding(0, 1)
	hintStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	warnStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("214")).Bold(true)
	errStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	okStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("78"))
	headerStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("205")).Bold(true)
	cursorStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("205")).Bold(true)
	boxStyle    = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("240")).Padding(0, 1).MarginTop(1)
)

type sessionState int

const (
	stateMainMenu sessionState = iota
	statePrefixList
	stateCommandList
	stateFlagList
	stateVariableList
	statePrefixForm
	stateCommandForm
	stateFlagForm
	stateVariableForm
)

// formMode различает создание нового элемента и редактирование существующего.
type formMode int

const (
	modeCreate formMode = iota
	modeEdit
)

var mainMenuItems = []string{"Префиксы", "Глобальные переменные"}

type model struct {
	state  sessionState
	cfg    *models.Config
	width  int
	height int

	menuIndex int

	prefixesList  list.Model
	commandsList  list.Model
	flagsList     list.Model
	variablesList list.Model

	activePrefixIdx  int
	activeCommandIdx int

	// форма и её буферы
	form     *huh.Form
	formMode formMode
	editKey  string       // исходный ключ переменной/индекс флага при редактировании
	fb       *formBuffers // привязанные к форме значения (стабильный указатель)

	// UX-состояние
	collision  bool
	syntaxErr  string // живая ошибка `bash -n` для поля run (не блокирует ввод)
	testOutput string
	testErr    bool
	status     string
	statusErr  bool

	// подтверждение удаления
	confirmDelete bool
	deleteLabel   string // что именно удаляем — для текста запроса
}

func newList(title string) list.Model {
	l := list.New([]list.Item{}, list.NewDefaultDelegate(), 0, 0)
	l.Title = title
	l.Styles.Title = titleStyle
	return l
}

// NewModel создаёт корневую модель приложения.
func NewModel(cfg *models.Config) model {
	if cfg.Variables == nil {
		cfg.Variables = map[string]string{}
	}
	m := model{
		state:         stateMainMenu,
		cfg:           cfg,
		prefixesList:  newList("CLI Manager · Префиксы"),
		commandsList:  newList(""),
		flagsList:     newList(""),
		variablesList: newList("CLI Manager · Глобальные переменные"),
	}
	m.refreshPrefixList()
	m.refreshVariableList()
	return m
}

// newFormBuffers аллоцирует свежий буфер формы (на новом указателе) и сбрасывает
// связанное с формой UX-состояние. Вызывается перед открытием любой формы.
func (m *model) newFormBuffers() {
	m.fb = &formBuffers{}
	m.collision = false
	m.syntaxErr = ""
	m.testOutput = ""
	m.testErr = false
	m.editKey = ""
}

// --- Обновление списков ---

func (m *model) refreshPrefixList() {
	items := make([]list.Item, len(m.cfg.Prefixes))
	for i, p := range m.cfg.Prefixes {
		items[i] = prefixItem{prefix: p}
	}
	m.prefixesList.SetItems(items)
}

func (m *model) refreshCommandList() {
	p := m.cfg.Prefixes[m.activePrefixIdx]
	m.commandsList.Title = fmt.Sprintf("'%s' · Команды", p.Name)
	items := make([]list.Item, len(p.Commands))
	for i, c := range p.Commands {
		items[i] = commandItem{cmd: c}
	}
	m.commandsList.SetItems(items)
}

func (m *model) refreshFlagList() {
	p := m.cfg.Prefixes[m.activePrefixIdx]
	c := p.Commands[m.activeCommandIdx]
	m.flagsList.Title = fmt.Sprintf("'%s %s' · Флаги", p.Name, c.Name)
	items := make([]list.Item, len(c.Flags))
	for i, f := range c.Flags {
		items[i] = flagItem{flag: f}
	}
	m.flagsList.SetItems(items)
}

func (m *model) refreshVariableList() {
	keys := make([]string, 0, len(m.cfg.Variables))
	for k := range m.cfg.Variables {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	items := make([]list.Item, len(keys))
	for i, k := range keys {
		items[i] = variableItem{key: k, value: m.cfg.Variables[k]}
	}
	m.variablesList.SetItems(items)
}

// persist сохраняет конфигурацию и регенерирует ZSH-скрипт.
func (m *model) persist() {
	if err := storage.SaveConfig(m.cfg); err != nil {
		m.status = "Ошибка сохранения: " + err.Error()
		m.statusErr = true
		return
	}
	if err := (compiler.ZshCompiler{}).Generate(m.cfg); err != nil {
		m.status = "Ошибка генерации zsh: " + err.Error()
		m.statusErr = true
		return
	}
	m.status = "Сохранено и скомпилировано ✓"
	m.statusErr = false
}

func (m model) Init() tea.Cmd { return nil }

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Глобальный выход.
	if key, ok := msg.(tea.KeyMsg); ok && key.String() == "ctrl+c" {
		return m, tea.Quit
	}

	if size, ok := msg.(tea.WindowSizeMsg); ok {
		m.width, m.height = size.Width, size.Height
		h, v := docStyle.GetFrameSize()
		w, ht := size.Width-h, size.Height-v
		m.prefixesList.SetSize(w, ht)
		m.commandsList.SetSize(w, ht)
		m.flagsList.SetSize(w, ht)
		m.variablesList.SetSize(w, ht)
	}

	// Активная форма перехватывает обработку.
	if m.isFormState() {
		return m.updateForm(msg)
	}

	// Режим подтверждения удаления перехватывает ввод (навигация заблокирована).
	if m.confirmDelete {
		if key, ok := msg.(tea.KeyMsg); ok {
			switch key.String() {
			case "y", "Y", "enter":
				m.performDelete()
			case "n", "N", "esc":
				m.confirmDelete = false
				m.deleteLabel = ""
			}
		}
		return m, nil
	}

	if key, ok := msg.(tea.KeyMsg); ok {
		switch m.state {
		case stateMainMenu:
			return m.updateMainMenu(key)
		case statePrefixList:
			if cmd, handled := m.updatePrefixList(key); handled {
				return m, cmd
			}
		case stateCommandList:
			if cmd, handled := m.updateCommandList(key); handled {
				return m, cmd
			}
		case stateFlagList:
			if cmd, handled := m.updateFlagList(key); handled {
				return m, cmd
			}
		case stateVariableList:
			if cmd, handled := m.updateVariableList(key); handled {
				return m, cmd
			}
		}
	}

	// Прокидываем сообщение в активный список (навигация, фильтрация).
	var cmd tea.Cmd
	switch m.state {
	case statePrefixList:
		m.prefixesList, cmd = m.prefixesList.Update(msg)
	case stateCommandList:
		m.commandsList, cmd = m.commandsList.Update(msg)
	case stateFlagList:
		m.flagsList, cmd = m.flagsList.Update(msg)
	case stateVariableList:
		m.variablesList, cmd = m.variablesList.Update(msg)
	}
	return m, cmd
}

func (m model) isFormState() bool {
	switch m.state {
	case statePrefixForm, stateCommandForm, stateFlagForm, stateVariableForm:
		return true
	}
	return false
}

// --- Главное меню ---

func (m model) updateMainMenu(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch key.String() {
	case "up", "k":
		if m.menuIndex > 0 {
			m.menuIndex--
		}
	case "down", "j":
		if m.menuIndex < len(mainMenuItems)-1 {
			m.menuIndex++
		}
	case "q":
		return m, tea.Quit
	case "enter":
		switch m.menuIndex {
		case 0:
			m.refreshPrefixList()
			m.state = statePrefixList
		case 1:
			m.refreshVariableList()
			m.state = stateVariableList
		}
	}
	return m, nil
}

// --- Списки: возвращают (cmd, handled) ---

func (m *model) updatePrefixList(key tea.KeyMsg) (tea.Cmd, bool) {
	switch key.String() {
	case "esc":
		m.state = stateMainMenu
		return nil, true
	case "n":
		m.newFormBuffers()
		cmd := m.initPrefixForm()
		m.formMode = modeCreate
		m.state = statePrefixForm
		return cmd, true
	case "e":
		if item, ok := m.prefixesList.SelectedItem().(prefixItem); ok {
			m.activePrefixIdx = m.prefixesList.Index()
			m.newFormBuffers()
			m.fb.name = item.prefix.Name
			m.fb.description = item.prefix.Description
			cmd := m.initPrefixForm()
			m.formMode = modeEdit
			m.state = statePrefixForm
			return cmd, true
		}
	case "d":
		if item, ok := m.prefixesList.SelectedItem().(prefixItem); ok {
			m.confirmDelete = true
			m.deleteLabel = fmt.Sprintf("префикс '%s' (команд: %d)",
				item.prefix.Name, len(item.prefix.Commands))
		}
		return nil, true
	case "enter":
		if _, ok := m.prefixesList.SelectedItem().(prefixItem); ok {
			m.activePrefixIdx = m.prefixesList.Index()
			m.refreshCommandList()
			m.state = stateCommandList
		}
		return nil, true
	}
	return nil, false
}

func (m *model) updateCommandList(key tea.KeyMsg) (tea.Cmd, bool) {
	switch key.String() {
	case "esc":
		m.refreshPrefixList()
		m.state = statePrefixList
		return nil, true
	case "n":
		m.newFormBuffers()
		cmd := m.initCommandForm()
		m.formMode = modeCreate
		m.state = stateCommandForm
		return cmd, true
	case "e":
		if _, ok := m.commandsList.SelectedItem().(commandItem); ok {
			m.activeCommandIdx = m.commandsList.Index()
			c := m.cfg.Prefixes[m.activePrefixIdx].Commands[m.activeCommandIdx]
			m.newFormBuffers()
			m.fb.name = c.Name
			m.fb.description = c.Description
			m.fb.run = c.Run
			m.fb.acceptsArgs = c.AcceptsArgs
			m.fb.condition = c.Condition
			cmd := m.initCommandForm()
			m.formMode = modeEdit
			m.state = stateCommandForm
			return cmd, true
		}
	case "d":
		if item, ok := m.commandsList.SelectedItem().(commandItem); ok {
			m.confirmDelete = true
			m.deleteLabel = fmt.Sprintf("команду '%s'", item.cmd.Name)
		}
		return nil, true
	case "enter":
		if _, ok := m.commandsList.SelectedItem().(commandItem); ok {
			m.activeCommandIdx = m.commandsList.Index()
			m.refreshFlagList()
			m.state = stateFlagList
		}
		return nil, true
	}
	return nil, false
}

func (m *model) updateFlagList(key tea.KeyMsg) (tea.Cmd, bool) {
	switch key.String() {
	case "esc":
		m.refreshCommandList()
		m.state = stateCommandList
		return nil, true
	case "n":
		m.newFormBuffers()
		cmd := m.initFlagForm()
		m.formMode = modeCreate
		m.state = stateFlagForm
		return cmd, true
	case "e":
		if _, ok := m.flagsList.SelectedItem().(flagItem); ok {
			idx := m.flagsList.Index()
			f := m.cfg.Prefixes[m.activePrefixIdx].Commands[m.activeCommandIdx].Flags[idx]
			m.newFormBuffers()
			m.editKey = fmt.Sprintf("%d", idx)
			m.fb.name = f.Name
			m.fb.description = f.Description
			m.fb.run = f.Run
			cmd := m.initFlagForm()
			m.formMode = modeEdit
			m.state = stateFlagForm
			return cmd, true
		}
	case "d":
		if item, ok := m.flagsList.SelectedItem().(flagItem); ok {
			m.confirmDelete = true
			m.deleteLabel = fmt.Sprintf("флаг '%s'", item.flag.Name)
		}
		return nil, true
	}
	return nil, false
}

func (m *model) updateVariableList(key tea.KeyMsg) (tea.Cmd, bool) {
	switch key.String() {
	case "esc":
		m.state = stateMainMenu
		return nil, true
	case "n":
		m.newFormBuffers()
		cmd := m.initVariableForm()
		m.formMode = modeCreate
		m.state = stateVariableForm
		return cmd, true
	case "e":
		if item, ok := m.variablesList.SelectedItem().(variableItem); ok {
			m.newFormBuffers()
			m.editKey = item.key
			m.fb.name = item.key
			m.fb.value = item.value
			cmd := m.initVariableForm()
			m.formMode = modeEdit
			m.state = stateVariableForm
			return cmd, true
		}
	case "d":
		if item, ok := m.variablesList.SelectedItem().(variableItem); ok {
			m.confirmDelete = true
			m.deleteLabel = fmt.Sprintf("переменную '%s'", item.key)
		}
		return nil, true
	}
	return nil, false
}

// performDelete выполняет удаление выбранного элемента после подтверждения.
// Вызывается только из режима confirmDelete, где навигация заблокирована, поэтому
// текущий индекс списка соответствует тому, что пользователь подтверждает.
func (m *model) performDelete() {
	switch m.state {
	case statePrefixList:
		idx := m.prefixesList.Index()
		if idx >= 0 && idx < len(m.cfg.Prefixes) {
			m.cfg.Prefixes = append(m.cfg.Prefixes[:idx], m.cfg.Prefixes[idx+1:]...)
			m.persist()
			m.refreshPrefixList()
		}
	case stateCommandList:
		cs := m.cfg.Prefixes[m.activePrefixIdx].Commands
		idx := m.commandsList.Index()
		if idx >= 0 && idx < len(cs) {
			m.cfg.Prefixes[m.activePrefixIdx].Commands = append(cs[:idx], cs[idx+1:]...)
			m.persist()
			m.refreshCommandList()
		}
	case stateFlagList:
		fs := m.cfg.Prefixes[m.activePrefixIdx].Commands[m.activeCommandIdx].Flags
		idx := m.flagsList.Index()
		if idx >= 0 && idx < len(fs) {
			m.cfg.Prefixes[m.activePrefixIdx].Commands[m.activeCommandIdx].Flags =
				append(fs[:idx], fs[idx+1:]...)
			m.persist()
			m.refreshFlagList()
		}
	case stateVariableList:
		if item, ok := m.variablesList.SelectedItem().(variableItem); ok {
			delete(m.cfg.Variables, item.key)
			m.persist()
			m.refreshVariableList()
		}
	}
	m.confirmDelete = false
	m.deleteLabel = ""
}

// --- Обработка форм ---

func (m model) updateForm(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case "esc":
			m.exitForm()
			return m, nil
		case "ctrl+t":
			// Тестовый запуск bash-кода из поля "run".
			if m.state == stateCommandForm || m.state == stateFlagForm {
				code := m.fb.run
				out, err := validation.ValidateBash(code)
				m.testOutput = strings.TrimRight(out, "\n")
				m.testErr = err != nil
				if m.testOutput == "" {
					if err != nil {
						m.testOutput = err.Error()
					} else {
						m.testOutput = "(пустой вывод, код завершён успешно)"
					}
				}
				return m, nil
			}
		}
	}

	f, cmd := m.form.Update(msg)
	if form, ok := f.(*huh.Form); ok {
		m.form = form
	}

	// Живое предупреждение о коллизии имени команды.
	if m.state == stateCommandForm {
		m.collision = validation.CheckCollision(m.fb.name)
	}

	// Живая (неблокирующая) проверка синтаксиса bash-кода поля run.
	if m.state == stateCommandForm || m.state == stateFlagForm {
		if err := validation.CheckSyntax(m.fb.run); err != nil {
			m.syntaxErr = err.Error()
		} else {
			m.syntaxErr = ""
		}
	}

	if m.form.State == huh.StateCompleted {
		m.commitForm()
		return m, nil
	}
	return m, cmd
}

// exitForm закрывает форму без сохранения.
func (m *model) exitForm() {
	switch m.state {
	case statePrefixForm:
		m.state = statePrefixList
	case stateCommandForm:
		m.state = stateCommandList
	case stateFlagForm:
		m.state = stateFlagList
	case stateVariableForm:
		m.state = stateVariableList
	}
	m.testOutput = ""
	m.collision = false
}

// commitForm применяет данные формы к конфигурации, сохраняет и возвращает к списку.
// Значения берём из стабильного буфера m.fb (см. комментарий к formBuffers).
func (m *model) commitForm() {
	switch m.state {
	case statePrefixForm:
		if m.formMode == modeEdit {
			m.cfg.Prefixes[m.activePrefixIdx].Name = m.fb.name
			m.cfg.Prefixes[m.activePrefixIdx].Description = m.fb.description
		} else {
			m.cfg.Prefixes = append(m.cfg.Prefixes, models.Prefix{
				Name:        m.fb.name,
				Description: m.fb.description,
			})
		}
		m.persist()
		m.refreshPrefixList()
		m.state = statePrefixList

	case stateCommandForm:
		c := models.Command{
			Name:        m.fb.name,
			Description: m.fb.description,
			Run:         m.fb.run,
			AcceptsArgs: m.fb.acceptsArgs,
			Condition:   m.fb.condition,
		}
		cmds := m.cfg.Prefixes[m.activePrefixIdx].Commands
		if m.formMode == modeEdit {
			c.Flags = cmds[m.activeCommandIdx].Flags // сохраняем существующие флаги
			cmds[m.activeCommandIdx] = c
		} else {
			m.cfg.Prefixes[m.activePrefixIdx].Commands = append(cmds, c)
		}
		m.persist()
		m.refreshCommandList()
		m.state = stateCommandList

	case stateFlagForm:
		f := models.Flag{
			Name:        m.fb.name,
			Description: m.fb.description,
			Run:         m.fb.run,
		}
		flags := m.cfg.Prefixes[m.activePrefixIdx].Commands[m.activeCommandIdx].Flags
		if m.formMode == modeEdit {
			var idx int
			fmt.Sscanf(m.editKey, "%d", &idx)
			flags[idx] = f
		} else {
			m.cfg.Prefixes[m.activePrefixIdx].Commands[m.activeCommandIdx].Flags =
				append(flags, f)
		}
		m.persist()
		m.refreshFlagList()
		m.state = stateFlagList

	case stateVariableForm:
		if m.formMode == modeEdit && m.editKey != "" && m.editKey != m.fb.name {
			delete(m.cfg.Variables, m.editKey) // переименование
		}
		m.cfg.Variables[m.fb.name] = m.fb.value
		m.persist()
		m.refreshVariableList()
		m.state = stateVariableList
	}

	m.testOutput = ""
	m.collision = false
}

// --- Рендеринг ---

func (m model) View() string {
	switch m.state {
	case stateMainMenu:
		return m.viewMainMenu()
	case statePrefixList:
		return m.listView(m.prefixesList.View(),
			"[n] новый  [e] изменить  [d] удалить  [enter] открыть  [esc] меню  [ctrl+c] выход")
	case stateCommandList:
		return m.listView(m.commandsList.View(),
			"[n] новая  [e] изменить  [d] удалить  [enter] флаги  [esc] назад")
	case stateFlagList:
		return m.listView(m.flagsList.View(),
			"[n] новый  [e] изменить  [d] удалить  [esc] назад")
	case stateVariableList:
		return m.listView(m.variablesList.View(),
			"[n] новая  [e] изменить  [d] удалить  [esc] меню")
	case statePrefixForm, stateCommandForm, stateFlagForm, stateVariableForm:
		return m.viewForm()
	}
	return ""
}

func (m model) viewMainMenu() string {
	var b strings.Builder
	b.WriteString(headerStyle.Render("CLI Manager") + "\n")
	b.WriteString(hintStyle.Render("Конструктор составных команд для ZSH") + "\n\n")
	for i, item := range mainMenuItems {
		cursor := "  "
		line := item
		if i == m.menuIndex {
			cursor = cursorStyle.Render("› ")
			line = cursorStyle.Render(item)
		}
		b.WriteString(cursor + line + "\n")
	}
	b.WriteString("\n" + hintStyle.Render("[↑/↓] выбор  [enter] открыть  [q] выход"))
	if m.status != "" {
		b.WriteString("\n\n" + m.statusLine())
	}
	return docStyle.Render(b.String())
}

func (m model) listView(body, hints string) string {
	out := body + "\n\n  "
	if m.confirmDelete {
		out += warnStyle.Render(fmt.Sprintf("Удалить %s? ", m.deleteLabel)) +
			hintStyle.Render("[y] да  [n] нет")
	} else {
		out += hintStyle.Render(hints)
	}
	if m.status != "" {
		out += "\n  " + m.statusLine()
	}
	return docStyle.Render(out)
}

func (m model) statusLine() string {
	if m.statusErr {
		return errStyle.Render(m.status)
	}
	return okStyle.Render(m.status)
}

func (m model) viewForm() string {
	var b strings.Builder
	b.WriteString(m.form.View())

	if m.state == stateCommandForm && m.collision {
		b.WriteString("\n" + warnStyle.Render(
			fmt.Sprintf("⚠️  Команда '%s' уже существует в системе — возможно перекрытие!",
				m.fb.name)))
	}

	if (m.state == stateCommandForm || m.state == stateFlagForm) && m.syntaxErr != "" {
		b.WriteString("\n" + errStyle.Render("✗ синтаксис bash: "+m.syntaxErr))
	}

	if m.testOutput != "" {
		title := okStyle.Render("Результат теста (Ctrl+T):")
		if m.testErr {
			title = errStyle.Render("Результат теста (ошибка):")
		}
		b.WriteString("\n" + boxStyle.Render(title+"\n"+m.testOutput))
	}

	b.WriteString("\n\n" + hintStyle.Render("[enter] далее/сохранить  [esc] отмена  [ctrl+t] тест"))
	return docStyle.Render(b.String())
}

// Run запускает TUI-приложение.
func Run(cfg *models.Config) {
	p := tea.NewProgram(NewModel(cfg), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "Ошибка при запуске TUI:", err)
		os.Exit(1)
	}
}
