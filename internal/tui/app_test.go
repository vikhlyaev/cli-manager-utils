package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"cli-manager/internal/models"
)

// feed прогоняет последовательность сообщений через Update, возвращая итоговую модель.
func feed(m model, msgs ...tea.Msg) model {
	for _, msg := range msgs {
		tm, _ := m.Update(msg)
		m = tm.(model)
	}
	return m
}

func runes(s string) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

// TestFormBufferCapturesInput — ядро регрессии: ввод в поле формы должен
// сохраняться в стабильном буфере, переживая копирование модели в Update.
func TestFormBufferCapturesInput(t *testing.T) {
	m := NewModel(&models.Config{Variables: map[string]string{}})
	m = feed(m,
		tea.WindowSizeMsg{Width: 80, Height: 24},
		tea.KeyMsg{Type: tea.KeyEnter}, // главное меню -> Префиксы
		runes("n"),                     // открыть форму нового префикса
		runes("av"),                    // ввести имя
	)
	if m.fb == nil {
		t.Fatal("буфер формы не инициализирован")
	}
	if m.fb.name != "av" {
		t.Fatalf("ожидалось, что буфер захватит 'av', получено %q", m.fb.name)
	}
}

// TestCommitPrefixWritesBuffer — путь сохранения: при завершении формы в конфиг
// попадает именно введённое в поля значение (а не пустая строка).
func TestCommitPrefixWritesBuffer(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := NewModel(&models.Config{Variables: map[string]string{}})
	m = feed(m,
		tea.WindowSizeMsg{Width: 80, Height: 24},
		tea.KeyMsg{Type: tea.KeyEnter}, // -> Префиксы
		runes("n"),                     // новая форма
		runes("av"),                    // ввод имени в первое поле
	)
	// Имитируем завершение формы (huh.StateCompleted) — то, что делает Update.
	m.commitForm()

	if got := len(m.cfg.Prefixes); got != 1 {
		t.Fatalf("ожидался 1 префикс, получено %d", got)
	}
	if m.cfg.Prefixes[0].Name != "av" {
		t.Errorf("имя префикса: ожидалось 'av', получено %q", m.cfg.Prefixes[0].Name)
	}
	if m.state != statePrefixList {
		t.Errorf("после сохранения ожидалось возвращение к списку префиксов")
	}
}

// TestDeleteRequiresConfirmation — нажатие `d` не удаляет сразу, а переводит в
// режим подтверждения; удаление происходит только после `y`.
func TestDeleteRequiresConfirmation(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	cfg := &models.Config{
		Variables: map[string]string{},
		Prefixes:  []models.Prefix{{Name: "av"}, {Name: "gx"}},
	}
	m := NewModel(cfg)
	m = feed(m,
		tea.WindowSizeMsg{Width: 80, Height: 24},
		tea.KeyMsg{Type: tea.KeyEnter}, // -> Префиксы
		runes("d"),                     // запрос на удаление
	)
	if !m.confirmDelete {
		t.Fatal("после 'd' ожидался режим подтверждения")
	}
	if len(m.cfg.Prefixes) != 2 {
		t.Fatalf("до подтверждения ничего не должно удаляться, осталось %d", len(m.cfg.Prefixes))
	}

	// Отмена возвращает к списку без удаления.
	mc := feed(m, runes("n"))
	if mc.confirmDelete || len(mc.cfg.Prefixes) != 2 {
		t.Fatalf("после 'n' ожидалась отмена без удаления, осталось %d", len(mc.cfg.Prefixes))
	}

	// Подтверждение удаляет выбранный элемент.
	my := feed(m, runes("y"))
	if my.confirmDelete {
		t.Error("после 'y' режим подтверждения должен сброситься")
	}
	if len(my.cfg.Prefixes) != 1 || my.cfg.Prefixes[0].Name != "gx" {
		t.Fatalf("ожидался один префикс 'gx', получено %+v", my.cfg.Prefixes)
	}
}

// TestEditPrefixPrefills — при редактировании буфер формы предзаполняется текущими
// значениями (раньше они подставлялись после построения формы и терялись).
func TestEditPrefixPrefills(t *testing.T) {
	cfg := &models.Config{
		Variables: map[string]string{},
		Prefixes:  []models.Prefix{{Name: "av", Description: "тулкит"}},
	}
	m := NewModel(cfg)
	m = feed(m,
		tea.WindowSizeMsg{Width: 80, Height: 24},
		tea.KeyMsg{Type: tea.KeyEnter}, // -> Префиксы
		runes("e"),                     // редактировать выбранный
	)
	if m.fb == nil || m.fb.name != "av" || m.fb.description != "тулкит" {
		t.Fatalf("форма редактирования не предзаполнена: %+v", m.fb)
	}
}
