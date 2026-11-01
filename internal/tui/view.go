package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/list"

	"cli-manager/internal/buildinfo"
	"cli-manager/internal/compiler"
	"cli-manager/internal/importer"
)

func (m model) View() string {
	switch m.state {
	case stateMainMenu:
		return m.viewMainMenu()
	case statePrefixList:
		return m.listScreen(m.prefixesList,
			"CLI Manager · Префиксы", "наборы команд верхнего уровня",
			"Пока пусто. Нажмите [n], чтобы создать первый префикс.",
			"[enter] открыть  [n] новый  [e] изменить  [d] удалить  [p] "+m.shellName()+"  [?] все клавиши  [esc] меню")
	case stateCommandList:
		return m.listScreen(m.commandsList,
			m.breadcrumb(m.cmdPath)+" · команды", "Enter: лист → флаги, ветвь → вглубь",
			"Команд нет. Нажмите [n], чтобы добавить команду.",
			"[enter] открыть  [s] подкоманды  [f] флаги  [n] новая  [e] изменить  [d] удалить  [?] все клавиши  [esc] назад")
	case stateFlagList:
		return m.listScreen(m.flagsList,
			m.breadcrumb(m.flagCmdPath)+" · флаги", "опции команды-листа",
			"Флагов нет. Нажмите [n], чтобы добавить флаг.",
			"[n] новый  [e] изменить  [d] удалить  [⇧↑/↓] порядок  [?] все клавиши  [esc] назад")
	case stateVariableList:
		return m.listScreen(m.variablesList,
			"CLI Manager · Глобальные переменные", "ключ → значение, экспортируются в скрипт",
			"Переменных нет. Нажмите [n], чтобы добавить переменную.",
			"[n] новая  [e] изменить  [d] удалить  [u] отмена  [?] все клавиши  [esc] меню")
	case statePreview:
		return m.viewPreview()
	}
	if m.state.isForm() {
		return m.viewForm()
	}
	return ""
}

func (m model) viewMainMenu() string {
	var b strings.Builder
	b.WriteString(headerStyle.Render("CLI Manager") + " " + hintStyle.Render(buildinfo.Version) + "\n")
	b.WriteString(hintStyle.Render("Конструктор составных команд для "+strings.ToUpper(m.shellName())) + "\n\n")
	for i, item := range m.menuItems() {
		cursor, line := "  ", item
		if i == m.menuIndex {
			cursor, line = cursorStyle.Render("› "), cursorStyle.Render(item)
		}
		b.WriteString(cursor + line + "\n")
	}
	b.WriteString("\n" + hintStyle.Render("[↑/↓] выбор  [enter] открыть  [?] клавиши  [q] выход"))
	if m.configWarn != "" {
		b.WriteString("\n\n" + warnStyle.Render(m.configWarn))
	}
	if m.notSourced {
		b.WriteString("\n\n" + warnStyle.Render("⚠ Скрипт не подключён к "+importer.RCPath(m.shellName())+
			" — выполните: cli-manager init"))
	}
	if m.status != "" {
		b.WriteString("\n\n" + m.statusLine())
	}
	return docStyle.Render(b.String())
}

// listScreen рисует экран-список: шапка («где ты»), тело — список или текст
// пустого состояния («что делать»), строка подсказок или запрос удаления.
func (m model) listScreen(l list.Model, title, subtitle, emptyHint, hints string) string {
	var b strings.Builder

	b.WriteString(headerStyle.Render(title))
	if subtitle != "" {
		b.WriteString("  " + hintStyle.Render("· "+subtitle))
	}
	b.WriteString("\n\n")

	if len(l.Items()) == 0 {
		b.WriteString(hintStyle.Render(emptyHint))
	} else {
		b.WriteString(l.View())
	}

	b.WriteString("\n\n")
	if m.confirmDelete {
		b.WriteString(warnStyle.Render(fmt.Sprintf("Удалить %s? ", m.deleteLabel)) +
			hintStyle.Render("[y] да  [n] нет"))
	} else {
		b.WriteString(hintStyle.Render(hints))
	}
	if m.status != "" {
		b.WriteString("\n" + m.statusLine())
	}
	return docStyle.Render(b.String())
}

// viewPreview рисует предпросмотр сгенерированного скрипта с путём итогового
// файла и прокручиваемым содержимым.
func (m model) viewPreview() string {
	path := compiler.For(m.cfg.Shell).OutputPath()
	header := headerStyle.Render(m.previewTitle) + "\n" + hintStyle.Render("Файл: "+path)
	hints := hintStyle.Render("[↑/↓] прокрутка  [esc] назад")
	if m.previewTitle == helpTitle {
		return docStyle.Render(headerStyle.Render(m.previewTitle) + "\n\n" + m.previewVP.View() + "\n\n" + hints)
	}
	return docStyle.Render(header + "\n\n" + m.previewVP.View() + "\n\n" + hints)
}

// formTitles — заголовки форм: [создание, редактирование].
var formTitles = map[sessionState][2]string{
	statePrefixForm:   {"Новый префикс", "Изменение префикса"},
	stateCommandForm:  {"Новая команда", "Изменение команды"},
	stateFlagForm:     {"Новый флаг", "Изменение флага"},
	stateVariableForm: {"Новая переменная", "Изменение переменной"},
}

// formHeader строит заголовок формы: что создаём/редактируем и где в дереве.
func (m model) formHeader() string {
	title := formTitles[m.state][0]
	if m.formMode == modeEdit {
		title = formTitles[m.state][1]
	}
	var ctx string
	switch m.state {
	case statePrefixForm:
		ctx = "верхний уровень"
	case stateCommandForm:
		ctx = "в " + m.breadcrumb(m.cmdPath)
	case stateFlagForm:
		ctx = "у команды " + m.breadcrumb(m.flagCmdPath)
	case stateVariableForm:
		ctx = "глобальная переменная"
	}
	return headerStyle.Render(title) + "  " + hintStyle.Render("· "+ctx)
}

func (m model) viewForm() string {
	var b strings.Builder
	b.WriteString(m.formHeader() + "\n\n")
	b.WriteString(m.form.View())

	if m.state == statePrefixForm && m.collision != "" {
		b.WriteString("\n" + warnStyle.Render("⚠️  "+m.collision))
	}

	if m.state.hasRunField() {
		if m.syntaxErr != "" {
			b.WriteString("\n" + errStyle.Render("✗ синтаксис bash: "+m.syntaxErr))
		}
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

// variablesPanel перечисляет глобальные переменные в форме ${NAME}, чтобы при
// наборе действия их не держать в голове. Пусто, если переменных нет.
func (m model) variablesPanel() string {
	if len(m.cfg.Variables) == 0 {
		return ""
	}
	keys := sortedKeys(m.cfg.Variables)
	for i, k := range keys {
		keys[i] = "${" + k + "}"
	}
	return hintStyle.Render("Переменные: " + strings.Join(keys, "  "))
}

func (m model) statusLine() string {
	if m.statusErr {
		return errStyle.Render(m.status)
	}
	return okStyle.Render(m.status)
}
