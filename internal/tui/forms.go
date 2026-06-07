package tui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
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

func notEmpty(label string) func(string) error {
	return func(s string) error {
		if s == "" {
			return fmt.Errorf("%s не может быть пустым", label)
		}
		return nil
	}
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
				Validate(notEmpty("имя")),
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
				Validate(notEmpty("имя")),
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
				Value(&m.fb.condition),
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
				Validate(notEmpty("имя")),
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
				Validate(notEmpty("имя")),
			huh.NewInput().
				Key("value").
				Title("Значение").
				Value(&m.fb.value),
		),
	).WithShowHelp(false)
	return m.form.Init()
}
