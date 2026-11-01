package tui

import "github.com/charmbracelet/lipgloss"

var (
	docStyle    = lipgloss.NewStyle().Margin(1, 2)
	hintStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	warnStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("214")).Bold(true)
	errStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	okStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("78"))
	headerStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("205")).Bold(true)
	cursorStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("205")).Bold(true)
	boxStyle    = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("240")).Padding(0, 1).MarginTop(1)
)

// chromeLines — строки, которые экраны резервируют под свою шапку (заголовок,
// подзаголовок, отступы) и строку подсказок снизу.
const chromeLines = 5
