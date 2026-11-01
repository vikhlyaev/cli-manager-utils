package tui

import (
	"fmt"

	"cli-manager/internal/models"
)

// prefixItem — элемент списка префиксов.
type prefixItem struct{ prefix models.Prefix }

// disabledMark — пометка выключенного элемента в начале заголовка.
const disabledMark = "⏸ "

func mark(disabled bool, title string) string {
	if disabled {
		return disabledMark + title + " · выключен"
	}
	return title
}

func (i prefixItem) Title() string {
	t := i.prefix.Name
	if i.prefix.Description != "" {
		t = fmt.Sprintf("%s — %s", i.prefix.Name, i.prefix.Description)
	}
	return mark(i.prefix.Disabled, t)
}
func (i prefixItem) Description() string {
	return fmt.Sprintf("команд: %d", len(i.prefix.Commands))
}
func (i prefixItem) FilterValue() string { return i.prefix.Name }

// commandItem — элемент списка команд.
type commandItem struct{ cmd models.Command }

func (i commandItem) Title() string {
	t := i.cmd.Name
	if i.cmd.IsBranch() {
		t += " ▸" // маркер ветви с подкомандами
	}
	return mark(i.cmd.Disabled, t)
}
func (i commandItem) Description() string {
	desc := i.cmd.Description
	if desc == "" {
		desc = i.cmd.Run
	}
	var meta string
	if i.cmd.IsBranch() {
		meta = fmt.Sprintf("подкоманд: %d", len(i.cmd.Subcommands))
	} else {
		meta = fmt.Sprintf("флагов: %d", len(i.cmd.Flags))
	}
	if i.cmd.Usage != "" {
		meta += " · " + i.cmd.Usage
	} else if i.cmd.AcceptsArgs {
		meta += " · принимает аргументы"
	}
	if i.cmd.Condition != "" {
		meta += " · условие"
	}
	if desc != "" {
		return fmt.Sprintf("%s · %s", desc, meta)
	}
	return meta
}
func (i commandItem) FilterValue() string { return i.cmd.Name }

// flagItem — элемент списка флагов.
type flagItem struct{ flag models.Flag }

func (i flagItem) Title() string { return mark(i.flag.Disabled, i.flag.Name) }
func (i flagItem) Description() string {
	if i.flag.Description != "" {
		return i.flag.Description
	}
	return i.flag.Run
}
func (i flagItem) FilterValue() string { return i.flag.Name }

// variableItem — элемент списка глобальных переменных.
type variableItem struct {
	key   string
	value string
}

func (i variableItem) Title() string       { return i.key }
func (i variableItem) Description() string { return i.value }
func (i variableItem) FilterValue() string { return i.key }
