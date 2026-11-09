package models

// Flag описывает именованный флаг команды со своим bash-действием.
type Flag struct {
	Name        string `yaml:"name" json:"name"`
	Description string `yaml:"description,omitempty" json:"description,omitempty"`
	Run         string `yaml:"run" json:"run"`
	// AfterCommand — перед действием флага выполнить действие самой команды
	// (напр. команда делает cd, а флаг запускает в этой папке инструмент).
	// Если действие команды завершилось ошибкой, флаг не выполняется.
	AfterCommand bool `yaml:"after_command,omitempty" json:"after_command,omitempty"`
	// Disabled — флаг выключен: остаётся в конфиге, но не попадает в скрипт.
	Disabled bool `yaml:"disabled,omitempty" json:"disabled,omitempty"`
}

// Command описывает команду внутри префикса.
//
// Команда может быть листом (есть Run, опционально Flags) либо ветвью-диспетчером
// (есть Subcommands произвольной вложенности). Если заданы Subcommands, при
// генерации они имеют приоритет над Flags, а Run используется как действие по
// умолчанию, когда подкоманда не распознана.
type Command struct {
	Name        string `yaml:"name" json:"name"`
	Description string `yaml:"description,omitempty" json:"description,omitempty"`
	Run         string `yaml:"run,omitempty" json:"run,omitempty"`
	// Usage — описание аргументов для справки, напр. "<файл> [строки]".
	Usage       string    `yaml:"usage,omitempty" json:"usage,omitempty"`
	AcceptsArgs bool      `yaml:"accepts_args,omitempty" json:"accepts_args,omitempty"`
	Condition   string    `yaml:"condition,omitempty" json:"condition,omitempty"`
	Flags       []Flag    `yaml:"flags,omitempty" json:"flags,omitempty"`
	Subcommands []Command `yaml:"subcommands,omitempty" json:"subcommands,omitempty"`
	// AfterCommand — перед действием подкоманды выполнить действие
	// родительской команды (напр. родитель делает cd, а подкоманда запускает
	// в этой папке инструмент). Для команд верхнего уровня не действует.
	AfterCommand bool `yaml:"after_command,omitempty" json:"after_command,omitempty"`
	// Disabled — команда (со всеми вложенными) выключена и не генерируется.
	Disabled bool `yaml:"disabled,omitempty" json:"disabled,omitempty"`
}

// IsBranch сообщает, является ли команда ветвью-диспетчером (имеет подкоманды).
func (c Command) IsBranch() bool { return len(c.Subcommands) > 0 }

// Prefix описывает корневой префикс (составная команда верхнего уровня).
type Prefix struct {
	Name        string    `yaml:"name" json:"name"`
	Description string    `yaml:"description,omitempty" json:"description,omitempty"`
	Commands    []Command `yaml:"commands,omitempty" json:"commands,omitempty"`
	// Disabled — префикс выключен: функция не генерируется.
	Disabled bool `yaml:"disabled,omitempty" json:"disabled,omitempty"`
}

// Config — корневая структура конфигурации cli-manager.
type Config struct {
	// Shell — целевая оболочка для генерации скрипта ("zsh" или "bash").
	// Пустое значение трактуется как "zsh" (поведение по умолчанию).
	Shell     string            `yaml:"shell,omitempty" json:"shell,omitempty"`
	Variables map[string]string `yaml:"variables,omitempty" json:"variables,omitempty"`
	Prefixes  []Prefix          `yaml:"prefixes" json:"prefixes"`
}
