package models

// Flag описывает именованный флаг команды со своим bash-действием.
type Flag struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description,omitempty"`
	Run         string `yaml:"run"`
	// Disabled — флаг выключен: остаётся в конфиге, но не попадает в скрипт.
	Disabled bool `yaml:"disabled,omitempty"`
}

// Command описывает команду внутри префикса.
//
// Команда может быть листом (есть Run, опционально Flags) либо ветвью-диспетчером
// (есть Subcommands произвольной вложенности). Если заданы Subcommands, при
// генерации они имеют приоритет над Flags, а Run используется как действие по
// умолчанию, когда подкоманда не распознана.
type Command struct {
	Name        string    `yaml:"name"`
	Description string    `yaml:"description,omitempty"`
	Run         string    `yaml:"run,omitempty"`
	AcceptsArgs bool      `yaml:"accepts_args,omitempty"`
	Condition   string    `yaml:"condition,omitempty"`
	Flags       []Flag    `yaml:"flags,omitempty"`
	Subcommands []Command `yaml:"subcommands,omitempty"`
	// Disabled — команда (со всеми вложенными) выключена и не генерируется.
	Disabled bool `yaml:"disabled,omitempty"`
}

// IsBranch сообщает, является ли команда ветвью-диспетчером (имеет подкоманды).
func (c Command) IsBranch() bool { return len(c.Subcommands) > 0 }

// Prefix описывает корневой префикс (составная команда верхнего уровня).
type Prefix struct {
	Name        string    `yaml:"name"`
	Description string    `yaml:"description,omitempty"`
	Commands    []Command `yaml:"commands,omitempty"`
	// Disabled — префикс выключен: функция не генерируется.
	Disabled bool `yaml:"disabled,omitempty"`
}

// Config — корневая структура конфигурации cli-manager.
type Config struct {
	// Shell — целевая оболочка для генерации скрипта ("zsh" или "bash").
	// Пустое значение трактуется как "zsh" (поведение по умолчанию).
	Shell     string            `yaml:"shell,omitempty"`
	Variables map[string]string `yaml:"variables,omitempty"`
	Prefixes  []Prefix          `yaml:"prefixes"`
}
