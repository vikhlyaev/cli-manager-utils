// Package webui — локальный веб-интерфейс cli-manager (подкоманда `ui`).
//
// Фронтенд загружает конфиг целиком, правит его в памяти и сохраняет целиком.
// Сервер — тонкий слой над storage, compiler и validation: своей бизнес-логики
// у него нет. Код из браузера здесь не выполняется — только `bash -n`.
package webui

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"

	"cli-manager/internal/compiler"
	"cli-manager/internal/importer"
	"cli-manager/internal/models"
	"cli-manager/internal/storage"
	"cli-manager/internal/validation"
)

//go:embed assets
var assets embed.FS

// TokenHeader — заголовок, в котором страница передаёт токен сессии.
const TokenHeader = "X-CLI-Manager-Token"

// maxBody — предел размера тела запроса.
const maxBody = 1 << 20

// Server обслуживает статику и /api/*. Создаётся через New.
type Server struct {
	token string
	// addr — адрес, на котором слушает сервер ("127.0.0.1:port"); от него
	// зависят допустимые Host и Origin. Выставляется через SetAddr.
	addr string
	mu   sync.Mutex // сериализует чтение-сверку-запись конфига
	mux  *http.ServeMux
}

// New создаёт сервер со случайным токеном.
func New() (*Server, error) {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return nil, err
	}
	s := &Server{token: hex.EncodeToString(b)}

	static, err := fs.Sub(assets, "assets")
	if err != nil {
		return nil, err
	}
	s.mux = http.NewServeMux()
	s.mux.Handle("GET /", http.FileServerFS(static))
	s.mux.HandleFunc("GET /api/config", s.getConfig)
	s.mux.HandleFunc("PUT /api/config", s.putConfig)
	s.mux.HandleFunc("POST /api/preview", s.preview)
	s.mux.HandleFunc("POST /api/check", s.check)
	s.mux.HandleFunc("POST /api/validate-name", s.validateName)
	return s, nil
}

// Token возвращает токен сессии (передаётся странице в URL).
func (s *Server) Token() string { return s.token }

// SetAddr задаёт адрес, на котором фактически слушает сервер.
func (s *Server) SetAddr(addr string) { s.addr = addr }

// URL возвращает адрес страницы с токеном.
func (s *Server) URL() string { return "http://" + s.addr + "/#token=" + s.token }

// ServeHTTP проверяет Host, а для /api/* — ещё токен и Origin.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !s.hostAllowed(r.Host) {
		http.Error(w, "недопустимый Host", http.StatusForbidden)
		return
	}
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Frame-Options", "DENY")
	w.Header().Set("Content-Security-Policy", "default-src 'self'; frame-ancestors 'none'")

	if strings.HasPrefix(r.URL.Path, "/api/") {
		got := r.Header.Get(TokenHeader)
		if subtle.ConstantTimeCompare([]byte(got), []byte(s.token)) != 1 {
			http.Error(w, "нет или неверный токен", http.StatusUnauthorized)
			return
		}
		if o := r.Header.Get("Origin"); o != "" && !s.originAllowed(o) {
			http.Error(w, "недопустимый Origin", http.StatusForbidden)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, maxBody)
		w.Header().Set("Cache-Control", "no-store")
	}
	s.mux.ServeHTTP(w, r)
}

// hostAllowed пропускает только 127.0.0.1:port и localhost:port — защита от
// DNS rebinding (чужой домен, указывающий на 127.0.0.1).
func (s *Server) hostAllowed(host string) bool {
	_, port, err := net.SplitHostPort(s.addr)
	if err != nil {
		return false
	}
	return host == "127.0.0.1:"+port || host == "localhost:"+port
}

func (s *Server) originAllowed(origin string) bool {
	return strings.HasPrefix(origin, "http://") && s.hostAllowed(strings.TrimPrefix(origin, "http://"))
}

// problem — проблема конфига в JSON.
type problem struct {
	Path string `json:"path"`
	Msg  string `json:"msg"`
}

func toProblems(ps []validation.Problem) []problem {
	out := make([]problem, 0, len(ps))
	for _, p := range ps {
		out = append(out, problem{p.Path, p.Msg})
	}
	return out
}

type configResponse struct {
	Config     *models.Config `json:"config"`
	Version    string         `json:"version"`
	OutputPath string         `json:"outputPath"`
	ConfigPath string         `json:"configPath"`
	Problems   []problem      `json:"problems"`
	// GenerateError — конфиг сохранён, но скрипт перегенерировать не удалось.
	GenerateError string `json:"generateError,omitempty"`
}

// diskVersion возвращает хеш config.yaml на диске ("" — файла нет).
func diskVersion() (string, error) {
	data, err := os.ReadFile(storage.ConfigPath())
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

func (s *Server) getConfig(w http.ResponseWriter, _ *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()

	cfg, err := storage.LoadConfig()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "ошибка загрузки конфига: "+err.Error())
		return
	}
	ver, err := diskVersion()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, s.response(cfg, ver))
}

func (s *Server) response(cfg *models.Config, ver string) configResponse {
	return configResponse{
		Config:     cfg,
		Version:    ver,
		OutputPath: compiler.For(cfg.Shell).OutputPath(),
		ConfigPath: storage.ConfigPath(),
		Problems:   toProblems(validation.ValidateConfig(cfg)),
	}
}

type putRequest struct {
	Config  *models.Config `json:"config"`
	Version string         `json:"version"`
}

// putConfig сохраняет конфиг и перегенерирует скрипт. Если файл на диске
// изменился с момента загрузки (правка из TUI или вручную) — 409. Конфиг с
// проблемами не сохраняется — 422 со списком проблем.
func (s *Server) putConfig(w http.ResponseWriter, r *http.Request) {
	var req putRequest
	if !readJSON(w, r, &req) {
		return
	}
	if req.Config == nil {
		writeError(w, http.StatusBadRequest, "нет поля config")
		return
	}
	normalize(req.Config)

	s.mu.Lock()
	defer s.mu.Unlock()

	cur, err := diskVersion()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if cur != req.Version {
		writeError(w, http.StatusConflict, "конфиг изменён на диске после загрузки")
		return
	}
	if ps := validation.ValidateConfig(req.Config); len(ps) > 0 {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]any{
			"error":    "в конфиге есть ошибки — исправьте их перед сохранением",
			"problems": toProblems(ps),
		})
		return
	}
	if err := storage.SaveConfig(req.Config); err != nil {
		writeError(w, http.StatusInternalServerError, "ошибка сохранения: "+err.Error())
		return
	}
	ver, _ := diskVersion()
	resp := s.response(req.Config, ver)
	if err := compiler.For(req.Config.Shell).Generate(req.Config); err != nil {
		// Конфиг уже сохранён, а прежний скрипт остался на месте: Generate
		// пишет атомарно и только после проверки синтаксиса.
		resp.GenerateError = err.Error()
	}
	writeJSON(w, http.StatusOK, resp)
}

// normalize приводит конфиг из JSON к виду, который даёт LoadConfig.
func normalize(cfg *models.Config) {
	if cfg.Variables == nil {
		cfg.Variables = map[string]string{}
	}
}

func (s *Server) preview(w http.ResponseWriter, r *http.Request) {
	var cfg models.Config
	if !readJSON(w, r, &cfg) {
		return
	}
	normalize(&cfg)
	writeJSON(w, http.StatusOK, map[string]any{
		"script":   compiler.For(cfg.Shell).Render(&cfg),
		"problems": toProblems(validation.ValidateConfig(&cfg)),
	})
}

type checkRequest struct {
	Kind string `json:"kind"` // "run" или "condition"
	Code string `json:"code"`
}

// check — статическая проверка синтаксиса (`bash -n`), код не выполняется.
func (s *Server) check(w http.ResponseWriter, r *http.Request) {
	var req checkRequest
	if !readJSON(w, r, &req) {
		return
	}
	var err error
	switch req.Kind {
	case "run":
		err = validation.CheckSyntax(req.Code)
	case "condition":
		err = validation.CheckCondition(req.Code)
	default:
		writeError(w, http.StatusBadRequest, "kind: ожидается run или condition")
		return
	}
	writeJSON(w, http.StatusOK, checkResult(err, ""))
}

type nameRequest struct {
	Kind  string `json:"kind"` // prefix, command, flag, variable
	Name  string `json:"name"`
	Shell string `json:"shell"`
}

func (s *Server) validateName(w http.ResponseWriter, r *http.Request) {
	var req nameRequest
	if !readJSON(w, r, &req) {
		return
	}
	var err error
	warn := ""
	switch req.Kind {
	case "prefix":
		err = validation.ValidateIdentifier(req.Name)
		if err == nil {
			warn = prefixCollision(req.Name, req.Shell)
		}
	case "variable":
		err = validation.ValidateIdentifier(req.Name)
	case "command":
		err = validation.ValidateCommandName(req.Name)
	case "flag":
		err = validation.ValidateFlagName(req.Name)
	default:
		writeError(w, http.StatusBadRequest, "kind: ожидается prefix, command, flag или variable")
		return
	}
	writeJSON(w, http.StatusOK, checkResult(err, warn))
}

// prefixCollision предупреждает, если префикс перекроет системную команду или
// alias из rc-файла оболочки (как в TUI).
func prefixCollision(name, shell string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	if validation.CheckCollision(name) {
		return fmt.Sprintf("'%s' уже есть в системе — префикс перекроет эту команду", name)
	}
	if shell == "" {
		shell = "zsh"
	}
	rc := importer.RCPath(shell)
	if list, err := importer.LoadAliasesFrom(rc); err == nil {
		for _, a := range list {
			if a.Name == name {
				return fmt.Sprintf("в %s есть alias '%s' — при вызове сработает он, а не префикс", rc, name)
			}
		}
	}
	return ""
}

func checkResult(err error, warn string) map[string]any {
	res := map[string]any{"ok": err == nil}
	if err != nil {
		res["error"] = err.Error()
	}
	if warn != "" {
		res["warning"] = warn
	}
	return res
}

func readJSON(w http.ResponseWriter, r *http.Request, v any) bool {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		writeError(w, http.StatusBadRequest, "некорректный JSON: "+err.Error())
		return false
	}
	return true
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
