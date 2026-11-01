package tui

import (
	"fmt"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"

	"cli-manager/internal/importer"
	"cli-manager/internal/models"
	"cli-manager/internal/validation"
)

// Жизненный цикл формы: newFormBuffers → (заполнение буфера при правке) →
// startForm → updateForm на каждое сообщение → commitForm или closeForm.

// newFormBuffers создаёт свежий буфер формы (на новом указателе) и сбрасывает
// связанное с формой состояние. Вызывается перед открытием любой формы.
func (m *model) newFormBuffers() {
	m.fb = &formBuffers{}
	m.editIdx = -1
	m.editKey = ""
	m.lastName, m.lastRun = "", ""
	m.aliases = nil
	m.clearFormFeedback()
}

func (m *model) clearFormFeedback() {
	m.collision = ""
	m.syntaxErr = ""
	m.testOutput = ""
	m.testErr = false
}

// startForm строит форму для state и переключается на неё. Буфер m.fb должен
// быть подготовлен заранее.
func (m *model) startForm(state sessionState, mode formMode) tea.Cmd {
	m.formMode = mode
	m.state = state
	switch state {
	case statePrefixForm:
		return m.initPrefixForm()
	case stateCommandForm:
		return m.initCommandForm()
	case stateFlagForm:
		return m.initFlagForm()
	case stateVariableForm:
		return m.initVariableForm()
	}
	return nil
}

func (m model) updateForm(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case "esc":
			m.closeForm()
			return m, nil
		case "ctrl+t":
			if m.state.hasRunField() {
				m.runTest()
				return m, nil
			}
		}
	}

	f, cmd := m.form.Update(msg)
	if form, ok := f.(*huh.Form); ok {
		m.form = form
	}
	m.liveChecks()

	if m.form.State == huh.StateCompleted {
		m.commitForm()
		return m, nil
	}
	return m, cmd
}

// liveChecks обновляет неблокирующие подсказки: коллизию имени префикса и
// синтаксис bash в поле run. Проверки запускаются только когда поле изменилось.
func (m *model) liveChecks() {
	if m.state == statePrefixForm && m.fb.name != m.lastName {
		m.lastName = m.fb.name
		m.collision = m.prefixCollision(m.fb.name)
	}
	if m.state.hasRunField() && m.fb.run != m.lastRun {
		m.lastRun = m.fb.run
		m.syntaxErr = ""
		if err := validation.CheckSyntax(m.fb.run); err != nil {
			m.syntaxErr = err.Error()
		}
	}
}

// prefixCollision сообщает, что перекроет префикс с именем name. Префикс —
// это shell-функция, поэтому важны совпадения с командами из PATH/builtin и с
// alias из rc-файла выбранной оболочки (при вызове alias срабатывает раньше
// функции). Имена команд внутри префикса — лишь аргументы функции.
func (m *model) prefixCollision(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	if validation.CheckCollision(name) {
		return fmt.Sprintf("'%s' уже есть в системе — префикс перекроет эту команду", name)
	}
	rc := importer.RCPath(m.shellName())
	if m.aliases == nil {
		m.aliases = map[string]bool{}
		if list, err := importer.LoadAliasesFrom(rc); err == nil {
			for _, a := range list {
				m.aliases[a.Name] = true
			}
		}
	}
	if m.aliases[name] {
		return fmt.Sprintf("в %s есть alias '%s' — при вызове сработает он, а не префикс",
			strings.Replace(rc, homeDir(), "~", 1), name)
	}
	return ""
}

func homeDir() string {
	h, _ := os.UserHomeDir()
	return h
}

// runTest выполняет bash-код из поля run (Ctrl+T) и сохраняет вывод для показа.
func (m *model) runTest() {
	out, err := validation.ValidateBash(m.fb.run)
	m.testOutput = strings.TrimRight(out, "\n")
	m.testErr = err != nil
	if m.testOutput != "" {
		return
	}
	if err != nil {
		m.testOutput = err.Error()
	} else {
		m.testOutput = "(пустой вывод, код завершён успешно)"
	}
}

// closeForm закрывает форму без сохранения.
func (m *model) closeForm() {
	m.state = formReturn[m.state]
	m.clearFormFeedback()
}

// commitForm применяет буфер формы к конфигурации, сохраняет и возвращает к
// списку. Значения берутся из стабильного буфера m.fb (см. formBuffers).
func (m *model) commitForm() {
	m.mutateIf(m.applyForm)
	m.closeForm()
	m.refreshList(m.state)
}

// applyForm переносит буфер в конфигурацию. Возвращает false, если применять
// некуда (редактируемый элемент или команда флагов исчезли).
func (m *model) applyForm() bool {
	editing := m.formMode == modeEdit
	switch m.state {
	case statePrefixForm:
		if !editing {
			m.cfg.Prefixes = append(m.cfg.Prefixes, models.Prefix{Name: m.fb.name, Description: m.fb.description})
			return true
		}
		if !inRange(m.editIdx, len(m.cfg.Prefixes)) {
			return false
		}
		p := &m.cfg.Prefixes[m.editIdx] // команды префикса сохраняем
		p.Name, p.Description = m.fb.name, m.fb.description

	case stateCommandForm:
		children := m.childrenPtr()
		c := m.fb.command()
		if !editing {
			*children = append(*children, c)
			return true
		}
		if !inRange(m.editIdx, len(*children)) {
			return false
		}
		old := (*children)[m.editIdx]
		// Вложенное и признак выключения формой не редактируются — сохраняем.
		c.Flags, c.Subcommands, c.Disabled = old.Flags, old.Subcommands, old.Disabled
		(*children)[m.editIdx] = c

	case stateFlagForm:
		cmd := m.commandAt(m.flagCmdPath)
		if cmd == nil {
			return false
		}
		if !editing {
			cmd.Flags = append(cmd.Flags, m.fb.flag())
			return true
		}
		if !inRange(m.editIdx, len(cmd.Flags)) {
			return false
		}
		f := m.fb.flag()
		f.Disabled = cmd.Flags[m.editIdx].Disabled
		cmd.Flags[m.editIdx] = f

	case stateVariableForm:
		if editing && m.editKey != "" && m.editKey != m.fb.name {
			delete(m.cfg.Variables, m.editKey) // переименование
		}
		m.cfg.Variables[m.fb.name] = m.fb.value
	}
	return true
}
