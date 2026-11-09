package tui

import (
	"fmt"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"

	"cli-manager/internal/models"
	"cli-manager/internal/validation"
)

// formBuffers хранит значения, привязанные к полям формы huh.
//
// Важно: huh пишет значение прямо по переданному указателю (Value(&...)).
// Bubble Tea копирует модель по значению на каждом Update, поэтому привязка к
// полям самой модели даёт указатель на устаревшую копию — значения теряются.
// Держим буферы в отдельной структуре, а модель ссылается на неё указателем:
// все копии модели делят один и тот же буфер, и указатели остаются стабильными.
type formBuffers struct {
	name        string
	description string
	run         string
	value       string
	condition   string
	usage       string
	acceptsArgs bool
	afterCmd    bool // флаг/подкоманда: сначала действие команды-родителя
}

func (fb *formBuffers) loadCommand(c models.Command) {
	fb.name, fb.description, fb.run = c.Name, c.Description, c.Run
	fb.acceptsArgs, fb.condition, fb.usage = c.AcceptsArgs, c.Condition, c.Usage
	fb.afterCmd = c.AfterCommand
}

// command собирает команду из буфера (без флагов и подкоманд).
func (fb *formBuffers) command() models.Command {
	return models.Command{
		Name:         fb.name,
		Description:  fb.description,
		Run:          trimRun(fb.run),
		AcceptsArgs:  fb.acceptsArgs,
		Condition:    fb.condition,
		Usage:        strings.TrimSpace(fb.usage),
		AfterCommand: fb.afterCmd,
	}
}

// trimRun убирает хвостовые переводы строки (их дописывают внешние редакторы).
func trimRun(s string) string { return strings.TrimRight(s, "\n") }

func (fb *formBuffers) loadFlag(f models.Flag) {
	fb.name, fb.description, fb.run = f.Name, f.Description, f.Run
	fb.afterCmd = f.AfterCommand
}

func (fb *formBuffers) flag() models.Flag {
	return models.Flag{Name: fb.name, Description: fb.description, Run: trimRun(fb.run), AfterCommand: fb.afterCmd}
}

// validatePrefixName проверяет формат имени префикса (имя zsh-функции) и его
// уникальность среди существующих префиксов (исключая редактируемый).
func (m *model) validatePrefixName(s string) error {
	if err := validation.ValidateIdentifier(s); err != nil {
		return err
	}
	for i, p := range m.cfg.Prefixes {
		if !m.isEdited(i) && p.Name == s {
			return fmt.Errorf("префикс %q уже существует", s)
		}
	}
	return nil
}

// validateCommandName проверяет формат имени команды и уникальность на текущем
// уровне дерева (исключая редактируемую команду).
func (m *model) validateCommandName(s string) error {
	if err := validation.ValidateCommandName(s); err != nil {
		return err
	}
	for i, c := range *m.childrenPtr() {
		if !m.isEdited(i) && c.Name == s {
			return fmt.Errorf("команда %q уже существует на этом уровне", s)
		}
	}
	return nil
}

// validateFlagName проверяет формат имени флага и уникальность среди флагов
// открытой команды (исключая редактируемый флаг).
func (m *model) validateFlagName(s string) error {
	if err := validation.ValidateFlagName(s); err != nil {
		return err
	}
	cmd := m.commandAt(m.flagCmdPath)
	if cmd == nil {
		return nil
	}
	for i, f := range cmd.Flags {
		if !m.isEdited(i) && f.Name == s {
			return fmt.Errorf("флаг %q уже существует у команды", s)
		}
	}
	return nil
}

// isEdited сообщает, что элемент с индексом i — тот, который сейчас правится
// (его имя не конфликтует само с собой).
func (m *model) isEdited(i int) bool {
	return m.formMode == modeEdit && i == m.editIdx
}

// validateVariableName проверяет формат имени переменной и уникальность ключа
// (исключая редактируемый ключ — переименование на прежнее имя допустимо).
func (m *model) validateVariableName(s string) error {
	if err := validation.ValidateIdentifier(s); err != nil {
		return err
	}
	if s == m.editKey {
		return nil // имя не менялось
	}
	if _, exists := m.cfg.Variables[s]; exists {
		return fmt.Errorf("переменная %q уже существует", s)
	}
	return nil
}

// validateCondition пропускает пустое условие, иначе проверяет его синтаксис.
func validateCondition(s string) error {
	return validation.CheckCondition(s)
}

// runField — многострочное поле bash-кода. Ctrl+E (когда поле в фокусе)
// открывает код во внешнем редакторе — это умеет сам huh; задаём только
// редактор и расширение .sh для подсветки синтаксиса.
func runField(v *string) *huh.Text {
	return huh.NewText().
		Key("run").
		Title("Действие (bash) · Ctrl+T — тест · Ctrl+E — редактор").
		Value(v).
		Lines(3).
		Editor(editorCommand()...).
		EditorExtension("sh")
}

// editorCommand возвращает внешний редактор: $VISUAL, затем $EDITOR, иначе nano.
func editorCommand() []string {
	for _, env := range []string{"VISUAL", "EDITOR"} {
		if f := strings.Fields(os.Getenv(env)); len(f) > 0 {
			return f
		}
	}
	return []string{"nano"}
}

// initPrefixForm — форма создания/редактирования префикса.
// Буфер m.fb должен быть подготовлен (и при редактировании заполнен) заранее.
func (m *model) initPrefixForm() tea.Cmd {
	m.form = huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Key("name").
				Title("Имя префикса").
				Description("Корневой шорткат, например: av. Проверяется на коллизию с командами и alias").
				Value(&m.fb.name).
				Validate(m.validatePrefixName),
			huh.NewInput().
				Key("description").
				Title("Описание").
				Value(&m.fb.description),
		),
	).WithShowHelp(false)
	return m.form.Init()
}

// initCommandForm — форма создания/редактирования команды.
func (m *model) initCommandForm() tea.Cmd {
	m.form = huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Key("name").
				Title("Имя команды").
				Value(&m.fb.name).
				Validate(m.validateCommandName),
			huh.NewInput().
				Key("description").
				Title("Описание").
				Value(&m.fb.description),
			runField(&m.fb.run),
			huh.NewConfirm().
				Key("accepts_args").
				Title("Принимает аргументы ($@)?").
				Description("В run можно обращаться к ним по номеру: rake loc=\"$1\"").
				Value(&m.fb.acceptsArgs),
			huh.NewInput().
				Key("usage").
				Title("Аргументы для справки (необязательно)").
				Description("Напр.: <файл> [строки] — показывается в av --help; <…> обязательны").
				Value(&m.fb.usage),
			huh.NewConfirm().
				Key("after_command").
				Title("Сначала выполнить действие родительской команды?").
				Description("Для подкоманды: напр. родитель делает cd, а подкоманда запускает в этой папке rake").
				Value(&m.fb.afterCmd),
			huh.NewInput().
				Key("condition").
				Title("Условие автодополнения (необязательно)").
				Description(`Напр.: [[ -f docker-compose.yml ]]`).
				Value(&m.fb.condition).
				Validate(validateCondition),
		),
	).WithShowHelp(false)
	return m.form.Init()
}

// initFlagForm — форма создания/редактирования флага.
func (m *model) initFlagForm() tea.Cmd {
	m.form = huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Key("name").
				Title("Флаг").
				Description("Например: --force").
				Value(&m.fb.name).
				Validate(m.validateFlagName),
			huh.NewInput().
				Key("description").
				Title("Описание").
				Value(&m.fb.description),
			runField(&m.fb.run),
			huh.NewConfirm().
				Key("after_command").
				Title("Сначала выполнить действие команды?").
				Description("Да — напр. команда делает cd, а флаг запускает инструмент в этой папке. Нет — флаг заменяет действие команды").
				Value(&m.fb.afterCmd),
		),
	).WithShowHelp(false)
	return m.form.Init()
}

// initVariableForm — форма создания/редактирования глобальной переменной.
func (m *model) initVariableForm() tea.Cmd {
	m.form = huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Key("name").
				Title("Имя переменной").
				Description("Напр.: VAULT_DIR").
				Value(&m.fb.name).
				Validate(m.validateVariableName),
			huh.NewInput().
				Key("value").
				Title("Значение").
				Value(&m.fb.value),
		),
	).WithShowHelp(false)
	return m.form.Init()
}
