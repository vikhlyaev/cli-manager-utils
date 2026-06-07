package models

// Flag описывает именованный флаг команды со своим bash-действием.
type Flag struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description,omitempty"`
	Run         string `yaml:"run"`
}

// Command описывает команду внутри префикса.
type Command struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description,omitempty"`
	Run         string `yaml:"run"`
	AcceptsArgs bool   `yaml:"accepts_args,omitempty"`
	Condition   string `yaml:"condition,omitempty"`
	Flags       []Flag `yaml:"flags,omitempty"`
}

// Prefix описывает корневой префикс (составная команда верхнего уровня).
type Prefix struct {
	Name        string    `yaml:"name"`
	Description string    `yaml:"description,omitempty"`
	Commands    []Command `yaml:"commands,omitempty"`
}

// Config — корневая структура конфигурации cli-manager.
type Config struct {
	Variables map[string]string `yaml:"variables,omitempty"`
	Prefixes  []Prefix          `yaml:"prefixes"`
}
