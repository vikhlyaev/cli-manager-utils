package tui

import (
	"os"
	"path/filepath"
	"strings"
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

// TestReorderCommands — `J` перемещает выбранную команду вниз, порядок
// сохраняется, выделение следует за элементом.
func TestReorderCommands(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	cfg := &models.Config{
		Variables: map[string]string{},
		Prefixes: []models.Prefix{{Name: "av", Commands: []models.Command{
			{Name: "a"}, {Name: "b"},
		}}},
	}
	m := NewModel(cfg)
	m = feed(m,
		tea.WindowSizeMsg{Width: 80, Height: 24},
		tea.KeyMsg{Type: tea.KeyEnter}, // -> Префиксы
		tea.KeyMsg{Type: tea.KeyEnter}, // -> Команды (av)
		runes("J"),                     // переместить первую команду вниз
	)
	cs := m.cfg.Prefixes[0].Commands
	if cs[0].Name != "b" || cs[1].Name != "a" {
		t.Fatalf("ожидался порядок b,a, получено %s,%s", cs[0].Name, cs[1].Name)
	}
	if m.commandsList.Index() != 1 {
		t.Errorf("выделение должно следовать за перемещённым элементом (idx 1), получено %d",
			m.commandsList.Index())
	}
}

// TestPreviewOpensAndReturns — `p` открывает предпросмотр zsh, Esc возвращает назад.
func TestPreviewOpensAndReturns(t *testing.T) {
	cfg := &models.Config{
		Variables: map[string]string{},
		Prefixes: []models.Prefix{{Name: "av", Commands: []models.Command{
			{Name: "note", Run: "echo hi"},
		}}},
	}
	m := NewModel(cfg)
	m = feed(m,
		tea.WindowSizeMsg{Width: 80, Height: 24},
		tea.KeyMsg{Type: tea.KeyEnter}, // -> Префиксы
		runes("p"),                     // предпросмотр
	)
	if m.state != statePreview {
		t.Fatalf("ожидалось состояние предпросмотра, получено %v", m.state)
	}
	m = feed(m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.state != statePrefixList {
		t.Fatalf("после Esc ожидался список префиксов, получено %v", m.state)
	}
}

// TestToggleShell — пункт «Оболочка» в главном меню переключает zsh↔bash и
// записывает выбор в конфиг.
func TestToggleShell(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := NewModel(&models.Config{Variables: map[string]string{}})
	if m.shellName() != "zsh" {
		t.Fatalf("по умолчанию ожидался zsh, получено %s", m.shellName())
	}
	m = feed(m,
		tea.WindowSizeMsg{Width: 80, Height: 24},
		runes("j"), runes("j"), // главное меню: индекс 0 -> 2 (Оболочка)
		tea.KeyMsg{Type: tea.KeyEnter}, // переключить -> bash
	)
	if m.shellName() != "bash" || m.cfg.Shell != "bash" {
		t.Fatalf("после переключения ожидался bash, получено %q", m.cfg.Shell)
	}
	m = feed(m, tea.KeyMsg{Type: tea.KeyEnter}) // обратно -> zsh
	if m.shellName() != "zsh" {
		t.Fatalf("повторное переключение должно вернуть zsh, получено %s", m.shellName())
	}
}

// TestImportAliases — `i` в списке префиксов импортирует alias из ~/.zshrc в
// префикс-контейнер «imported», пропуская невалидные имена.
func TestImportAliases(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	rc := "alias gs='git status'\nalias ll=\"ls -la\"\nalias g+='bad name'\n"
	if err := os.WriteFile(filepath.Join(home, ".zshrc"), []byte(rc), 0o644); err != nil {
		t.Fatal(err)
	}

	m := NewModel(&models.Config{Variables: map[string]string{}})
	m = feed(m,
		tea.WindowSizeMsg{Width: 80, Height: 24},
		tea.KeyMsg{Type: tea.KeyEnter}, // -> Префиксы
		runes("i"),                     // импорт
	)

	var imp *models.Prefix
	for i := range m.cfg.Prefixes {
		if m.cfg.Prefixes[i].Name == importedPrefix {
			imp = &m.cfg.Prefixes[i]
		}
	}
	if imp == nil {
		t.Fatalf("ожидался префикс %q, получено %+v", importedPrefix, m.cfg.Prefixes)
	}
	if len(imp.Commands) != 2 { // gs, ll — валидны; g+ пропущен
		t.Fatalf("ожидалось 2 импортированных команды, получено %d: %+v", len(imp.Commands), imp.Commands)
	}
	if imp.Commands[0].Name != "gs" || imp.Commands[0].Run != "git status" {
		t.Errorf("первая команда: ожидалось gs/'git status', получено %q/%q",
			imp.Commands[0].Name, imp.Commands[0].Run)
	}

	// Повторный импорт не должен дублировать уже добавленные команды.
	m = feed(m, runes("i"))
	if len(m.cfg.Prefixes[len(m.cfg.Prefixes)-1].Commands) != 2 {
		t.Errorf("повторный импорт не должен дублировать команды")
	}
}

// TestEmptyListScreen — пустой список показывает русский заголовок-путь и
// инструктивную подсказку, без английского «No items».
func TestEmptyListScreen(t *testing.T) {
	cfg := &models.Config{
		Variables: map[string]string{},
		Prefixes:  []models.Prefix{{Name: "av", Commands: []models.Command{{Name: "deploy"}}}},
	}
	m := NewModel(cfg)
	m = feed(m,
		tea.WindowSizeMsg{Width: 80, Height: 24},
		tea.KeyMsg{Type: tea.KeyEnter}, // Префиксы
		tea.KeyMsg{Type: tea.KeyEnter}, // Команды (deploy)
		tea.KeyMsg{Type: tea.KeyEnter}, // deploy — лист, Enter -> флаги (пусто)
	)
	view := m.View()
	if !strings.Contains(view, "av deploy") {
		t.Errorf("ожидался заголовок-путь 'av deploy':\n%s", view)
	}
	if !strings.Contains(view, "Флагов нет") {
		t.Errorf("ожидался русский текст пустого состояния:\n%s", view)
	}
	if strings.Contains(view, "No items") {
		t.Errorf("английское 'No items' не должно отображаться:\n%s", view)
	}
}

// TestFormHeaderContext — заголовок формы отражает режим, тип и путь в дереве.
func TestFormHeaderContext(t *testing.T) {
	cfg := &models.Config{
		Variables: map[string]string{},
		Prefixes: []models.Prefix{{Name: "av", Commands: []models.Command{
			{Name: "db", Subcommands: []models.Command{
				{Name: "up", Flags: []models.Flag{{Name: "--force"}}},
			}},
		}}},
	}
	m := NewModel(cfg)

	m.state = stateCommandForm
	m.formMode = modeCreate
	m.cmdPath = []int{0} // внутри db
	if h := m.formHeader(); !strings.Contains(h, "Новая команда") || !strings.Contains(h, "av db") {
		t.Errorf("заголовок команды должен показывать режим и путь, получено %q", h)
	}

	m.state = stateFlagForm
	m.formMode = modeEdit
	m.flagCmdPath = []int{0, 0} // av db up
	if h := m.formHeader(); !strings.Contains(h, "Изменение флага") || !strings.Contains(h, "av db up") {
		t.Errorf("заголовок флага должен показывать путь команды, получено %q", h)
	}
}

// TestEnterOpensFlagsForLeaf — на команде-листе Enter открывает её флаги
// (регресс: после ввода вложенности Enter на листе должен вести к флагам,
// а не к пустому списку подкоманд).
func TestEnterOpensFlagsForLeaf(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	cfg := &models.Config{
		Variables: map[string]string{},
		Prefixes: []models.Prefix{{Name: "av", Commands: []models.Command{
			{Name: "deploy", Run: "echo deploy", Flags: []models.Flag{{Name: "--force"}}},
		}}},
	}
	m := NewModel(cfg)
	m = feed(m,
		tea.WindowSizeMsg{Width: 80, Height: 24},
		tea.KeyMsg{Type: tea.KeyEnter}, // Префиксы
		tea.KeyMsg{Type: tea.KeyEnter}, // Команды (deploy)
		tea.KeyMsg{Type: tea.KeyEnter}, // deploy — лист, Enter -> флаги
	)
	if m.state != stateFlagList {
		t.Fatalf("Enter на листе должен открывать флаги, состояние %v", m.state)
	}
	if c := m.commandAt(m.flagCmdPath); c == nil || len(c.Flags) != 1 {
		t.Fatalf("ожидался 1 флаг у deploy, получено %+v", c)
	}
}

// TestNestedCommandNavigation — Enter углубляется в подкоманды, Esc возвращает
// на уровень выше.
func TestNestedCommandNavigation(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	cfg := &models.Config{
		Variables: map[string]string{},
		Prefixes: []models.Prefix{{Name: "av", Commands: []models.Command{
			{Name: "db", Subcommands: []models.Command{{Name: "migrate"}}},
		}}},
	}
	m := NewModel(cfg)
	m = feed(m,
		tea.WindowSizeMsg{Width: 80, Height: 24},
		tea.KeyMsg{Type: tea.KeyEnter}, // -> Префиксы
		tea.KeyMsg{Type: tea.KeyEnter}, // -> Команды (av): [db]
		tea.KeyMsg{Type: tea.KeyEnter}, // вглубь db: [migrate]
	)
	if len(m.cmdPath) != 1 || m.cmdPath[0] != 0 {
		t.Fatalf("ожидался cmdPath [0] (db), получено %v", m.cmdPath)
	}
	if got := len(*m.childrenPtr()); got != 1 || (*m.childrenPtr())[0].Name != "migrate" {
		t.Fatalf("на уровне db ожидалась подкоманда migrate, получено %+v", *m.childrenPtr())
	}

	m = feed(m, tea.KeyMsg{Type: tea.KeyEsc}) // назад на верхний уровень
	if len(m.cmdPath) != 0 || m.state != stateCommandList {
		t.Fatalf("после Esc ожидался верхний уровень команд, cmdPath=%v state=%v", m.cmdPath, m.state)
	}
}

// TestAddNestedSubcommand — добавление команды на вложенном уровне попадает в
// Subcommands родителя, не трогая верхний уровень.
func TestAddNestedSubcommand(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	cfg := &models.Config{
		Variables: map[string]string{},
		Prefixes: []models.Prefix{{Name: "av", Commands: []models.Command{
			{Name: "db", Subcommands: []models.Command{{Name: "migrate"}}},
		}}},
	}
	m := NewModel(cfg)
	m = feed(m,
		tea.WindowSizeMsg{Width: 80, Height: 24},
		tea.KeyMsg{Type: tea.KeyEnter}, // Префиксы
		tea.KeyMsg{Type: tea.KeyEnter}, // Команды (db)
		tea.KeyMsg{Type: tea.KeyEnter}, // вглубь db
		runes("n"),                     // новая подкоманда
		runes("seed"),                  // имя
	)
	m.commitForm()

	subs := m.cfg.Prefixes[0].Commands[0].Subcommands
	if len(subs) != 2 || subs[1].Name != "seed" {
		t.Fatalf("ожидались migrate,seed под db; получено %+v", subs)
	}
	if len(m.cfg.Prefixes[0].Commands) != 1 {
		t.Errorf("верхний уровень не должен меняться, получено %d команд", len(m.cfg.Prefixes[0].Commands))
	}
}

// TestFlagsForNestedLeaf — на листе Enter открывает флаги; путь флагов
// учитывает вложенность.
func TestFlagsForNestedLeaf(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	cfg := &models.Config{
		Variables: map[string]string{},
		Prefixes: []models.Prefix{{Name: "av", Commands: []models.Command{
			{Name: "db", Subcommands: []models.Command{
				{Name: "up", Flags: []models.Flag{{Name: "--force"}}},
			}},
		}}},
	}
	m := NewModel(cfg)
	m = feed(m,
		tea.WindowSizeMsg{Width: 80, Height: 24},
		tea.KeyMsg{Type: tea.KeyEnter}, // Префиксы
		tea.KeyMsg{Type: tea.KeyEnter}, // Команды (db) — ветвь
		tea.KeyMsg{Type: tea.KeyEnter}, // вглубь db: [up]
		tea.KeyMsg{Type: tea.KeyEnter}, // up — лист, Enter -> флаги
	)
	if m.state != stateFlagList {
		t.Fatalf("ожидался список флагов, получено %v", m.state)
	}
	if len(m.flagCmdPath) != 2 || m.flagCmdPath[0] != 0 || m.flagCmdPath[1] != 0 {
		t.Fatalf("ожидался flagCmdPath [0 0] (db up), получено %v", m.flagCmdPath)
	}
	if c := m.commandAt(m.flagCmdPath); c == nil || len(c.Flags) != 1 || c.Flags[0].Name != "--force" {
		t.Fatalf("флаги команды up не найдены по пути: %+v", c)
	}
}

// TestVariablesPanel — панель подсказки перечисляет определённые переменные
// в форме ${NAME} и пуста, когда переменных нет.
func TestVariablesPanel(t *testing.T) {
	m := NewModel(&models.Config{Variables: map[string]string{"B": "2", "A": "1"}})
	p := m.variablesPanel()
	if !strings.Contains(p, "${A}") || !strings.Contains(p, "${B}") {
		t.Fatalf("панель должна содержать ${A} и ${B}, получено: %q", p)
	}
	m2 := NewModel(&models.Config{Variables: map[string]string{}})
	if m2.variablesPanel() != "" {
		t.Errorf("без переменных панель должна быть пустой, получено: %q", m2.variablesPanel())
	}
}

// TestUndoRestoresAfterDelete — `u` откатывает последнюю мутацию (удаление).
func TestUndoRestoresAfterDelete(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	cfg := &models.Config{
		Variables: map[string]string{},
		Prefixes:  []models.Prefix{{Name: "av"}, {Name: "gx"}},
	}
	m := NewModel(cfg)
	m = feed(m,
		tea.WindowSizeMsg{Width: 80, Height: 24},
		tea.KeyMsg{Type: tea.KeyEnter}, // -> Префиксы
		runes("d"),                     // запрос удаления
		runes("y"),                     // подтвердить -> удалён 'av'
	)
	if len(m.cfg.Prefixes) != 1 || m.cfg.Prefixes[0].Name != "gx" {
		t.Fatalf("после удаления ожидался один 'gx', получено %+v", m.cfg.Prefixes)
	}

	m = feed(m, runes("u")) // отмена
	if len(m.cfg.Prefixes) != 2 || m.cfg.Prefixes[0].Name != "av" || m.cfg.Prefixes[1].Name != "gx" {
		t.Fatalf("после отмены ожидались av,gx; получено %+v", m.cfg.Prefixes)
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
