package tui

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"

	"cli-manager/internal/compiler"
	"cli-manager/internal/importer"
	"cli-manager/internal/models"
	"cli-manager/internal/storage"
	"cli-manager/internal/validation"
)

// importedPrefix — имя префикса-контейнера, в который складываются alias,
// импортированные из ~/.zshrc.
const importedPrefix = "imported"

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
	statePreview
)

// formMode различает создание нового элемента и редактирование существующего.
type formMode int

const (
	modeCreate formMode = iota
	modeEdit
)

// shellName возвращает нормализованное имя целевой оболочки ("zsh" по умолчанию).
func (m model) shellName() string {
	if strings.ToLower(strings.TrimSpace(m.cfg.Shell)) == "bash" {
		return "bash"
	}
	return "zsh"
}

// menuItems формирует пункты главного меню; третий пункт показывает и переключает
// целевую оболочку.
func (m model) menuItems() []string {
	return []string{"Префиксы", "Глобальные переменные", "Оболочка: " + m.shellName()}
}

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
	activeCommandIdx int   // выбранный/редактируемый индекс в текущем списке команд
	cmdPath          []int // путь индексов до узла, чьи дети показаны в списке команд
	flagCmdPath      []int // путь до команды, чьи флаги показаны в списке флагов

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

	// предпросмотр сгенерированного zsh
	previewVP     viewport.Model
	previewReady  bool         // viewport получил размеры (WindowSizeMsg)
	previewTitle  string       // заголовок над окном предпросмотра
	previousState sessionState // куда вернуться по Esc из предпросмотра

	// undo: снимки конфигурации до каждой мутации (последний — самый свежий)
	undoStack []*models.Config
}

const undoLimit = 50

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

// childrenPtr возвращает указатель на срез команд, отображаемый сейчас в списке
// команд (дети узла, адресованного cmdPath; при пустом пути — команды префикса).
func (m *model) childrenPtr() *[]models.Command {
	cmds := &m.cfg.Prefixes[m.activePrefixIdx].Commands
	for _, idx := range m.cmdPath {
		if idx >= 0 && idx < len(*cmds) {
			cmds = &(*cmds)[idx].Subcommands
		}
	}
	return cmds
}

// commandAt возвращает указатель на команду по пути индексов (относительно
// активного префикса) или nil, если путь невалиден.
func (m *model) commandAt(path []int) *models.Command {
	cmds := &m.cfg.Prefixes[m.activePrefixIdx].Commands
	var cmd *models.Command
	for _, idx := range path {
		if idx < 0 || idx >= len(*cmds) {
			return nil
		}
		cmd = &(*cmds)[idx]
		cmds = &cmd.Subcommands
	}
	return cmd
}

// breadcrumb строит человекочитаемый путь ("av db migrate") по пути индексов.
func (m *model) breadcrumb(path []int) string {
	parts := []string{m.cfg.Prefixes[m.activePrefixIdx].Name}
	cmds := m.cfg.Prefixes[m.activePrefixIdx].Commands
	for _, idx := range path {
		if idx < 0 || idx >= len(cmds) {
			break
		}
		parts = append(parts, cmds[idx].Name)
		cmds = cmds[idx].Subcommands
	}
	return strings.Join(parts, " ")
}

// validPathPrefix усекает путь до первого невалидного индекса (после изменения
// дерева, напр. при undo).
func (m *model) validPathPrefix(path []int) []int {
	cmds := m.cfg.Prefixes[m.activePrefixIdx].Commands
	out := path[:0:0]
	for _, idx := range path {
		if idx < 0 || idx >= len(cmds) {
			break
		}
		out = append(out, idx)
		cmds = cmds[idx].Subcommands
	}
	return out
}

func (m *model) refreshCommandList() {
	m.commandsList.Title = fmt.Sprintf("'%s' · Команды", m.breadcrumb(m.cmdPath))
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
	m.flagsList.Title = fmt.Sprintf("'%s' · Флаги", m.breadcrumb(m.flagCmdPath))
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
	if err := compiler.For(m.cfg.Shell).Generate(m.cfg); err != nil {
		m.status = "Ошибка генерации zsh: " + err.Error()
		m.statusErr = true
		return
	}
	m.status = "Сохранено и скомпилировано ✓"
	m.statusErr = false
}

// pushUndo сохраняет снимок текущей конфигурации в undo-стек. Вызывается перед
// каждой мутацией (до изменения cfg).
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

// undo откатывает последнюю мутацию: восстанавливает снимок, сохраняет его на
// диск, перегенерирует скрипт и обновляет списки.
func (m *model) undo() {
	if len(m.undoStack) == 0 {
		m.status = "Нечего отменять"
		m.statusErr = false
		return
	}
	prev := m.undoStack[len(m.undoStack)-1]
	m.undoStack = m.undoStack[:len(m.undoStack)-1]
	m.cfg = prev

	if err := storage.SaveConfig(m.cfg); err != nil {
		m.status = "Ошибка отмены: " + err.Error()
		m.statusErr = true
		return
	}
	if err := compiler.For(m.cfg.Shell).Generate(m.cfg); err != nil {
		m.status = "Ошибка генерации zsh: " + err.Error()
		m.statusErr = true
		return
	}
	m.clampAndRefresh()
	m.status = "Отменено ✓"
	m.statusErr = false
}

// clampAndRefresh приводит активные индексы к актуальным границам после замены
// конфигурации (undo) и пересобирает списки, удерживая выделение в допустимых
// пределах.
func (m *model) clampAndRefresh() {
	if m.activePrefixIdx >= len(m.cfg.Prefixes) {
		m.activePrefixIdx = len(m.cfg.Prefixes) - 1
	}
	if m.activePrefixIdx < 0 {
		m.activePrefixIdx = 0
	}
	m.refreshPrefixList()
	m.refreshVariableList()

	if len(m.cfg.Prefixes) == 0 {
		m.cmdPath = nil
		m.flagCmdPath = nil
		return
	}

	// Усекаем пути до валидных после возможного изменения дерева.
	m.cmdPath = m.validPathPrefix(m.cmdPath)
	m.refreshCommandList()
	if m.commandAt(m.flagCmdPath) != nil {
		m.refreshFlagList()
	} else {
		m.flagCmdPath = nil
	}
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

		// Резервируем строки под заголовок (имя + путь), отступы и подсказки.
		vpH := ht - 5
		if vpH < 1 {
			vpH = 1
		}
		if !m.previewReady {
			m.previewVP = viewport.New(w, vpH)
			m.previewReady = true
		} else {
			m.previewVP.Width, m.previewVP.Height = w, vpH
		}
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

	// Экран предпросмотра: Esc возвращает назад, остальное — прокрутка viewport.
	if m.state == statePreview {
		if key, ok := msg.(tea.KeyMsg); ok {
			switch key.String() {
			case "esc", "q":
				m.state = m.previousState
				return m, nil
			}
		}
		var cmd tea.Cmd
		m.previewVP, cmd = m.previewVP.Update(msg)
		return m, cmd
	}

	// Отмена последней мутации доступна во всех списках.
	if key, ok := msg.(tea.KeyMsg); ok && key.String() == "u" {
		switch m.state {
		case statePrefixList, stateCommandList, stateFlagList, stateVariableList:
			m.undo()
			return m, nil
		}
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
		if m.menuIndex < len(m.menuItems())-1 {
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
		case 2:
			m.toggleShell()
		}
	}
	return m, nil
}

// toggleShell переключает целевую оболочку zsh↔bash и пересохраняет конфигурацию
// (что регенерирует скрипт уже выбранным компилятором).
func (m *model) toggleShell() {
	if m.shellName() == "zsh" {
		m.cfg.Shell = "bash"
	} else {
		m.cfg.Shell = "zsh"
	}
	m.persist()
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
	case "p":
		if item, ok := m.prefixesList.SelectedItem().(prefixItem); ok {
			content := compiler.For(m.cfg.Shell).RenderPrefix(item.prefix)
			m.openPreview("Предпросмотр zsh · префикс '"+item.prefix.Name+"'", content)
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
		m.prefixesList.Select(m.prefixesList.Index())
		return nil, true
	case "enter":
		if _, ok := m.prefixesList.SelectedItem().(prefixItem); ok {
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
	switch key.String() {
	case "esc":
		// Выходим на уровень выше: либо к родительской команде, либо к префиксам.
		if len(m.cmdPath) > 0 {
			child := m.cmdPath[len(m.cmdPath)-1]
			m.cmdPath = m.cmdPath[:len(m.cmdPath)-1]
			m.refreshCommandList()
			m.commandsList.Select(child)
		} else {
			m.refreshPrefixList()
			m.state = statePrefixList
		}
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
			c := (*m.childrenPtr())[m.activeCommandIdx]
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
	case "p":
		p := m.cfg.Prefixes[m.activePrefixIdx]
		content := compiler.For(m.cfg.Shell).RenderPrefix(p)
		m.openPreview("Предпросмотр zsh · префикс '"+p.Name+"'", content)
		return nil, true
	case "K", "shift+up":
		m.moveCommand(-1)
		return nil, true
	case "J", "shift+down":
		m.moveCommand(1)
		return nil, true
	case "f":
		// Открыть флаги выбранной команды (актуально для листьев).
		if _, ok := m.commandsList.SelectedItem().(commandItem); ok {
			idx := m.commandsList.Index()
			m.flagCmdPath = append(append([]int{}, m.cmdPath...), idx)
			m.refreshFlagList()
			m.flagsList.Select(0)
			m.state = stateFlagList
		}
		return nil, true
	case "enter":
		// Углубиться в подкоманды выбранной команды.
		if _, ok := m.commandsList.SelectedItem().(commandItem); ok {
			idx := m.commandsList.Index()
			m.cmdPath = append(m.cmdPath, idx)
			m.refreshCommandList()
			m.commandsList.Select(0)
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
		if cmd := m.commandAt(m.flagCmdPath); cmd != nil {
			if _, ok := m.flagsList.SelectedItem().(flagItem); ok {
				idx := m.flagsList.Index()
				f := cmd.Flags[idx]
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
		}
	case "d":
		if item, ok := m.flagsList.SelectedItem().(flagItem); ok {
			m.confirmDelete = true
			m.deleteLabel = fmt.Sprintf("флаг '%s'", item.flag.Name)
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
	m.pushUndo()
	switch m.state {
	case statePrefixList:
		idx := m.prefixesList.Index()
		if idx >= 0 && idx < len(m.cfg.Prefixes) {
			m.cfg.Prefixes = append(m.cfg.Prefixes[:idx], m.cfg.Prefixes[idx+1:]...)
			m.persist()
			m.refreshPrefixList()
		}
	case stateCommandList:
		children := m.childrenPtr()
		idx := m.commandsList.Index()
		if idx >= 0 && idx < len(*children) {
			*children = append((*children)[:idx], (*children)[idx+1:]...)
			m.persist()
			m.refreshCommandList()
		}
	case stateFlagList:
		if cmd := m.commandAt(m.flagCmdPath); cmd != nil {
			idx := m.flagsList.Index()
			if idx >= 0 && idx < len(cmd.Flags) {
				cmd.Flags = append(cmd.Flags[:idx], cmd.Flags[idx+1:]...)
				m.persist()
				m.refreshFlagList()
			}
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

// openPreview наполняет viewport содержимым и переключается на экран предпросмотра,
// запоминая текущее состояние для возврата по Esc.
func (m *model) openPreview(title, content string) {
	m.previewTitle = title
	if m.previewReady {
		m.previewVP.SetContent(content)
		m.previewVP.GotoTop()
	}
	m.previousState = m.state
	m.state = statePreview
}

// movePrefix/moveCommand/moveFlag переставляют выбранный элемент на delta позиций
// (порядок влияет на справку и автодополнение), сохраняют и удерживают выделение
// на перемещённом элементе.
func (m *model) movePrefix(delta int) {
	ps := m.cfg.Prefixes
	i := m.prefixesList.Index()
	j := i + delta
	if i < 0 || j < 0 || j >= len(ps) {
		return
	}
	m.pushUndo()
	ps[i], ps[j] = ps[j], ps[i]
	m.persist()
	m.refreshPrefixList()
	m.prefixesList.Select(j)
}

func (m *model) moveCommand(delta int) {
	cs := *m.childrenPtr()
	i := m.commandsList.Index()
	j := i + delta
	if i < 0 || j < 0 || j >= len(cs) {
		return
	}
	m.pushUndo()
	cs[i], cs[j] = cs[j], cs[i]
	m.persist()
	m.refreshCommandList()
	m.commandsList.Select(j)
}

func (m *model) moveFlag(delta int) {
	cmd := m.commandAt(m.flagCmdPath)
	if cmd == nil {
		return
	}
	fs := cmd.Flags
	i := m.flagsList.Index()
	j := i + delta
	if i < 0 || j < 0 || j >= len(fs) {
		return
	}
	m.pushUndo()
	fs[i], fs[j] = fs[j], fs[i]
	m.persist()
	m.refreshFlagList()
	m.flagsList.Select(j)
}

// importAliases читает alias из ~/.zshrc и добавляет их командами в префикс-
// контейнер importedPrefix. Невалидные по имени команды и уже существующие
// пропускаются; пустой контейнер не создаётся.
func (m *model) importAliases() {
	aliases, err := importer.LoadAliases()
	if err != nil {
		m.status = "Импорт: не удалось прочитать ~/.zshrc: " + err.Error()
		m.statusErr = true
		return
	}

	// Находим контейнер (если уже есть) и собираем имена существующих команд.
	idx := -1
	existing := map[string]bool{}
	for i, p := range m.cfg.Prefixes {
		if p.Name == importedPrefix {
			idx = i
			for _, c := range p.Commands {
				existing[c.Name] = true
			}
			break
		}
	}

	// Отбираем пригодные к импорту до любых мутаций.
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
		m.status = fmt.Sprintf("Импорт: нечего добавлять (пропущено %d)", skipped)
		m.statusErr = false
		return
	}

	m.pushUndo()
	if idx == -1 {
		m.cfg.Prefixes = append(m.cfg.Prefixes, models.Prefix{
			Name: importedPrefix, Description: "Импортированные алиасы из ~/.zshrc",
		})
		idx = len(m.cfg.Prefixes) - 1
	}
	m.cfg.Prefixes[idx].Commands = append(m.cfg.Prefixes[idx].Commands, toAdd...)
	m.persist()
	m.refreshPrefixList()
	m.status = fmt.Sprintf("Импорт: добавлено %d, пропущено %d", len(toAdd), skipped)
	m.statusErr = false
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
	m.pushUndo()
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
		children := m.childrenPtr()
		if m.formMode == modeEdit {
			old := (*children)[m.activeCommandIdx]
			c.Flags = old.Flags             // сохраняем существующие флаги
			c.Subcommands = old.Subcommands // и вложенные подкоманды
			(*children)[m.activeCommandIdx] = c
		} else {
			*children = append(*children, c)
		}
		m.persist()
		m.refreshCommandList()
		m.state = stateCommandList

	case stateFlagForm:
		if cmd := m.commandAt(m.flagCmdPath); cmd != nil {
			f := models.Flag{
				Name:        m.fb.name,
				Description: m.fb.description,
				Run:         m.fb.run,
			}
			if m.formMode == modeEdit {
				var idx int
				fmt.Sscanf(m.editKey, "%d", &idx)
				cmd.Flags[idx] = f
			} else {
				cmd.Flags = append(cmd.Flags, f)
			}
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
			"[n] новый  [e] изменить  [d] удалить  [⇧↑/↓] порядок  [p] zsh  [i] импорт  [u] отмена  [enter] открыть  [esc] меню")
	case stateCommandList:
		return m.listView(m.commandsList.View(),
			"[n] новая  [e] изменить  [d] удалить  [⇧↑/↓] порядок  [f] флаги  [p] zsh  [u] отмена  [enter] вглубь  [esc] назад")
	case stateFlagList:
		return m.listView(m.flagsList.View(),
			"[n] новый  [e] изменить  [d] удалить  [⇧↑/↓] порядок  [u] отмена  [esc] назад")
	case stateVariableList:
		return m.listView(m.variablesList.View(),
			"[n] новая  [e] изменить  [d] удалить  [u] отмена  [esc] меню")
	case statePreview:
		return m.viewPreview()
	case statePrefixForm, stateCommandForm, stateFlagForm, stateVariableForm:
		return m.viewForm()
	}
	return ""
}

// viewPreview отрисовывает экран предпросмотра сгенерированного zsh с заголовком,
// путём итогового файла (№7) и прокручиваемым содержимым.
func (m model) viewPreview() string {
	path := compiler.For(m.cfg.Shell).OutputPath()
	header := headerStyle.Render(m.previewTitle) + "\n" +
		hintStyle.Render("Файл: "+path)
	hints := hintStyle.Render("[↑/↓] прокрутка  [esc] назад")
	return docStyle.Render(header + "\n\n" + m.previewVP.View() + "\n\n" + hints)
}

func (m model) viewMainMenu() string {
	var b strings.Builder
	b.WriteString(headerStyle.Render("CLI Manager") + "\n")
	b.WriteString(hintStyle.Render("Конструктор составных команд для "+strings.ToUpper(m.shellName())) + "\n\n")
	for i, item := range m.menuItems() {
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

// variablesPanel возвращает подсказку со списком определённых глобальных
// переменных в форме ${NAME} — чтобы при наборе действия их не приходилось
// держать в голове. Пусто, если переменных нет.
func (m model) variablesPanel() string {
	if len(m.cfg.Variables) == 0 {
		return ""
	}
	keys := make([]string, 0, len(m.cfg.Variables))
	for k := range m.cfg.Variables {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	chips := make([]string, len(keys))
	for i, k := range keys {
		chips[i] = "${" + k + "}"
	}
	return hintStyle.Render("Переменные: " + strings.Join(chips, "  "))
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

	if m.state == stateCommandForm || m.state == stateFlagForm {
		if panel := m.variablesPanel(); panel != "" {
			b.WriteString("\n" + panel)
		}
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
