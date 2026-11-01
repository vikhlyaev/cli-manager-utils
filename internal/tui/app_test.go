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

// TestImportAliases — `i` в списке префиксов импортирует alias из rc-файла в
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

// TestValidateCommandNameChecksCurrentLevel — уникальность имени команды
// проверяется на текущем уровне дерева, а не среди команд верхнего уровня.
func TestValidateCommandNameChecksCurrentLevel(t *testing.T) {
	cfg := &models.Config{
		Variables: map[string]string{},
		Prefixes: []models.Prefix{{Name: "av", Commands: []models.Command{
			{Name: "db", Subcommands: []models.Command{{Name: "migrate"}}},
			{Name: "deploy"},
		}}},
	}
	m := NewModel(cfg)
	m.cmdPath = []int{0} // внутри db
	m.formMode = modeCreate

	if err := m.validateCommandName("migrate"); err == nil {
		t.Error("дубль 'migrate' на уровне db должен отклоняться")
	}
	if err := m.validateCommandName("deploy"); err != nil {
		t.Errorf("'deploy' есть только на верхнем уровне, внутри db имя свободно: %v", err)
	}

	m.formMode, m.editIdx = modeEdit, 0 // правим сам migrate
	if err := m.validateCommandName("migrate"); err != nil {
		t.Errorf("редактируемая команда не должна конфликтовать сама с собой: %v", err)
	}
}

// TestValidateFlagNameUsesOpenCommand — уникальность флага проверяется среди
// флагов открытой команды; раньше брались флаги не той команды, а индекс
// с вложенного уровня мог выйти за границы и уронить приложение.
func TestValidateFlagNameUsesOpenCommand(t *testing.T) {
	cfg := &models.Config{
		Variables: map[string]string{},
		Prefixes: []models.Prefix{{Name: "av", Commands: []models.Command{
			{Name: "db", Subcommands: []models.Command{
				{Name: "a"}, {Name: "b"}, {Name: "up", Flags: []models.Flag{{Name: "--force"}}},
			}},
		}}},
	}
	m := NewModel(cfg)
	m.flagCmdPath = []int{0, 2} // av db up
	m.formMode = modeCreate

	if err := m.validateFlagName("--force"); err == nil {
		t.Error("дубль '--force' у команды up должен отклоняться")
	}
	if err := m.validateFlagName("--dry-run"); err != nil {
		t.Errorf("новый флаг должен проходить: %v", err)
	}
}

// TestEditFlagReplacesInPlace — правка флага заменяет его на месте, не добавляя новый.
func TestEditFlagReplacesInPlace(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	cfg := &models.Config{
		Variables: map[string]string{},
		Prefixes: []models.Prefix{{Name: "av", Commands: []models.Command{
			{Name: "deploy", Flags: []models.Flag{{Name: "--a"}, {Name: "--b"}}},
		}}},
	}
	m := NewModel(cfg)
	m = feed(m,
		tea.WindowSizeMsg{Width: 80, Height: 24},
		tea.KeyMsg{Type: tea.KeyEnter}, // Префиксы
		tea.KeyMsg{Type: tea.KeyEnter}, // Команды
		tea.KeyMsg{Type: tea.KeyEnter}, // deploy — лист -> флаги
		runes("j"),                     // выбрать --b
		runes("e"),                     // редактировать
	)
	m.fb.name = "--c"
	m.commitForm()

	fs := m.cfg.Prefixes[0].Commands[0].Flags
	if len(fs) != 2 || fs[0].Name != "--a" || fs[1].Name != "--c" {
		t.Fatalf("ожидались --a,--c, получено %+v", fs)
	}
	if m.state != stateFlagList {
		t.Errorf("после сохранения ожидался список флагов, получено %v", m.state)
	}
}

// TestUndoCreatedPrefixLeavesCommandScreen — отмена создания префикса, внутри
// которого стоит пользователь, возвращает к списку префиксов (раньше View падал
// с index out of range).
func TestUndoCreatedPrefixLeavesCommandScreen(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := NewModel(&models.Config{Variables: map[string]string{}})
	m = feed(m,
		tea.WindowSizeMsg{Width: 80, Height: 24},
		tea.KeyMsg{Type: tea.KeyEnter}, // -> Префиксы
		runes("n"), runes("av"),
	)
	m.commitForm()
	m = feed(m, tea.KeyMsg{Type: tea.KeyEnter}) // внутрь av
	if m.state != stateCommandList {
		t.Fatalf("ожидался список команд, получено %v", m.state)
	}

	m = feed(m, runes("u"))
	if len(m.cfg.Prefixes) != 0 || m.state != statePrefixList {
		t.Fatalf("после отмены ожидался пустой список префиксов, state=%v prefixes=%+v", m.state, m.cfg.Prefixes)
	}
	_ = m.View() // не должно паниковать
}

// TestUndoRemovedFlagCommandLeavesFlagScreen — если undo убрал команду, чьи
// флаги открыты, экран поднимается к списку команд, а не остаётся на устаревших флагах.
func TestUndoRemovedFlagCommandLeavesFlagScreen(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	cfg := &models.Config{Variables: map[string]string{}, Prefixes: []models.Prefix{{Name: "av"}}}
	m := NewModel(cfg)
	m = feed(m,
		tea.WindowSizeMsg{Width: 80, Height: 24},
		tea.KeyMsg{Type: tea.KeyEnter}, // Префиксы
		tea.KeyMsg{Type: tea.KeyEnter}, // Команды av
		runes("n"), runes("x"),
	)
	m.commitForm()
	m = feed(m, tea.KeyMsg{Type: tea.KeyEnter}) // x — лист -> флаги
	if m.state != stateFlagList {
		t.Fatalf("ожидался список флагов, получено %v", m.state)
	}

	m = feed(m, runes("u"))
	if m.state != stateCommandList || m.flagCmdPath != nil {
		t.Fatalf("ожидался возврат к командам, state=%v flagCmdPath=%v", m.state, m.flagCmdPath)
	}
}

// TestPrefixCollisionWarning — имя префикса проверяется на совпадение с
// системной командой и с alias из ~/.zshrc; имена команд внутри префикса — нет.
func TestPrefixCollisionWarning(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if err := os.WriteFile(filepath.Join(home, ".zshrc"), []byte("alias gs='git status'\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	open := func(name string) model {
		m := NewModel(&models.Config{Variables: map[string]string{}})
		return feed(m,
			tea.WindowSizeMsg{Width: 80, Height: 24},
			tea.KeyMsg{Type: tea.KeyEnter}, // -> Префиксы
			runes("n"), runes(name),
		)
	}

	if m := open("ls"); !strings.Contains(m.collision, "уже есть в системе") {
		t.Errorf("префикс 'ls' должен предупреждать о системной команде, получено %q", m.collision)
	}
	if m := open("gs"); !strings.Contains(m.collision, "alias") {
		t.Errorf("префикс 'gs' должен предупреждать об alias, получено %q", m.collision)
	}
	if m := open("zzq_free"); m.collision != "" {
		t.Errorf("свободное имя не должно давать предупреждения, получено %q", m.collision)
	}

	// Команда 'ls' внутри префикса ничего не перекрывает.
	m := NewModel(&models.Config{Variables: map[string]string{}, Prefixes: []models.Prefix{{Name: "av"}}})
	m = feed(m,
		tea.WindowSizeMsg{Width: 80, Height: 24},
		tea.KeyMsg{Type: tea.KeyEnter}, // Префиксы
		tea.KeyMsg{Type: tea.KeyEnter}, // Команды av
		runes("n"), runes("ls"),
	)
	if m.collision != "" || strings.Contains(m.View(), "⚠️") {
		t.Errorf("имя команды не должно проверяться на коллизию, получено %q", m.collision)
	}
}

// TestUndoKeepsOpenPrefixByName — undo, вернувший префикс перед открытым,
// оставляет пользователя в том же префиксе, а не в соседнем по индексу.
func TestUndoKeepsOpenPrefixByName(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	cfg := &models.Config{
		Variables: map[string]string{},
		Prefixes: []models.Prefix{
			{Name: "av"},
			{Name: "gx", Commands: []models.Command{{Name: "status"}}},
		},
	}
	m := NewModel(cfg)
	m = feed(m,
		tea.WindowSizeMsg{Width: 80, Height: 24},
		tea.KeyMsg{Type: tea.KeyEnter}, // Префиксы
		runes("d"), runes("y"),         // удалить av -> остался gx
		tea.KeyMsg{Type: tea.KeyEnter}, // войти в gx
		runes("u"),                     // вернуть av (он встанет перед gx)
	)
	if m.state != stateCommandList {
		t.Fatalf("ожидался список команд, получено %v", m.state)
	}
	if got := m.activePrefix().Name; got != "gx" {
		t.Fatalf("после undo должен остаться открытым gx, открыт %q", got)
	}
	if !strings.Contains(m.View(), "gx · команды") {
		t.Errorf("заголовок должен показывать gx:\n%s", m.View())
	}
}

// TestUndoRenameKeepsOpenPrefix — undo переименования открытого префикса
// оставляет пользователя в нём же (поиск по прежнему индексу).
func TestUndoRenameKeepsOpenPrefix(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	cfg := &models.Config{
		Variables: map[string]string{},
		Prefixes:  []models.Prefix{{Name: "av"}, {Name: "gx", Commands: []models.Command{{Name: "st"}}}},
	}
	m := NewModel(cfg)
	m = feed(m,
		tea.WindowSizeMsg{Width: 80, Height: 24},
		tea.KeyMsg{Type: tea.KeyEnter}, // Префиксы
		runes("j"), runes("e"),         // редактировать gx
	)
	m.fb.name = "gy"
	m.commitForm()
	m = feed(m, tea.KeyMsg{Type: tea.KeyEnter}, runes("u")) // войти в gy, отменить переименование

	if m.state != stateCommandList || m.activePrefix().Name != "gx" {
		t.Fatalf("ожидался открытый gx, state=%v prefix=%q", m.state, m.activePrefix().Name)
	}
}

// TestHelpScreen — `?` открывает справку из меню и из списков, esc возвращает назад.
func TestHelpScreen(t *testing.T) {
	m := NewModel(&models.Config{Variables: map[string]string{}})
	m = feed(m, tea.WindowSizeMsg{Width: 80, Height: 30}, runes("?"))
	if m.state != statePreview || m.previewTitle != helpTitle {
		t.Fatalf("из меню: state=%v title=%q", m.state, m.previewTitle)
	}
	m = feed(m, tea.KeyMsg{Type: tea.KeyEsc}, tea.KeyMsg{Type: tea.KeyEnter}, runes("?"))
	if m.state != statePreview || !strings.Contains(m.View(), "все клавиши") {
		t.Fatalf("из списка префиксов справка не открылась: state=%v", m.state)
	}
	m = feed(m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.state != statePrefixList {
		t.Fatalf("esc должен вернуть к списку, state=%v", m.state)
	}
}

// TestStatusClearedOnNextKey — статус последнего действия исчезает при
// следующем нажатии, а не висит до конца сессии.
func TestStatusClearedOnNextKey(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := NewModel(&models.Config{Variables: map[string]string{}, Prefixes: []models.Prefix{{Name: "av"}}})
	m = feed(m, tea.WindowSizeMsg{Width: 80, Height: 24}, tea.KeyMsg{Type: tea.KeyEnter}, runes("u"))
	if m.status != "Нечего отменять" {
		t.Fatalf("status=%q", m.status)
	}
	m = feed(m, runes("j"))
	if m.status != "" {
		t.Fatalf("статус должен сброситься, получено %q", m.status)
	}
}

// TestMenuFullPreview — пункт меню показывает весь скрипт со всеми префиксами.
func TestMenuFullPreview(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := NewModel(&models.Config{Variables: map[string]string{}, Prefixes: []models.Prefix{{Name: "av"}, {Name: "gx"}}})
	m = feed(m, tea.WindowSizeMsg{Width: 80, Height: 200})
	for range menuPreview {
		m = feed(m, runes("j"))
	}
	m = feed(m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.state != statePreview {
		t.Fatalf("state=%v", m.state)
	}
	content := m.previewVP.View()
	if !strings.Contains(content, "function av {") || !strings.Contains(content, "function gx {") {
		t.Errorf("в предпросмотре должны быть оба префикса:\n%s", content)
	}
}

// TestFilterTypingDoesNotTriggerActions — пока набирается запрос, буквы
// уходят в строку поиска, а не срабатывают как n/e/d.
func TestFilterTypingDoesNotTriggerActions(t *testing.T) {
	m := NewModel(&models.Config{Variables: map[string]string{}, Prefixes: []models.Prefix{{Name: "av"}}})
	m = feed(m, tea.WindowSizeMsg{Width: 80, Height: 24}, tea.KeyMsg{Type: tea.KeyEnter},
		runes("/"), runes("n"), runes("d"))
	if m.state != statePrefixList || m.confirmDelete {
		t.Fatalf("набор фильтра не должен открывать форму/удаление: state=%v confirm=%v", m.state, m.confirmDelete)
	}
	if got := m.prefixesList.FilterInput.Value(); got != "nd" {
		t.Fatalf("текст фильтра = %q, ожидалось nd", got)
	}
}

// TestFilteredSelectionUsesRealIndex — при применённом фильтре действия
// работают с выбранным элементом, а не с элементом на той же позиции в
// полном списке; esc сначала снимает фильтр.
func TestFilteredSelectionUsesRealIndex(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	// Каждой ветке — своя модель: копии model делят конфиг по указателю.
	filtered := func() model {
		m := NewModel(&models.Config{Variables: map[string]string{}, Prefixes: []models.Prefix{
			{Name: "av"}, {Name: "db"}, {Name: "gx"},
		}})
		m = feed(m, tea.WindowSizeMsg{Width: 80, Height: 24}, tea.KeyMsg{Type: tea.KeyEnter})
		m.prefixesList.SetFilterText("gx")
		return m
	}

	if v := filtered().View(); !strings.Contains(v, "фильтр: gx") {
		t.Errorf("применённый фильтр должен быть виден:\n%s", v)
	}

	md := feed(filtered(), runes("d"), runes("y"))
	if len(md.cfg.Prefixes) != 2 || md.cfg.Prefixes[0].Name != "av" || md.cfg.Prefixes[1].Name != "db" {
		t.Fatalf("должен удалиться gx, осталось %+v", md.cfg.Prefixes)
	}

	me := feed(filtered(), tea.KeyMsg{Type: tea.KeyEnter})
	if me.state != stateCommandList || me.activePrefixIdx != 2 {
		t.Fatalf("enter должен открыть gx: state=%v idx=%d", me.state, me.activePrefixIdx)
	}

	mesc := feed(filtered(), tea.KeyMsg{Type: tea.KeyEsc})
	if mesc.state != statePrefixList || mesc.prefixesList.IsFiltered() {
		t.Fatalf("esc должен снять фильтр и остаться в списке: state=%v", mesc.state)
	}
}

func TestEditorCommand(t *testing.T) {
	t.Setenv("VISUAL", "")
	t.Setenv("EDITOR", "")
	if got := editorCommand(); len(got) != 1 || got[0] != "nano" {
		t.Errorf("по умолчанию nano, получено %v", got)
	}
	t.Setenv("EDITOR", "code -w")
	if got := editorCommand(); len(got) != 2 || got[0] != "code" || got[1] != "-w" {
		t.Errorf("EDITOR с аргументами: %v", got)
	}
	t.Setenv("VISUAL", "nvim")
	if got := editorCommand(); got[0] != "nvim" {
		t.Errorf("VISUAL важнее EDITOR: %v", got)
	}
}

// TestRunTrailingNewlineTrimmed — хвостовой перевод строки от редактора не
// попадает в конфиг.
func TestRunTrailingNewlineTrimmed(t *testing.T) {
	fb := &formBuffers{name: "x", run: "echo hi\n\n"}
	if got := fb.command().Run; got != "echo hi" {
		t.Errorf("command().Run = %q", got)
	}
	if got := fb.flag().Run; got != "echo hi" {
		t.Errorf("flag().Run = %q", got)
	}
}

// TestDuplicateCommand — `c` вставляет независимую копию после оригинала с
// уникальным именем; undo убирает копию.
func TestDuplicateCommand(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	cfg := &models.Config{Variables: map[string]string{}, Prefixes: []models.Prefix{{
		Name: "av", Commands: []models.Command{
			{Name: "note", Run: "echo", Flags: []models.Flag{{Name: "--x", Run: "echo x"}},
				Subcommands: []models.Command{{Name: "sub", Run: "echo s"}}},
			{Name: "last"},
		},
	}}}
	m := NewModel(cfg)
	m = feed(m, tea.WindowSizeMsg{Width: 80, Height: 24},
		tea.KeyMsg{Type: tea.KeyEnter}, tea.KeyMsg{Type: tea.KeyEnter}, // -> команды av
		runes("c"))

	cmds := m.cfg.Prefixes[0].Commands
	if len(cmds) != 3 || cmds[1].Name != "note-copy" || cmds[2].Name != "last" {
		t.Fatalf("ожидалась копия note-copy после note: %+v", cmds)
	}
	if m.commandsList.GlobalIndex() != 1 {
		t.Errorf("выделение должно перейти на копию, index=%d", m.commandsList.GlobalIndex())
	}
	cmds[1].Flags[0].Name = "--changed"
	cmds[1].Subcommands[0].Name = "changed"
	if cmds[0].Flags[0].Name != "--x" || cmds[0].Subcommands[0].Name != "sub" {
		t.Fatal("копия должна быть глубокой и не делить срезы с оригиналом")
	}

	m = feed(m, runes("k"), runes("c")) // ещё раз с оригинала
	if got := m.cfg.Prefixes[0].Commands[1].Name; got != "note-copy2" {
		t.Errorf("второе имя копии = %q, ожидалось note-copy2", got)
	}

	m = feed(m, runes("u"), runes("u"))
	if n := len(m.cfg.Prefixes[0].Commands); n != 2 {
		t.Errorf("после двух undo должно остаться 2 команды, получено %d", n)
	}
}

func TestDuplicatePrefixUsesUnderscore(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := NewModel(&models.Config{Variables: map[string]string{}, Prefixes: []models.Prefix{{Name: "av"}}})
	m = feed(m, tea.WindowSizeMsg{Width: 80, Height: 24}, tea.KeyMsg{Type: tea.KeyEnter}, runes("c"))
	if len(m.cfg.Prefixes) != 2 || m.cfg.Prefixes[1].Name != "av_copy" {
		t.Fatalf("ожидался префикс av_copy: %+v", m.cfg.Prefixes)
	}
}

// TestToggleDisabled — `x` выключает команду (она пропадает из скрипта),
// повторное `x` включает; правка через форму не сбрасывает признак.
func TestToggleDisabled(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	cfg := &models.Config{Variables: map[string]string{}, Prefixes: []models.Prefix{{
		Name: "av", Commands: []models.Command{{Name: "note", Run: "echo NOTE_RUN"}},
	}}}
	m := NewModel(cfg)
	m = feed(m, tea.WindowSizeMsg{Width: 80, Height: 24},
		tea.KeyMsg{Type: tea.KeyEnter}, tea.KeyMsg{Type: tea.KeyEnter}, runes("x"))
	if !m.cfg.Prefixes[0].Commands[0].Disabled {
		t.Fatal("после x команда должна быть выключена")
	}
	script, err := os.ReadFile(filepath.Join(home, ".local/share/cli-manager/generated.zsh"))
	if err != nil || strings.Contains(string(script), "NOTE_RUN") {
		t.Fatalf("выключенная команда не должна попадать в скрипт (err=%v)", err)
	}
	if !strings.Contains(m.View(), "⏸ note") {
		t.Errorf("в списке должна быть пометка выключения:\n%s", m.View())
	}

	// Правка формой сохраняет признак.
	m = feed(m, runes("e"))
	m.fb.description = "новое описание"
	m.commitForm()
	if c := m.cfg.Prefixes[0].Commands[0]; !c.Disabled || c.Description != "новое описание" {
		t.Fatalf("правка должна сохранить Disabled: %+v", c)
	}

	m = feed(m, runes("x"))
	if m.cfg.Prefixes[0].Commands[0].Disabled {
		t.Fatal("повторное x должно включить команду")
	}
}

// TestImportAliasesFromBashrc — при оболочке bash alias берутся из ~/.bashrc.
func TestImportAliasesFromBashrc(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	_ = os.WriteFile(filepath.Join(home, ".zshrc"), []byte("alias fromzsh='echo z'\n"), 0o644)
	_ = os.WriteFile(filepath.Join(home, ".bashrc"), []byte("alias frombash='echo b'\n"), 0o644)

	m := NewModel(&models.Config{Shell: "bash", Variables: map[string]string{}})
	m = feed(m, tea.WindowSizeMsg{Width: 80, Height: 24}, tea.KeyMsg{Type: tea.KeyEnter}, runes("i"))
	if len(m.cfg.Prefixes) != 1 || len(m.cfg.Prefixes[0].Commands) != 1 {
		t.Fatalf("ожидалась одна команда: %+v", m.cfg.Prefixes)
	}
	c := m.cfg.Prefixes[0].Commands[0]
	if c.Name != "frombash" || c.Description != "импорт из ~/.bashrc" {
		t.Errorf("ожидался alias из ~/.bashrc, получено %+v", c)
	}
	if !strings.Contains(m.status, "~/.bashrc") {
		t.Errorf("статус должен называть файл: %q", m.status)
	}
}

// TestMoveRefusedWhileFiltered — при поиске соседний на экране элемент не
// сосед в конфиге, поэтому перестановка запрещена.
func TestMoveRefusedWhileFiltered(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := NewModel(&models.Config{Variables: map[string]string{}, Prefixes: []models.Prefix{
		{Name: "aa"}, {Name: "bb"}, {Name: "ab"},
	}})
	m = feed(m, tea.WindowSizeMsg{Width: 80, Height: 24}, tea.KeyMsg{Type: tea.KeyEnter})
	m.prefixesList.SetFilterText("ab")
	m = feed(m, runes("K"))
	if m.cfg.Prefixes[0].Name != "aa" || m.cfg.Prefixes[1].Name != "bb" || m.cfg.Prefixes[2].Name != "ab" {
		t.Fatalf("порядок не должен меняться: %+v", m.cfg.Prefixes)
	}
	if !strings.Contains(m.status, "Сбросьте поиск") {
		t.Errorf("status=%q", m.status)
	}
}
