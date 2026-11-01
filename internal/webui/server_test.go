package webui

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"cli-manager/internal/compiler"
	"cli-manager/internal/models"
	"cli-manager/internal/storage"
)

const testAddr = "127.0.0.1:4242"

func newTestServer(t *testing.T) *Server {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("ZDOTDIR", "")
	s, err := New()
	if err != nil {
		t.Fatal(err)
	}
	s.SetAddr(testAddr)
	return s
}

// do выполняет запрос к серверу; по умолчанию с верными Host и токеном.
func do(t *testing.T, s *Server, method, path string, body any, mod ...func(*http.Request)) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatal(err)
		}
	}
	req := httptest.NewRequest(method, "http://"+testAddr+path, &buf)
	req.Host = testAddr
	req.Header.Set(TokenHeader, s.Token())
	for _, m := range mod {
		m(req)
	}
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	return rec
}

func decode(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &m); err != nil {
		t.Fatalf("ответ не JSON: %v: %s", err, rec.Body.String())
	}
	return m
}

func TestAccessControl(t *testing.T) {
	s := newTestServer(t)
	cases := []struct {
		name string
		mod  func(*http.Request)
		want int
	}{
		{"без токена", func(r *http.Request) { r.Header.Del(TokenHeader) }, http.StatusUnauthorized},
		{"чужой токен", func(r *http.Request) { r.Header.Set(TokenHeader, "deadbeef") }, http.StatusUnauthorized},
		{"чужой Host", func(r *http.Request) { r.Host = "evil.example:4242" }, http.StatusForbidden},
		{"чужой порт", func(r *http.Request) { r.Host = "127.0.0.1:1" }, http.StatusForbidden},
		{"чужой Origin", func(r *http.Request) { r.Header.Set("Origin", "http://evil.example") }, http.StatusForbidden},
		{"localhost разрешён", func(r *http.Request) { r.Host = "localhost:4242" }, http.StatusOK},
		{"свой Origin разрешён", func(r *http.Request) { r.Header.Set("Origin", "http://"+testAddr) }, http.StatusOK},
	}
	for _, c := range cases {
		rec := do(t, s, "GET", "/api/config", nil, c.mod)
		if rec.Code != c.want {
			t.Errorf("%s: код %d, ждали %d (%s)", c.name, rec.Code, c.want, rec.Body.String())
		}
	}
}

func TestStaticServedWithoutToken(t *testing.T) {
	s := newTestServer(t)
	rec := do(t, s, "GET", "/", nil, func(r *http.Request) { r.Header.Del(TokenHeader) })
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "app.js") {
		t.Fatalf("код %d: %s", rec.Code, rec.Body.String())
	}
	if csp := rec.Header().Get("Content-Security-Policy"); !strings.Contains(csp, "default-src 'self'") {
		t.Errorf("нет CSP: %q", csp)
	}
}

func sampleConfig() *models.Config {
	return &models.Config{
		Variables: map[string]string{"PROJ": "/tmp/proj"},
		Prefixes: []models.Prefix{{
			Name: "av",
			Commands: []models.Command{{
				Name: "hello",
				Run:  "echo hello",
				Flags: []models.Flag{{
					Name: "--loud",
					Run:  "echo HELLO",
				}},
			}},
		}},
	}
}

func TestSaveRoundTrip(t *testing.T) {
	s := newTestServer(t)

	got := decode(t, do(t, s, "GET", "/api/config", nil))
	if got["version"] != "" {
		t.Fatalf("без файла версия должна быть пустой: %v", got["version"])
	}

	rec := do(t, s, "PUT", "/api/config", map[string]any{"config": sampleConfig(), "version": ""})
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT: %d %s", rec.Code, rec.Body.String())
	}
	put := decode(t, rec)
	if put["version"] == "" || put["generateError"] != nil {
		t.Fatalf("PUT: %v", put)
	}

	cfg, err := storage.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Prefixes) != 1 || cfg.Prefixes[0].Commands[0].Flags[0].Name != "--loud" || cfg.Variables["PROJ"] != "/tmp/proj" {
		t.Fatalf("на диске не то: %+v", cfg)
	}
	script, err := os.ReadFile(compiler.For("").OutputPath())
	if err != nil || !strings.Contains(string(script), "echo HELLO") {
		t.Fatalf("скрипт не сгенерирован: %v", err)
	}

	again := decode(t, do(t, s, "GET", "/api/config", nil))
	if again["version"] != put["version"] {
		t.Errorf("версия после GET %v, после PUT %v", again["version"], put["version"])
	}
}

func TestSaveConflict(t *testing.T) {
	s := newTestServer(t)
	if err := storage.SaveConfig(sampleConfig()); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(storage.ConfigPath())

	cfg := sampleConfig()
	cfg.Prefixes[0].Name = "changed"
	rec := do(t, s, "PUT", "/api/config", map[string]any{"config": cfg, "version": "stale"})
	if rec.Code != http.StatusConflict {
		t.Fatalf("ждали 409, получили %d %s", rec.Code, rec.Body.String())
	}
	after, _ := os.ReadFile(storage.ConfigPath())
	if !bytes.Equal(before, after) {
		t.Error("при конфликте файл изменился")
	}
}

func TestSaveRejectsInvalid(t *testing.T) {
	s := newTestServer(t)
	cfg := sampleConfig()
	cfg.Prefixes = append(cfg.Prefixes, models.Prefix{Name: "bad name"})
	rec := do(t, s, "PUT", "/api/config", map[string]any{"config": cfg, "version": ""})
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("ждали 422, получили %d %s", rec.Code, rec.Body.String())
	}
	if ps, _ := decode(t, rec)["problems"].([]any); len(ps) == 0 {
		t.Error("нет списка проблем")
	}
	if _, err := os.Stat(storage.ConfigPath()); !os.IsNotExist(err) {
		t.Error("невалидный конфиг записан на диск")
	}
}

func TestPreviewMatchesRender(t *testing.T) {
	s := newTestServer(t)
	cfg := sampleConfig()
	rec := do(t, s, "POST", "/api/preview", cfg)
	if rec.Code != http.StatusOK {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	if got := decode(t, rec)["script"]; got != compiler.For("").Render(cfg) {
		t.Error("превью не совпадает с Render")
	}
	// Превью ничего не пишет на диск.
	if _, err := os.Stat(compiler.For("").OutputPath()); !os.IsNotExist(err) {
		t.Error("превью записало скрипт")
	}
}

func TestCheck(t *testing.T) {
	s := newTestServer(t)
	cases := []struct {
		kind, code string
		ok         bool
	}{
		{"run", "echo ok", true},
		{"run", "if then fi (", false},
		{"condition", "[[ -f x ]]", true},
		{"condition", "[[ -f", false},
	}
	for _, c := range cases {
		res := decode(t, do(t, s, "POST", "/api/check", map[string]string{"kind": c.kind, "code": c.code}))
		if res["ok"] != c.ok {
			t.Errorf("%s %q: %v", c.kind, c.code, res)
		}
	}
	if rec := do(t, s, "POST", "/api/check", map[string]string{"kind": "exec", "code": "x"}); rec.Code != http.StatusBadRequest {
		t.Errorf("неизвестный kind: %d", rec.Code)
	}
}

func TestValidateName(t *testing.T) {
	s := newTestServer(t)
	cases := []struct {
		kind, name string
		ok         bool
	}{
		{"prefix", "av", true},
		{"prefix", "my-cmd", false},
		{"command", "db-migrate", true},
		{"flag", "--force", true},
		{"flag", "--", false},
		{"variable", "2foo", false},
	}
	for _, c := range cases {
		res := decode(t, do(t, s, "POST", "/api/validate-name", map[string]string{"kind": c.kind, "name": c.name}))
		if res["ok"] != c.ok {
			t.Errorf("%s %q: %v", c.kind, c.name, res)
		}
	}
	// Имя системной утилиты — предупреждение о коллизии.
	res := decode(t, do(t, s, "POST", "/api/validate-name", map[string]string{"kind": "prefix", "name": "ls"}))
	if res["ok"] != true || res["warning"] == nil {
		t.Errorf("ls: ждали предупреждение: %v", res)
	}
}

func TestBodyLimit(t *testing.T) {
	s := newTestServer(t)
	big := map[string]string{"kind": "run", "code": strings.Repeat("x", maxBody+1)}
	if rec := do(t, s, "POST", "/api/check", big); rec.Code != http.StatusBadRequest {
		t.Errorf("большое тело: код %d", rec.Code)
	}
}
