package tui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"

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
	acceptsArgs bool
}

// validatePrefixName проверяет формат имени префикса (имя zsh-функции) и его
// уникальность среди существующих префиксов (исключая редактируемый).
func (m *model) validatePrefixName(s string) error {
	if err := validation.ValidateIdentifier(s); err != nil {
		return err
	}
	for i, p := range m.cfg.Prefixes {
		if m.formMode == modeEdit && i == m.activePrefixIdx {
			continue
		}
		if p.Name == s {
			return fmt.Errorf("префикс %q уже существует", s)
		}
	}
	return nil
}

// validateCommandName проверяет формат имени команды и уникальность в пределах
// активного префикса (исключая редактируемую команду).
func (m *model) validateCommandName(s string) error {
	if err := validation.ValidateCommandName(s); err != nil {
		return err
	}
	for i, c := range m.cfg.Prefixes[m.activePrefixIdx].Commands {
		if m.formMode == modeEdit && i == m.activeCommandIdx {
			continue
		}
		if c.Name == s {
			return fmt.Errorf("команда %q уже существует в префиксе", s)
		}
	}
	return nil
}

// validateFlagName проверяет формат имени флага и уникальность в пределах
// активной команды (исключая редактируемый флаг).
func (m *model) validateFlagName(s string) error {
	if err := validation.ValidateFlagName(s); err != nil {
		return err
	}
	var editIdx = -1
	if m.formMode == modeEdit {
		fmt.Sscanf(m.editKey, "%d", &editIdx)
	}
	for i, f := range m.cfg.Prefixes[m.activePrefixIdx].Commands[m.activeCommandIdx].Flags {
		if i == editIdx {
			continue
		}
		if f.Name == s {
			return fmt.Errorf("флаг %q уже существует у команды", s)
		}
	}
	return nil
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

// initPrefixForm — форма создания/редактирования префикса.
// Буфер m.fb должен быть подготовлен (и при редактировании заполнен) заранее.
func (m *model) initPrefixForm() tea.Cmd {
	m.form = huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Key("name").
				Title("Имя префикса").
				Description("Корневой шорткат, например: av").
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
				Description("Будет проверено на коллизию с системными командами").
				Value(&m.fb.name).
				Validate(m.validateCommandName),
			huh.NewInput().
				Key("description").
				Title("Описание").
				Value(&m.fb.description),
			huh.NewText().
				Key("run").
				Title("Действие (bash) · Ctrl+T — тест").
				Value(&m.fb.run).
				Lines(3),
			huh.NewConfirm().
				Key("accepts_args").
				Title("Принимает аргументы ($@)?").
				Value(&m.fb.acceptsArgs),
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
			huh.NewText().
				Key("run").
				Title("Действие (bash) · Ctrl+T — тест").
				Value(&m.fb.run).
				Lines(3),
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
