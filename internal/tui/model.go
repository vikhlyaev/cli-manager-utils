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

	"cli-manager/internal/models"
	"cli-manager/internal/validation"
)

// importedPrefix — имя префикса-контейнера, в который складываются alias,
// импортированные из ~/.zshrc.
const importedPrefix = "imported"

// version — версия приложения, отображается в заголовке главного экрана.
const version = "1.0.0"

// undoLimit — максимальная глубина undo-стека.
const undoLimit = 50

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

// formReturn — экран-список, на который возвращается каждая форма после
// сохранения или отмены. Ключи этой карты — ровно все состояния-формы.
var formReturn = map[sessionState]sessionState{
	statePrefixForm:   statePrefixList,
	stateCommandForm:  stateCommandList,
	stateFlagForm:     stateFlagList,
	stateVariableForm: stateVariableList,
}

func (s sessionState) isForm() bool {
	_, ok := formReturn[s]
	return ok
}

// hasRunField сообщает, есть ли у формы поле bash-кода run (команда и флаг).
func (s sessionState) hasRunField() bool {
	return s == stateCommandForm || s == stateFlagForm
}

// formMode различает создание нового элемента и редактирование существующего.
type formMode int

const (
	modeCreate formMode = iota
	modeEdit
)

type model struct {
	state sessionState
	cfg   *models.Config

	menuIndex int

	prefixesList  list.Model
	commandsList  list.Model
	flagsList     list.Model
	variablesList list.Model

	activePrefixIdx int   // префикс, чьи команды открыты
	cmdPath         []int // путь индексов до узла, чьи дети показаны в списке команд
	flagCmdPath     []int // путь до команды, чьи флаги показаны в списке флагов

	// Форма и её буферы.
	form     *huh.Form
	formMode formMode
	editIdx  int          // индекс редактируемого префикса/команды/флага на своём уровне
	editKey  string       // исходный ключ редактируемой переменной
	fb       *formBuffers // привязанные к форме значения (стабильный указатель)

	// Живые проверки формы. lastName/lastRun — значения, для которых проверка
	// уже выполнена: bash запускается только когда поле действительно изменилось.
	collision  string          // предупреждение: имя префикса перекрывает команду или alias
	syntaxErr  string          // ошибка `bash -n` для поля run (не блокирует ввод)
	aliases    map[string]bool // alias из rc-файла; читается один раз на форму
	lastName   string
	lastRun    string
	testOutput string
	testErr    bool

	status    string
	statusErr bool

	// Подтверждение удаления.
	confirmDelete bool
	deleteLabel   string // что именно удаляем — для текста запроса

	// Предпросмотр сгенерированного скрипта.
	previewVP     viewport.Model
	previewReady  bool         // viewport получил размеры (WindowSizeMsg)
	previewTitle  string       // заголовок над окном предпросмотра
	previousState sessionState // куда вернуться по Esc из предпросмотра

	// Снимки конфигурации до каждой мутации (последний — самый свежий).
	undoStack []*models.Config
}

func newList() list.Model {
	l := list.New([]list.Item{}, list.NewDefaultDelegate(), 0, 0)
	// Заголовок, статус-бар, встроенную справку и пустое состояние bubbles
	// отключаем — свою шапку, подсказки и текст пустого экрана рисует listScreen.
	l.SetShowTitle(false)
	l.SetShowStatusBar(false)
	l.SetShowHelp(false)
	l.SetFilteringEnabled(false)
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
		prefixesList:  newList(),
		commandsList:  newList(),
		flagsList:     newList(),
		variablesList: newList(),
	}
	m.refreshPrefixList()
	m.refreshVariableList()
	return m
}

func (m model) Init() tea.Cmd { return nil }

// shellName возвращает нормализованное имя целевой оболочки ("zsh" по умолчанию).
func (m model) shellName() string {
	if strings.ToLower(strings.TrimSpace(m.cfg.Shell)) == "bash" {
		return "bash"
	}
	return "zsh"
}

// menuItems формирует пункты главного меню; третий пункт показывает и
// переключает целевую оболочку.
func (m model) menuItems() []string {
	return []string{"Префиксы", "Глобальные переменные", "Оболочка: " + m.shellName()}
}

// sortedKeys возвращает ключи карты в отсортированном порядке.
func sortedKeys(mp map[string]string) []string {
	keys := make([]string, 0, len(mp))
	for k := range mp {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func inRange(i, n int) bool { return i >= 0 && i < n }

// Run запускает TUI-приложение.
func Run(cfg *models.Config) {
	m := NewModel(cfg)
	if ps := validation.ValidateConfig(cfg); len(ps) > 0 {
		m.setError(fmt.Sprintf("⚠ В конфиге проблем: %d (первая — %s). Подробнее: cli-manager doctor", len(ps), ps[0]))
	}
	p := tea.NewProgram(m, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "Ошибка при запуске TUI:", err)
		os.Exit(1)
	}
}
