package tui

import (
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
)

// Update — корневой обработчик сообщений. Порядок приоритетов:
// Ctrl+C → размер окна → форма → подтверждение удаления → предпросмотр →
// клавиши текущего экрана → навигация в активном списке.
func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok && key.String() == "ctrl+c" {
		return m, tea.Quit
	}
	if size, ok := msg.(tea.WindowSizeMsg); ok {
		m.resize(size) // и дальше: форма тоже должна узнать о размере
	}

	switch {
	case m.state.isForm():
		return m.updateForm(msg)
	case m.confirmDelete:
		m.updateConfirmDelete(msg)
		return m, nil
	case m.state == statePreview:
		return m.updatePreview(msg)
	}

	if key, ok := msg.(tea.KeyMsg); ok {
		if m.state == stateMainMenu {
			return m.updateMainMenu(key)
		}
		if cmd, handled := m.updateListKey(key); handled {
			return m, cmd
		}
	}

	var cmd tea.Cmd
	if l := m.activeList(); l != nil {
		*l, cmd = l.Update(msg)
	}
	return m, cmd
}

// resize подгоняет списки и окно предпросмотра под новый размер терминала.
func (m *model) resize(size tea.WindowSizeMsg) {
	h, v := docStyle.GetFrameSize()
	w := size.Width - h
	bodyH := max(size.Height-v-chromeLines, 1)

	for _, l := range []*list.Model{&m.prefixesList, &m.commandsList, &m.flagsList, &m.variablesList} {
		l.SetSize(w, bodyH)
	}

	if !m.previewReady {
		m.previewVP = viewport.New(w, bodyH)
		m.previewReady = true
	} else {
		m.previewVP.Width, m.previewVP.Height = w, bodyH
	}
}

// updateConfirmDelete ждёт ответа на запрос удаления; остальной ввод игнорируется.
func (m *model) updateConfirmDelete(msg tea.Msg) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return
	}
	switch key.String() {
	case "y", "Y", "enter":
		m.performDelete()
	case "n", "N", "esc":
		m.confirmDelete, m.deleteLabel = false, ""
	}
}

// updatePreview: Esc/q возвращает назад, остальное — прокрутка viewport.
func (m model) updatePreview(msg tea.Msg) (tea.Model, tea.Cmd) {
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
