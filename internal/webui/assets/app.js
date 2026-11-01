// cli-manager web UI — vanilla JS, без зависимостей и сборки.
// Конфиг загружается целиком, правится в памяти и сохраняется целиком (PUT).
"use strict";

const $ = (sel) => document.querySelector(sel);

const state = {
  token: "",
  cfg: null,          // рабочая копия конфига
  version: "",        // хеш config.yaml на момент загрузки/сохранения
  saved: "",          // JSON сохранённого состояния — для индикатора правок
  outputPath: "",
  problems: [],       // [{path, msg}] из ValidateConfig
  sel: null,          // {p, c: [...], f} — выбранный узел
  expanded: new Set(),
  filter: "",
  vars: [],           // [{k, v}] — переменные в порядке редактирования
  tab: "editor",
};

// ── Утилиты ──────────────────────────────────────────────

function el(tag, attrs = {}, ...children) {
  const node = document.createElement(tag);
  for (const [k, v] of Object.entries(attrs)) {
    if (v === undefined || v === null || v === false) continue;
    if (k === "class") node.className = v;
    else if (k.startsWith("on")) node.addEventListener(k.slice(2), v);
    else if (k === "text") node.textContent = v;
    else if (v === true) node.setAttribute(k, "");
    else node.setAttribute(k, v);
  }
  for (const c of children.flat()) {
    if (c === null || c === undefined || c === false) continue;
    node.append(c instanceof Node ? c : document.createTextNode(String(c)));
  }
  return node;
}

function debounce(fn, ms) {
  let t;
  return (...args) => {
    clearTimeout(t);
    t = setTimeout(() => fn(...args), ms);
  };
}

// clean убирает пустые поля так же, как omitempty на сервере: тогда сравнение
// «сохранено/не сохранено» не реагирует на пустые строки и false.
function clean(v) {
  if (Array.isArray(v)) return v.map(clean);
  if (v && typeof v === "object") {
    const out = {};
    for (const [k, val] of Object.entries(v)) {
      if (val === "" || val === false || val === null || val === undefined) continue;
      if (Array.isArray(val) && val.length === 0 && k !== "prefixes") continue;
      if (k === "variables") {
        // Значения переменных не чистим: пустая строка — допустимое значение.
        if (Object.keys(val).length) out[k] = { ...val };
        continue;
      }
      out[k] = clean(val);
    }
    return out;
  }
  return v;
}

function snapshot() {
  return JSON.stringify(clean(state.cfg));
}

let toastTimer;
function toast(msg, isError = false) {
  const t = $("#toast");
  t.textContent = msg;
  t.className = "toast" + (isError ? " error" : "");
  t.hidden = false;
  clearTimeout(toastTimer);
  toastTimer = setTimeout(() => (t.hidden = true), isError ? 6000 : 2500);
}

function banner(content, isError = false) {
  const b = $("#banner");
  b.replaceChildren(...(content ? [content].flat() : []));
  b.className = "banner" + (isError ? " error" : "");
  b.hidden = !content;
}

// ── API ──────────────────────────────────────────────────

async function api(method, path, body) {
  const res = await fetch(path, {
    method,
    headers: { "Content-Type": "application/json", "X-CLI-Manager-Token": state.token },
    body: body === undefined ? undefined : JSON.stringify(body),
  });
  if (res.status === 401) sessionExpired();
  let data = {};
  try {
    data = await res.json();
  } catch {
    data = { error: res.statusText };
  }
  return { status: res.status, data };
}

function sessionExpired() {
  banner("Сессия устарела (сервер перезапущен?). Откройте новую ссылку, которую напечатала команда cli-manager ui.", true);
}

// ── Модель дерева ────────────────────────────────────────

const keyOf = (s) => ["p" + s.p, ...s.c.map((i) => "c" + i), s.f != null ? "f" + s.f : null].filter(Boolean).join("/");
const kindOf = (s) => (s.f != null ? "flag" : s.c.length ? "command" : "prefix");

function resolve(s) {
  if (!s) return null;
  const prefix = state.cfg.prefixes[s.p];
  if (!prefix) return null;
  const chain = [];
  let list = prefix.commands || [];
  for (const i of s.c) {
    const cmd = list[i];
    if (!cmd) return null;
    chain.push(cmd);
    list = cmd.subcommands || [];
  }
  const cmd = chain[chain.length - 1] || null;
  const flag = s.f != null ? (cmd && (cmd.flags || [])[s.f]) || null : null;
  if (s.f != null && !flag) return null;
  return { prefix, chain, cmd, flag, node: flag || cmd || prefix };
}

// siblings — массив, в котором лежит узел, и его индекс.
function siblings(s) {
  const r = resolve(s);
  if (s.f != null) return { list: (r.cmd.flags ||= []), idx: s.f };
  if (s.c.length === 0) return { list: state.cfg.prefixes, idx: s.p };
  const parent = s.c.length === 1 ? r.prefix : r.chain[r.chain.length - 2];
  const field = s.c.length === 1 ? "commands" : "subcommands";
  return { list: (parent[field] ||= []), idx: s.c[s.c.length - 1] };
}

function withIndex(s, idx) {
  if (s.f != null) return { ...s, f: idx };
  if (s.c.length === 0) return { ...s, p: idx };
  return { ...s, c: [...s.c.slice(0, -1), idx] };
}

// problemPath строит путь узла в формате validation.Problem.Path.
function problemPath(s) {
  const r = resolve(s);
  if (!r) return "";
  if (kindOf(s) === "prefix") return "префикс " + r.prefix.name;
  const parts = [r.prefix.name, ...r.chain.map((c) => c.name)];
  if (r.flag) parts.push(r.flag.name);
  return parts.join(" ");
}

function problemsFor(s) {
  const path = problemPath(s);
  return state.problems.filter((p) => p.path === path);
}

function uniqueName(base, taken, sep) {
  if (!taken.includes(base)) return base;
  for (let i = 2; ; i++) {
    const n = base + sep + i;
    if (!taken.includes(n)) return n;
  }
}

// ── Мутации ──────────────────────────────────────────────

function changed({ tree = true, detail = false } = {}) {
  updateDirty();
  if (tree) renderTree();
  if (detail) renderDetail();
  schedulePreview();
}

function select(s) {
  state.sel = s;
  if (s) {
    // Раскрываем всех предков выбранного узла.
    const anc = { p: s.p, c: [], f: null };
    state.expanded.add(keyOf(anc));
    for (const i of s.c) {
      anc.c = [...anc.c, i];
      state.expanded.add(keyOf(anc));
    }
  }
  renderTree();
  renderDetail();
}

function addPrefix() {
  const taken = state.cfg.prefixes.map((p) => p.name);
  state.cfg.prefixes.push({ name: uniqueName("new", taken, "_"), commands: [] });
  changed();
  select({ p: state.cfg.prefixes.length - 1, c: [], f: null });
  focusName();
}

function addCommand(s) {
  const r = resolve(s);
  const parent = r.cmd || r.prefix;
  const field = r.cmd ? "subcommands" : "commands";
  const list = (parent[field] ||= []);
  list.push({ name: uniqueName("cmd", list.map((c) => c.name), "-"), run: "" });
  changed();
  select({ p: s.p, c: [...s.c, list.length - 1], f: null });
  focusName();
}

function addFlag(s) {
  const r = resolve(s);
  const list = (r.cmd.flags ||= []);
  list.push({ name: uniqueName("--flag", list.map((f) => f.name), "-"), run: "" });
  changed();
  select({ ...s, f: list.length - 1 });
  focusName();
}

function duplicate(s) {
  const { list, idx } = siblings(s);
  const copy = structuredClone(list[idx]);
  copy.name = uniqueName(copy.name + (kindOf(s) === "prefix" ? "_copy" : "-copy"), list.map((x) => x.name), "");
  list.splice(idx + 1, 0, copy);
  changed();
  select(withIndex(s, idx + 1));
}

function move(s, delta) {
  const { list, idx } = siblings(s);
  const j = idx + delta;
  if (j < 0 || j >= list.length) return;
  [list[idx], list[j]] = [list[j], list[idx]];
  changed();
  select(withIndex(s, j));
}

function remove(s) {
  const r = resolve(s);
  const kind = { prefix: "префикс", command: "команду", flag: "флаг" }[kindOf(s)];
  const nested = r.node.commands?.length || r.node.subcommands?.length || r.node.flags?.length;
  const extra = nested ? " вместе со всем вложенным" : "";
  if (!confirm(`Удалить ${kind} «${r.node.name}»${extra}?`)) return;
  const { list, idx } = siblings(s);
  list.splice(idx, 1);
  state.sel = null;
  changed({ detail: true });
}

function focusName() {
  const input = $("#f-name");
  if (input) {
    input.focus();
    input.select();
  }
}

// ── Дерево ───────────────────────────────────────────────

function matches(node) {
  const q = state.filter;
  return !q || node.name.toLowerCase().includes(q) || (node.description || "").toLowerCase().includes(q);
}

// subtreeMatches — есть ли совпадение с фильтром в узле или его потомках.
function subtreeMatches(node) {
  if (matches(node)) return true;
  const kids = [...(node.commands || []), ...(node.subcommands || []), ...(node.flags || [])];
  return kids.some(subtreeMatches);
}

function renderTree() {
  const root = $("#tree");
  const prefixes = state.cfg.prefixes;
  if (prefixes.length === 0) {
    root.replaceChildren(el("li", { class: "tree-empty", text: "Префиксов пока нет" }));
    return;
  }
  const items = prefixes
    .map((p, i) => renderNode({ p: i, c: [], f: null }, p))
    .filter(Boolean);
  root.replaceChildren(...(items.length ? items : [el("li", { class: "tree-empty", text: "Ничего не найдено" })]));
}

function renderNode(s, node) {
  if (state.filter && !subtreeMatches(node)) return null;
  const kind = kindOf(s);
  const key = keyOf(s);
  const children = [];
  if (kind !== "flag") {
    const cmds = kind === "prefix" ? node.commands || [] : node.subcommands || [];
    cmds.forEach((c, i) => children.push(renderNode({ p: s.p, c: [...s.c, i], f: null }, c)));
    if (kind === "command") (node.flags || []).forEach((f, i) => children.push(renderNode({ ...s, f: i }, f)));
  }
  const kids = children.filter(Boolean);
  const open = state.filter ? true : state.expanded.has(key);
  const selected = state.sel && keyOf(state.sel) === key;
  const hasProblem = problemsFor(s).length > 0;

  const label = { prefix: "pfx", command: node.subcommands?.length ? "grp" : "cmd", flag: "flag" }[kind];
  const row = el(
    "div",
    {
      class: "node" + (selected ? " selected" : "") + (node.disabled ? " disabled" : ""),
      role: "treeitem",
      "aria-selected": selected ? "true" : "false",
      "aria-expanded": kids.length ? String(open) : undefined,
      tabindex: "0",
      onclick: () => select(s),
      onkeydown: (e) => {
        if (e.key === "Enter" || e.key === " ") {
          e.preventDefault();
          select(s);
        }
      },
    },
    el("button", {
      class: "caret" + (kids.length ? "" : " empty"),
      "aria-label": open ? "Свернуть" : "Развернуть",
      tabindex: "-1",
      text: open ? "▾" : "▸",
      onclick: (e) => {
        e.stopPropagation();
        if (open) state.expanded.delete(key);
        else state.expanded.add(key);
        renderTree();
      },
    }),
    el("span", { class: "kind kind-" + kind, text: label }),
    el("span", { class: "node-name", text: node.name || "(без имени)" }),
    el("span", { class: "node-desc", text: node.description || "" }),
    hasProblem ? el("span", { class: "dot", title: "Есть ошибки" }) : null,
  );
  return el("li", {}, row, kids.length && open ? el("ul", { role: "group" }, kids) : null);
}

// ── Форма узла ───────────────────────────────────────────

function field(label, control, { hint, id } = {}) {
  const msg = el("div", { class: "msg", id: id ? id + "-msg" : undefined });
  return el(
    "div",
    { class: "field" },
    el("label", { class: "field-label", for: control.id, text: label }),
    control,
    msg,
    hint ? el("div", { class: "hint", text: hint }) : null,
  );
}

// setMsg пишет результат проверки под полем input.
function setMsg(input, kind, text) {
  const m = input.closest(".field")?.querySelector(".msg");
  if (!m) return;
  m.className = "msg" + (kind ? " " + kind : "");
  m.textContent = text || "";
  input.classList.toggle("invalid", kind === "error");
}

function textInput(id, node, key, { mono = false, placeholder, onChange } = {}) {
  return el("input", {
    type: "text",
    id,
    class: mono ? "mono" : undefined,
    value: node[key] || "",
    placeholder,
    spellcheck: "false",
    autocomplete: "off",
    oninput: (e) => {
      node[key] = e.target.value;
      onChange ? onChange(e.target.value) : changed();
    },
  });
}

function codeArea(id, node, key, onChange) {
  const ta = el("textarea", {
    id,
    class: "mono",
    spellcheck: "false",
    rows: "6",
    placeholder: "bash-код",
    oninput: (e) => {
      node[key] = e.target.value;
      onChange(e.target.value);
    },
    onkeydown: (e) => {
      // Tab вставляет отступ, а не уводит фокус (Shift+Tab по-прежнему уводит).
      if (e.key === "Tab" && !e.shiftKey) {
        e.preventDefault();
        const { selectionStart: a, selectionEnd: b, value } = e.target;
        e.target.value = value.slice(0, a) + "\t" + value.slice(b);
        e.target.selectionStart = e.target.selectionEnd = a + 1;
        e.target.dispatchEvent(new Event("input"));
      }
    },
  });
  ta.value = node[key] || "";
  return ta;
}

function checkbox(id, node, key, label, hint) {
  return el(
    "label",
    { class: "check", for: id },
    el("input", {
      type: "checkbox",
      id,
      checked: !!node[key],
      onchange: (e) => {
        node[key] = e.target.checked;
        changed();
      },
    }),
    el("span", {}, el("span", { text: label }), hint ? el("div", { class: "hint", text: hint }) : null),
  );
}

// fieldCheck — живая проверка поля на сервере с debounce: свой таймер на
// каждое поле (по id), а элемент запоминается в момент ввода — ответ,
// пришедший после перерисовки формы (другой узел), отбрасывается.
function fieldCheck(fn, ms) {
  const timers = new Map();
  return (id, ...args) => {
    const input = document.getElementById(id);
    if (!input) return;
    clearTimeout(timers.get(id));
    timers.set(id, setTimeout(async () => {
      const res = await fn(...args);
      if (input.isConnected && res) setMsg(input, res[0], res[1]);
    }, ms));
  };
}

// Проверки на сервере: bash -n (код не выполняется) и формат имени.
const checkCode = fieldCheck(async (kind, code) => {
  if (!code.trim()) return ["", ""];
  const { status, data } = await api("POST", "/api/check", { kind, code });
  if (status !== 200) return null;
  return data.ok ? ["ok", "синтаксис в порядке"] : ["error", data.error];
}, 400);

const checkName = fieldCheck(async (kind, name) => {
  const { status, data } = await api("POST", "/api/validate-name", { kind, name, shell: state.cfg.shell || "zsh" });
  if (status !== 200) return null;
  if (!data.ok) return ["error", data.error];
  return data.warning ? ["warn", data.warning] : ["", ""];
}, 300);

function renderDetail() {
  const pane = $("#detail");
  const s = state.sel;
  const r = resolve(s);
  if (!r) {
    state.sel = null;
    pane.replaceChildren(emptyDetail());
    return;
  }
  const kind = kindOf(s);
  const node = r.node;
  const crumbs = [r.prefix.name, ...r.chain.map((c) => c.name), r.flag?.name].filter(Boolean).join(" ");
  const { list, idx } = siblings(s);

  const toolbar = el(
    "div",
    { class: "toolbar" },
    kind !== "flag"
      ? el("button", { class: "btn btn-small", onclick: () => addCommand(s), text: kind === "prefix" ? "+ Команда" : "+ Подкоманда" })
      : null,
    kind === "command" ? el("button", { class: "btn btn-small", onclick: () => addFlag(s), text: "+ Флаг" }) : null,
    el("button", { class: "btn btn-small", onclick: () => duplicate(s), text: "Дублировать" }),
    el("button", { class: "btn btn-small", onclick: () => move(s, -1), disabled: idx === 0, title: "Выше", "aria-label": "Выше", text: "↑" }),
    el("button", { class: "btn btn-small", onclick: () => move(s, 1), disabled: idx === list.length - 1, title: "Ниже", "aria-label": "Ниже", text: "↓" }),
    el("button", { class: "btn btn-small btn-danger", onclick: () => remove(s), text: "Удалить" }),
  );

  const title = { prefix: "Префикс", command: node.subcommands?.length ? "Группа команд" : "Команда", flag: "Флаг" }[kind];
  const head = el(
    "div",
    { class: "detail-head" },
    el("div", {}, el("div", { class: "crumbs", text: crumbs }), el("h2", { text: title })),
    toolbar,
  );

  const nameKind = kind === "prefix" ? "prefix" : kind;
  const nameHint = {
    prefix: "Станет именем shell-функции: буквы, цифры и _",
    command: "Буквы, цифры, _ и -",
    flag: "Например --force, -f или build",
  }[kind];
  const nameField = field(
    "Имя",
    textInput("f-name", node, "name", {
      mono: true,
      onChange: (v) => {
        changed();
        checkName("f-name", nameKind, v);
        const c = $(".crumbs");
        if (c) c.textContent = [r.prefix.name, ...r.chain.map((x) => x.name), r.flag?.name].filter(Boolean).join(" ");
      },
    }),
    { hint: nameHint, id: "f-name" },
  );

  const form = el("div", { class: "form" }, nameField, field("Описание", textInput("f-desc", node, "description"), { id: "f-desc" }));

  if (kind === "command" || kind === "flag") {
    const isBranch = kind === "command" && node.subcommands?.length;
    form.append(
      field("Действие (run)", codeArea("f-run", node, "run", (v) => {
        changed({ tree: false });
        checkCode("f-run", "run", v);
      }), {
        id: "f-run",
        hint: isBranch
          ? "У группы run выполняется, если подкоманда не распознана"
          : "Проверяется через bash -n, код не запускается",
      }),
    );
  }
  if (kind === "command") {
    form.append(
      el(
        "div",
        { class: "row-2" },
        field("Аргументы для справки (usage)", textInput("f-usage", node, "usage", { mono: true, placeholder: "<файл> [строки]" }), { id: "f-usage" }),
        field("Условие (condition)", textInput("f-cond", node, "condition", {
          mono: true,
          placeholder: "[[ -f docker-compose.yml ]]",
          onChange: (v) => {
            changed({ tree: false });
            checkCode("f-cond", "condition", v);
          },
        }), { id: "f-cond", hint: "Команда видна в автодополнении, только если условие истинно" }),
      ),
      checkbox("f-args", node, "accepts_args", "Принимает аргументы", "Аргументы пробрасываются в run как $@"),
    );
  }
  if (kind === "flag") {
    form.append(checkbox("f-after", node, "after_command", "Сначала выполнить действие команды",
      "Например, команда делает cd, а флаг запускает инструмент в этой папке"));
  }
  form.append(checkbox("f-disabled", node, "disabled", "Выключено", "Остаётся в конфиге, но не попадает в скрипт"));

  if (kind === "command" && node.subcommands?.length && node.flags?.length) {
    form.append(el("div", { class: "note warn", text: "У команды есть подкоманды — при генерации флаги игнорируются." }));
  }

  pane.replaceChildren(head, form);
  renderDetailProblemsOnly();

  // Быстрая навигация по вложенным узлам.
  const kids = [];
  if (kind === "prefix") (node.commands || []).forEach((c, i) => kids.push([c.name, { p: s.p, c: [i], f: null }]));
  if (kind === "command") {
    (node.subcommands || []).forEach((c, i) => kids.push([c.name, { p: s.p, c: [...s.c, i], f: null }]));
    (node.flags || []).forEach((f, i) => kids.push([f.name, { ...s, f: i }]));
  }
  const childBlock = kids.length
    ? el(
        "div",
        { class: "children" },
        el("h3", { text: "Вложенные" }),
        el("div", { class: "chips" }, kids.map(([name, ks]) => el("button", { class: "chip", text: name, onclick: () => select(ks) }))),
      )
    : null;

  if (childBlock) pane.append(childBlock);

  // Начальные проверки для уже заполненных полей.
  if (node.run) checkCode("f-run", "run", node.run);
  if (node.condition) checkCode("f-cond", "condition", node.condition);
  if (kind === "prefix" && node.name) checkName("f-name", "prefix", node.name);
}

function emptyDetail() {
  if (state.cfg.prefixes.length === 0) {
    return el(
      "div",
      { class: "empty-state" },
      el("h2", { text: "Начните с префикса" }),
      el("p", { text: "Префикс — корневая команда, например av. Внутри него — команды, подкоманды и флаги: av db migrate up." }),
      el("button", { class: "btn btn-primary", onclick: addPrefix, text: "+ Добавить префикс" }),
    );
  }
  return el("div", { class: "empty-state" }, el("p", { text: "Выберите узел в дереве слева" }));
}

// ── Переменные ───────────────────────────────────────────

const reIdent = /^[a-zA-Z_][a-zA-Z0-9_]*$/;

function syncVars() {
  const obj = {};
  for (const { k, v } of state.vars) if (k !== "") obj[k] = v;
  state.cfg.variables = obj;
  changed({ tree: false });
  renderVarMessages();
}

function renderVars() {
  const box = $("#vars");
  if (state.vars.length === 0) {
    box.replaceChildren(el("p", { class: "muted", text: "Переменных нет." }));
    return;
  }
  box.replaceChildren(
    ...state.vars.map((row, i) =>
      el(
        "div",
        { class: "var-row" },
        el("input", {
          type: "text", class: "mono", value: row.k, placeholder: "NAME", spellcheck: "false",
          "aria-label": "Имя переменной", id: "var-k-" + i,
          oninput: (e) => { row.k = e.target.value; syncVars(); },
        }),
        el("input", {
          type: "text", class: "mono var-value", value: row.v, placeholder: "значение", spellcheck: "false",
          "aria-label": "Значение",
          oninput: (e) => { row.v = e.target.value; syncVars(); },
        }),
        el("button", {
          class: "btn btn-small btn-danger", "aria-label": "Удалить переменную", text: "✕",
          onclick: () => { state.vars.splice(i, 1); syncVars(); renderVars(); },
        }),
        el("div", { class: "msg", id: "var-msg-" + i }),
      ),
    ),
  );
  renderVarMessages();
}

function renderVarMessages() {
  const seen = new Map();
  state.vars.forEach(({ k }, i) => {
    const m = document.getElementById("var-msg-" + i);
    const input = document.getElementById("var-k-" + i);
    if (!m) return;
    let msg = "";
    if (k === "") msg = "имя не может быть пустым";
    else if (!reIdent.test(k)) msg = "разрешены буквы, цифры и _, первый символ — буква или _";
    else if (seen.has(k)) msg = "переменная с таким именем уже есть — сохранится последняя";
    seen.set(k, true);
    m.textContent = msg;
    input.classList.toggle("invalid", !!msg);
  });
}

function addVar() {
  state.vars.push({ k: "", v: "" });
  renderVars();
  document.getElementById("var-k-" + (state.vars.length - 1))?.focus();
}

// ── Превью и проблемы ────────────────────────────────────

let previewSeq = 0;
const schedulePreview = debounce(async () => {
  const seq = ++previewSeq;
  const { status, data } = await api("POST", "/api/preview", clean(state.cfg));
  if (seq !== previewSeq || status !== 200) return;
  $("#script").textContent = data.script;
  setProblems(data.problems || []);
}, 500);

function setProblems(problems) {
  state.problems = problems;
  const box = $("#problems");
  if (problems.length === 0) {
    box.hidden = true;
  } else {
    box.hidden = false;
    box.replaceChildren(
      el("strong", { text: `Ошибки в конфиге (${problems.length}) — сохранение недоступно, пока они есть:` }),
      el("ul", {}, problems.map((p) => el("li", {}, el("button", { onclick: () => goToProblem(p), text: p.path }), ": " + p.msg))),
    );
  }
  renderTree();
  if (state.sel) renderDetailProblemsOnly();
}

// Обновляем только блок ошибок в форме, чтобы не сбивать фокус ввода.
function renderDetailProblemsOnly() {
  const form = $("#detail .form");
  if (!form) return;
  const first = form.firstElementChild;
  if (first && first.classList.contains("note") && first.dataset.problems) first.remove();
  const probs = problemsFor(state.sel);
  if (probs.length) {
    const note = el("div", { class: "note warn" }, probs.map((p) => el("div", { text: p.msg })));
    note.dataset.problems = "1";
    form.prepend(note);
  }
}

function goToProblem(p) {
  if (p.path.startsWith("переменная ")) return switchTab("vars");
  switchTab("editor");
  const found = findByPath(p.path);
  if (found) select(found);
}

function findByPath(path) {
  let hit = null;
  const visit = (s, node) => {
    if (hit) return;
    if (problemPath(s) === path) return (hit = s);
    const kind = kindOf(s);
    if (kind === "flag") return;
    const cmds = kind === "prefix" ? node.commands || [] : node.subcommands || [];
    cmds.forEach((c, i) => visit({ p: s.p, c: [...s.c, i], f: null }, c));
    if (kind === "command") (node.flags || []).forEach((f, i) => visit({ ...s, f: i }, f));
  };
  state.cfg.prefixes.forEach((p, i) => visit({ p: i, c: [], f: null }, p));
  return hit;
}

// ── Сохранение и загрузка ────────────────────────────────

function updateDirty() {
  const dirty = snapshot() !== state.saved;
  $("#dirty").hidden = !dirty;
  $("#save").disabled = !dirty;
  document.title = (dirty ? "● " : "") + "cli-manager";
  return dirty;
}

function applyLoaded(data) {
  state.cfg = data.config;
  state.cfg.prefixes ||= [];
  state.cfg.variables ||= {};
  state.version = data.version;
  state.outputPath = data.outputPath;
  state.saved = snapshot();
  state.vars = Object.keys(state.cfg.variables).sort().map((k) => ({ k, v: state.cfg.variables[k] }));
  $("#shell").value = (state.cfg.shell || "zsh").toLowerCase() === "bash" ? "bash" : "zsh";
  $("#output-path").textContent = data.outputPath;
}

async function load() {
  const { status, data } = await api("GET", "/api/config");
  if (status !== 200) {
    banner(data.error || "Не удалось загрузить конфиг", true);
    return false;
  }
  applyLoaded(data);
  if (state.sel && !resolve(state.sel)) state.sel = null;
  banner(null);
  renderTree();
  renderDetail();
  renderVars();
  updateDirty();
  setProblems(data.problems || []);
  schedulePreview();
  return true;
}

async function save() {
  if (!updateDirty()) return;
  const btn = $("#save");
  btn.disabled = true;
  const { status, data } = await api("PUT", "/api/config", { config: clean(state.cfg), version: state.version });
  if (status === 409) {
    banner([
      el("span", { text: "Конфиг изменился на диске (в TUI или вручную), пока вы его правили." }),
      el("button", {
        class: "btn btn-small",
        text: "Загрузить с диска (ваши правки пропадут)",
        onclick: () => load().then((ok) => ok && toast("Загружено с диска")),
      }),
    ]);
    updateDirty();
    return;
  }
  if (status === 422) {
    setProblems(data.problems || []);
    toast(data.error, true);
    updateDirty();
    return;
  }
  if (status !== 200) {
    toast(data.error || "Ошибка сохранения", true);
    updateDirty();
    return;
  }
  state.version = data.version;
  state.outputPath = data.outputPath;
  $("#output-path").textContent = data.outputPath;
  state.saved = snapshot();
  updateDirty();
  setProblems(data.problems || []);
  if (data.generateError) {
    banner("Конфиг сохранён, но скрипт не перегенерирован — остался прежний: " + data.generateError, true);
  } else {
    banner(null);
    toast("Сохранено и скомпилировано ✓");
  }
}

// ── Вкладки и запуск ─────────────────────────────────────

function switchTab(tab) {
  state.tab = tab;
  for (const b of document.querySelectorAll(".tab")) b.setAttribute("aria-selected", String(b.dataset.tab === tab));
  for (const name of ["editor", "vars", "preview"]) $("#tab-" + name).hidden = name !== tab;
  if (tab === "preview") schedulePreview();
}

function readToken() {
  const m = location.hash.match(/token=([0-9a-f]+)/);
  if (m) {
    try { sessionStorage.setItem("cli-manager-token", m[1]); } catch { /* приватный режим */ }
    history.replaceState(null, "", location.pathname);
    return m[1];
  }
  try { return sessionStorage.getItem("cli-manager-token") || ""; } catch { return ""; }
}

function init() {
  state.token = readToken();
  for (const b of document.querySelectorAll(".tab")) b.addEventListener("click", () => switchTab(b.dataset.tab));
  $("#add-prefix").addEventListener("click", addPrefix);
  $("#add-var").addEventListener("click", addVar);
  $("#save").addEventListener("click", save);
  $("#shell").addEventListener("change", (e) => {
    const cur = (state.cfg.shell || "").toLowerCase();
    state.cfg.shell = e.target.value === "bash" ? "bash" : cur === "zsh" ? state.cfg.shell : "";
    changed({ tree: false });
  });
  $("#tree-filter").addEventListener("input", (e) => {
    state.filter = e.target.value.trim().toLowerCase();
    renderTree();
  });
  $("#copy-script").addEventListener("click", async () => {
    try {
      await navigator.clipboard.writeText($("#script").textContent);
      toast("Скрипт скопирован");
    } catch {
      toast("Не удалось скопировать", true);
    }
  });
  document.addEventListener("keydown", (e) => {
    if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === "s") {
      e.preventDefault();
      save();
    }
  });
  window.addEventListener("beforeunload", (e) => {
    if (state.cfg && snapshot() !== state.saved) e.preventDefault();
  });

  // Новая ссылка в той же вкладке меняет только hash — страница не
  // перезагружается, поэтому подхватываем токен сами.
  window.addEventListener("hashchange", () => {
    if (!/token=/.test(location.hash)) return;
    state.token = readToken();
    if (state.cfg && snapshot() !== state.saved && !confirm("Загрузить конфиг заново? Несохранённые правки пропадут.")) return;
    load();
  });

  if (!state.token) {
    banner("Нет токена доступа. Откройте ссылку, которую напечатала команда cli-manager ui.", true);
    return;
  }
  load();
}

init();
