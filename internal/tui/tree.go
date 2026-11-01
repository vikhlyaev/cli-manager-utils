package tui

import (
	"strings"

	"cli-manager/internal/models"
)

// Навигация по дереву команд активного префикса. Путь — это индексы от команд
// префикса вниз по Subcommands: [0, 2] означает «третья подкоманда первой команды».

func (m *model) activePrefix() *models.Prefix {
	return &m.cfg.Prefixes[m.activePrefixIdx]
}

// resolve проходит путь от команд активного префикса и возвращает последнюю
// достигнутую команду (nil для пустого пути) и число валидных шагов.
func (m *model) resolve(path []int) (*models.Command, int) {
	cmds := &m.activePrefix().Commands
	var cmd *models.Command
	for n, idx := range path {
		if !inRange(idx, len(*cmds)) {
			return cmd, n
		}
		cmd = &(*cmds)[idx]
		cmds = &cmd.Subcommands
	}
	return cmd, len(path)
}

// commandAt возвращает команду по пути или nil, если путь пуст или невалиден.
func (m *model) commandAt(path []int) *models.Command {
	cmd, n := m.resolve(path)
	if n != len(path) {
		return nil
	}
	return cmd
}

// childrenPtr возвращает срез команд, показанный сейчас в списке команд: дети
// узла по cmdPath, а при пустом пути — команды префикса.
func (m *model) childrenPtr() *[]models.Command {
	if cmd, _ := m.resolve(m.cmdPath); cmd != nil {
		return &cmd.Subcommands
	}
	return &m.activePrefix().Commands
}

// validPathPrefix усекает путь до первого невалидного индекса (после изменения
// дерева, напр. при undo). Возвращает новый срез.
func (m *model) validPathPrefix(path []int) []int {
	_, n := m.resolve(path)
	return append([]int(nil), path[:n]...)
}

// breadcrumb строит человекочитаемый путь ("av db migrate") по пути индексов.
func (m *model) breadcrumb(path []int) string {
	p := m.activePrefix()
	parts := []string{p.Name}
	cmds := p.Commands
	for _, idx := range path {
		if !inRange(idx, len(cmds)) {
			break
		}
		parts = append(parts, cmds[idx].Name)
		cmds = cmds[idx].Subcommands
	}
	return strings.Join(parts, " ")
}

// openFlags открывает список флагов команды по индексу idx на текущем уровне.
func (m *model) openFlags(idx int) {
	m.flagCmdPath = append(append([]int(nil), m.cmdPath...), idx)
	m.refreshFlagList()
	m.flagsList.Select(0)
	m.state = stateFlagList
}

// enterSubcommands углубляется в подкоманды команды по индексу idx.
func (m *model) enterSubcommands(idx int) {
	m.cmdPath = append(m.cmdPath, idx)
	m.refreshCommandList()
	m.commandsList.Select(0)
}

// leaveSubcommands поднимается на уровень выше, выделяя команду, из которой
// вышли. На верхнем уровне возвращает false.
func (m *model) leaveSubcommands() bool {
	if len(m.cmdPath) == 0 {
		return false
	}
	child := m.cmdPath[len(m.cmdPath)-1]
	m.cmdPath = m.cmdPath[:len(m.cmdPath)-1]
	m.refreshCommandList()
	m.commandsList.Select(child)
	return true
}
