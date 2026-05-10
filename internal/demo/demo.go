package demo

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// SetupProject creates a temporary directory with a realistic Go project
// and git history for exploring grut without needing your own repository.
// Returns the project path and a cleanup function that removes the
// temporary directory.
func SetupProject() (string, func(), error) {
	dir, err := os.MkdirTemp("", "grut-demo-*")
	if err != nil {
		return "", nil, fmt.Errorf("create temp dir: %w", err)
	}

	cleanup := func() { _ = os.RemoveAll(dir) }

	if err := populateDemoProject(dir); err != nil {
		cleanup()
		return "", nil, fmt.Errorf("populate demo: %w", err)
	}

	return dir, cleanup, nil
}

func populateDemoProject(dir string) error {
	// Create directory structure.
	dirs := []string{
		"src/api", "src/api/v2/handlers/admin", "src/api/v2/middleware",
		"src/auth", "src/models", "src/middleware", "src/utils",
		"tests", "docs", "configs", "scripts",
		".github/workflows", ".grut/extensions",
	}
	for _, d := range dirs {
		if err := os.MkdirAll(filepath.Join(dir, d), 0o755); err != nil {
			return err
		}
	}

	// Write project files.
	files := map[string]string{
		"go.mod":                     goMod,
		"main.go":                    mainGo,
		"README.md":                  readmeMD,
		"src/api/handlers.go":        handlersGo,
		"src/auth/jwt.go":            jwtGo,
		"src/models/user.go":         userGo,
		"src/middleware/chain.go":    chainGo,
		"src/utils/helpers.go":       helpersGo,
		"tests/api_test.go":          apiTestGo,
		"configs/app.yaml":           appYAML,
		".github/workflows/ci.yml":   ciYAML,
		".grut/extensions/hello.lua": grutExtensionLua,
		".grut/config.toml":          grutConfigTOML,
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			return err
		}
	}

	// Build git history.
	return buildGitHistory(dir)
}

func buildGitHistory(dir string) error {
	runAs := func(name, email string, args ...string) error {
		cmd := exec.CommandContext(context.Background(), "git", args...)
		cmd.Dir = dir
		cmd.Env = append(
			os.Environ(),
			"GIT_AUTHOR_NAME="+name,
			"GIT_AUTHOR_EMAIL="+email,
			"GIT_COMMITTER_NAME="+name,
			"GIT_COMMITTER_EMAIL="+email,
		)
		return cmd.Run()
	}
	alice := func(args ...string) error { return runAs("Alice Chen", "alice@example.com", args...) }
	bob := func(args ...string) error { return runAs("Bob Kumar", "bob@example.com", args...) }
	carol := func(args ...string) error { return runAs("Carol Martinez", "carol@example.com", args...) }
	dev := func(args ...string) error { return runAs("Developer", "dev@example.com", args...) }

	write := func(name, content string) error {
		p := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			return err
		}
		return os.WriteFile(p, []byte(content), 0o644)
	}

	// -- Commit 1: Initial project scaffold (Developer) --
	if err := dev("init", "-b", "main"); err != nil {
		return fmt.Errorf("git init: %w", err)
	}
	if err := dev("add", "."); err != nil {
		return err
	}
	if err := dev("commit", "-m", "Initial project scaffold"); err != nil {
		return err
	}

	// -- Commit 2: Add database migrations (Alice) --
	_ = write("src/models/migrations.go", migrationsGo)
	_ = alice("add", ".")
	_ = alice("commit", "-m", "Add database migrations")

	// -- Commit 3: Add WebSocket handler (Bob) --
	_ = write("src/api/websocket.go", websocketGo)
	_ = bob("add", ".")
	_ = bob("commit", "-m", "Add WebSocket handler")

	// -- Commit 4: Add rate limiter middleware (Carol) --
	_ = write("src/middleware/ratelimit.go", ratelimitGo)
	_ = carol("add", ".")
	_ = carol("commit", "-m", "Add rate limiter middleware")

	// -- Commit 5: Add API documentation (Developer) --
	_ = write("docs/api.md", apiDoc)
	_ = dev("add", ".")
	_ = dev("commit", "-m", "Add API documentation")

	// -- Commit 6: Add Python API routes (Alice) --
	_ = write("src/api/routes.py", routesPy)
	_ = alice("add", ".")
	_ = alice("commit", "-m", "Add Python API routes")

	// -- Commit 7: Add TypeScript client (Bob) --
	_ = write("src/api/client.ts", clientTS)
	_ = bob("add", ".")
	_ = bob("commit", "-m", "Add TypeScript API client")

	// -- Commit 8: Add Docker configuration (Carol) --
	_ = write("Dockerfile", dockerfileContent)
	_ = write("docker-compose.yml", dockerComposeYML)
	_ = carol("add", ".")
	_ = carol("commit", "-m", "Add Docker configuration")

	// -- Commit 9: Add SQL schema and validation helpers (Alice) --
	_ = write("src/models/schema.sql", schemaSql)
	_ = appendFile(filepath.Join(dir, "src/utils/helpers.go"), helpersAppendAlice)
	_ = alice("add", ".")
	_ = alice("commit", "-m", "Add SQL schema and validation helpers")

	// -- Commit 10: Add shell scripts (Developer) --
	_ = write("scripts/setup.sh", setupSH)
	_ = write("scripts/deploy.ps1", deployPS1)
	_ = dev("add", ".")
	_ = dev("commit", "-m", "Add shell scripts")

	// -- Commit 11: Add admin dashboard handlers (Bob) --
	_ = write("src/api/v2/handlers/admin/dashboard.go", dashboardGo)
	_ = write("src/api/v2/handlers/admin/reports.go", reportsGo)
	_ = appendFile(filepath.Join(dir, "src/utils/helpers.go"), helpersAppendBob)
	_ = bob("add", ".")
	_ = bob("commit", "-m", "Add admin dashboard handlers")

	// -- Commit 12: Add v2 auth middleware (Carol) --
	_ = write("src/api/v2/middleware/auth.go", authMiddlewareV2Go)
	_ = appendFile(filepath.Join(dir, "src/utils/helpers.go"), helpersAppendCarol)
	_ = carol("add", ".")
	_ = carol("commit", "-m", "Add v2 auth middleware")

	// -- Commit 13: Update README with new features (Alice) --
	_ = write("README.md", readmeMDV2)
	_ = alice("add", ".")
	_ = alice("commit", "-m", "Update README with new features")

	// -- Commit 14: Add Makefile build targets (Developer) --
	_ = write("Makefile", makefileContent)
	_ = dev("add", ".")
	_ = dev("commit", "-m", "Add Makefile build targets")

	// -- Commit 15: Add environment config template (Bob) --
	_ = write(".env.example", envExample)
	_ = bob("add", ".")
	_ = bob("commit", "-m", "Add environment config template")

	// ---- Create fix branch BEFORE the conflict commit ----
	_ = dev("checkout", "-b", "fix/rate-limit-bypass")
	_ = write("src/middleware/ratelimit.go", ratelimitGoFix)
	_ = dev("add", ".")
	_ = dev("commit", "-m", "Fix rate limit edge case")

	// ---- Back to main: commit 16 creates the conflict ----
	_ = dev("checkout", "main")
	_ = write("src/middleware/ratelimit.go", ratelimitGoV2)
	_ = carol("add", ".")
	_ = carol("commit", "-m", "Refactor rate limiter to use in-place cleanup")

	// ---- Tags ----
	_ = dev("tag", "-a", "v0.1.0", "-m", "Release v0.1.0", "HEAD~15")
	_ = dev("tag", "-a", "v0.2.0", "-m", "Release v0.2.0", "HEAD~8")
	_ = dev("tag", "-a", "v0.3.0", "-m", "Release v0.3.0", "HEAD~1")

	// ---- Feature branches ----
	// develop: 1 commit ahead of main.
	_ = alice("checkout", "-b", "develop")
	_ = write("docs/develop.md", developDoc)
	_ = alice("add", ".")
	_ = alice("commit", "-m", "Set up develop branch")

	// feature/websocket-support: 2 commits ahead of main.
	_ = bob("checkout", "-b", "feature/websocket-support", "main")
	_ = appendFile(filepath.Join(dir, "src/api/websocket.go"), wsBroadcastAppend)
	_ = bob("add", ".")
	_ = bob("commit", "-m", "Add WebSocket broadcast support")
	_ = write("tests/websocket_test.go", wsTestGo)
	_ = bob("add", ".")
	_ = bob("commit", "-m", "Add WebSocket tests")

	// feature/api-v2: 2 commits ahead of main.
	_ = carol("checkout", "-b", "feature/api-v2", "main")
	_ = write("src/api/v2/handlers/endpoints.go", v2EndpointsGo)
	_ = carol("add", ".")
	_ = carol("commit", "-m", "Add v2 API endpoints")
	_ = write("docs/api-v2.md", apiV2Doc)
	_ = carol("add", ".")
	_ = carol("commit", "-m", "Add v2 API documentation")

	// ---- Return to main ----
	_ = dev("checkout", "main")

	// ---- Stash entries (LIFO: created in reverse display order) ----
	// Stash 3 (bottom of stash list): API rate limit config.
	_ = appendFile(filepath.Join(dir, "configs/app.yaml"),
		"\nrate_limit:\n  requests_per_minute: 200\n  burst: 50\n")
	_ = dev("stash", "push", "-m", "WIP: API rate limit config", "--", "configs/app.yaml")

	// Stash 2: user validation logic.
	_ = appendFile(filepath.Join(dir, "src/models/user.go"),
		"\n// ValidateUser checks required fields.\nfunc (u *User) Validate() error { return nil }\n")
	_ = dev("stash", "push", "-m", "WIP: user validation logic", "--", "src/models/user.go")

	// Stash 1 (top of stash list): health check v2.
	_ = write("src/api/health.go", healthGo)
	_ = dev("add", "src/api/health.go")
	_ = dev("stash", "push", "-m", "WIP: health check v2")

	// ---- Staged changes (2 files) ----
	_ = appendFile(filepath.Join(dir, "src/api/handlers.go"), "\n// TODO: Add pagination to ListUsers\n")
	_ = write("src/api/metrics.go", metricsGo)
	_ = dev("add", "src/api/handlers.go", "src/api/metrics.go")

	// ---- Unstaged changes (multi-hunk) ----
	// Modify main.go at top and bottom to produce two diff hunks.
	mainContent, _ := os.ReadFile(filepath.Join(dir, "main.go"))
	modified := strings.Replace(string(mainContent),
		"package main", "package main\n\n// TODO: Add graceful shutdown support", 1)
	modified += "\n// TODO: Add metrics collection\n"
	_ = os.WriteFile(filepath.Join(dir, "main.go"), []byte(modified), 0o644)

	// Modify chain.go at top and bottom for two hunks.
	chainContent, _ := os.ReadFile(filepath.Join(dir, "src/middleware/chain.go"))
	modChain := strings.Replace(string(chainContent),
		"package middleware", "package middleware\n\n// TODO: Add request ID middleware", 1)
	modChain += "\n// TODO: Add timeout middleware\n"
	_ = os.WriteFile(filepath.Join(dir, "src/middleware/chain.go"), []byte(modChain), 0o644)

	// Also an unstaged change in app.yaml.
	_ = appendFile(filepath.Join(dir, "configs/app.yaml"), "\n# TODO: Add Redis cache config\n")

	// ---- Untracked files ----
	_ = write("TODO.md", todoMD)
	_ = write("src/api/ping.go", pingGo)
	_ = write("src/api/cache.go", cacheGo)
	_ = write(".env.local", envLocal)

	// ---- Worktree ----
	wtDir := filepath.Join(dir, ".worktrees", "feature-api-v2")
	_ = os.MkdirAll(filepath.Dir(wtDir), 0o755)
	_ = dev("worktree", "add", wtDir, "feature/api-v2")

	return nil
}

func appendFile(path, content string) error {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	_, err = f.WriteString(content)
	return err
}

// ── File contents ──────────────────────────────────────────────────────

const goMod = `module github.com/myorg/myapp

go 1.23

require (
	github.com/gorilla/mux v1.8.1
	github.com/lib/pq v1.10.9
	golang.org/x/crypto v0.28.0
)
`

const mainGo = `package main

import (
	"log"
	"net/http"
	"os"
	"github.com/myorg/myapp/src/api"
	"github.com/myorg/myapp/src/middleware"
)

func main() {
	// Warn loudly if JWT_SECRET is still set to a well-known development default.
	if s := os.Getenv("JWT_SECRET"); s == "change-me" || s == "dev-secret-do-not-use" || s == "" {
		log.Println("WARNING: JWT_SECRET is unset or using a default value — set a strong secret before deploying")
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	router := api.NewRouter()
	handler := middleware.Chain(router,
		middleware.Logger,
		middleware.Recovery,
		middleware.CORS,
		middleware.RateLimit,
	)

	log.Printf("Server starting on :%s", port)
	if err := http.ListenAndServe(":"+port, handler); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}
`

const readmeMD = `# MyApp

A high-performance REST API service built with Go.

## Features

- RESTful API with versioned endpoints
- JWT authentication with refresh tokens
- PostgreSQL database with migrations
- Rate limiting and request throttling
- Structured logging with correlation IDs
- Health checks and metrics endpoints
- WebSocket support for real-time updates

## Quick Start

` + "`" + "`" + "`" + `bash
go run main.go
` + "`" + "`" + "`" + `

## Configuration

Copy ` + "`" + `configs/app.yaml` + "`" + ` and set your environment variables:

` + "`" + "`" + "`" + `yaml
server:
  port: 8080
  read_timeout: 30s
database:
  host: localhost
  port: 5432
  name: myapp
` + "`" + "`" + "`" + `

## API Endpoints

| Method | Path              | Description          |
|--------|-------------------|----------------------|
| GET    | /api/v1/users     | List users           |
| POST   | /api/v1/users     | Create user          |
| GET    | /api/v1/users/:id | Get user by ID       |
| PUT    | /api/v1/users/:id | Update user          |
| DELETE | /api/v1/users/:id | Delete user          |
| GET    | /health           | Health check         |
| GET    | /metrics          | Prometheus metrics   |

## License

MIT
`

const handlersGo = `package api

import (
	"encoding/json"
	"net/http"
	"github.com/gorilla/mux"
)

func NewRouter() *mux.Router {
	r := mux.NewRouter()
	r.HandleFunc("/api/v1/users", ListUsers).Methods("GET")
	r.HandleFunc("/api/v1/users", CreateUser).Methods("POST")
	r.HandleFunc("/api/v1/users/{id}", GetUser).Methods("GET")
	r.HandleFunc("/api/v1/users/{id}", UpdateUser).Methods("PUT")
	r.HandleFunc("/api/v1/users/{id}", DeleteUser).Methods("DELETE")
	r.HandleFunc("/health", HealthCheck).Methods("GET")
	return r
}

func ListUsers(w http.ResponseWriter, r *http.Request) {
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func CreateUser(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusCreated)
}

func GetUser(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	json.NewEncoder(w).Encode(map[string]string{"id": vars["id"]})
}

func UpdateUser(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
}

func DeleteUser(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNoContent)
}

func HealthCheck(w http.ResponseWriter, r *http.Request) {
	json.NewEncoder(w).Encode(map[string]string{"status": "healthy"})
}
`

const jwtGo = `package auth

import (
	"crypto/rand"
	"encoding/hex"
	"time"
	"golang.org/x/crypto/bcrypt"
)

type TokenPair struct {
	AccessToken  string
	RefreshToken string
	ExpiresAt    time.Time
}

func HashPassword(password string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), 14)
	return string(bytes), err
}

func CheckPassword(password, hash string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	return err == nil
}

func GenerateToken() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}
`

const userGo = `package models

import "time"

type User struct {
	ID        int64     ` + "`" + `json:"id"` + "`" + `
	Email     string    ` + "`" + `json:"email"` + "`" + `
	Name      string    ` + "`" + `json:"name"` + "`" + `
	Role      string    ` + "`" + `json:"role"` + "`" + `
	CreatedAt time.Time ` + "`" + `json:"created_at"` + "`" + `
	UpdatedAt time.Time ` + "`" + `json:"updated_at"` + "`" + `
}

type Session struct {
	ID        string    ` + "`" + `json:"id"` + "`" + `
	UserID    int64     ` + "`" + `json:"user_id"` + "`" + `
	Token     string    ` + "`" + `json:"token"` + "`" + `
	ExpiresAt time.Time ` + "`" + `json:"expires_at"` + "`" + `
}
`

const chainGo = `package middleware

import (
	"log"
	"net/http"
	"time"
)

func Chain(h http.Handler, middlewares ...func(http.Handler) http.Handler) http.Handler {
	for i := len(middlewares) - 1; i >= 0; i-- {
		h = middlewares[i](h)
	}
	return h
}

func Logger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		log.Printf("%s %s %v", r.Method, r.URL.Path, time.Since(start))
	})
}

func Recovery(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				log.Printf("panic recovered: %v", err)
				http.Error(w, "Internal Server Error", 500)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func CORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func RateLimit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r)
	})
}
`

const helpersGo = `package utils

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
	"unicode"
)

// GenerateID returns a cryptographically random hex-encoded identifier.
func GenerateID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// FormatDuration returns a human-readable duration string.
func FormatDuration(d time.Duration) string {
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	return fmt.Sprintf("%.1fs", d.Seconds())
}

// Ensure imports are used.
var _ = strings.TrimSpace
var _ = unicode.IsLetter
`

const apiTestGo = `package tests

import "testing"

func TestHealthCheck(t *testing.T) {
	t.Run("returns 200", func(t *testing.T) {
		// Test health endpoint
	})
}

func TestUserAPI(t *testing.T) {
	t.Run("list users returns array", func(t *testing.T) {
		// Test list endpoint
	})
	t.Run("create user returns 201", func(t *testing.T) {
		// Test create endpoint
	})
}
`

const appYAML = `server:
  port: 8080
  read_timeout: 30s
  write_timeout: 30s
  idle_timeout: 120s

database:
  host: localhost
  port: 5432
  name: myapp
  user: postgres
  ssl_mode: disable
  max_connections: 25
  max_idle: 5

logging:
  level: info
  format: json

auth:
  jwt_secret: change-me-in-production
  token_ttl: 24h
  refresh_ttl: 168h
`

const ciYAML = `name: CI

on:
  push:
    branches: [main]
  pull_request:
    branches: [main]

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: '1.23'
      - run: go test ./...
      - run: go vet ./...
`

const migrationsGo = `package models

import "time"

type Migration struct {
	Version   int
	Name      string
	AppliedAt time.Time
}

var Migrations = []Migration{
	{Version: 1, Name: "create_users_table"},
	{Version: 2, Name: "add_sessions_table"},
	{Version: 3, Name: "add_user_roles"},
}
`

const websocketGo = `package api

import (
	"encoding/json"
	"net/http"
)

type WebSocketHandler struct {
	clients map[string]chan []byte
}

func NewWebSocketHandler() *WebSocketHandler {
	return &WebSocketHandler{
		clients: make(map[string]chan []byte),
	}
}

func (ws *WebSocketHandler) HandleConnect(w http.ResponseWriter, r *http.Request) {
	json.NewEncoder(w).Encode(map[string]string{"status": "connected"})
}
`

const ratelimitGo = `package middleware

import (
	"net/http"
	"sync"
	"time"
)

type RateLimiter struct {
	mu       sync.Mutex
	requests map[string][]time.Time
	limit    int
	window   time.Duration
}

func NewRateLimiter(limit int, window time.Duration) *RateLimiter {
	return &RateLimiter{
		requests: make(map[string][]time.Time),
		limit:    limit,
		window:   window,
	}
}

func (rl *RateLimiter) Allow(key string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-rl.window)

	var valid []time.Time
	for _, t := range rl.requests[key] {
		if t.After(cutoff) {
			valid = append(valid, t)
		}
	}

	if len(valid) >= rl.limit {
		return false
	}

	rl.requests[key] = append(valid, now)
	return true
}

func (rl *RateLimiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !rl.Allow(r.RemoteAddr) {
			http.Error(w, "Too Many Requests", http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})
}
`

const apiDoc = `# API Documentation

## Authentication

All API endpoints except /health require a Bearer token.

## Rate Limiting

Default: 100 requests per minute per IP address.

## Error Responses

All errors follow RFC 7807 Problem Details format.
`

const healthGo = `package api

func HealthCheckV2() {}
`

const pingGo = `package api

func PingHandler() {}
`

const routesPy = `"""Flask-style API routes for the Python micro-service."""

from flask import Flask, jsonify, request

app = Flask(__name__)

users_db = []


@app.route("/api/v1/users", methods=["GET"])
def list_users():
    page = request.args.get("page", 1, type=int)
    per_page = request.args.get("per_page", 20, type=int)
    start = (page - 1) * per_page
    return jsonify({"users": users_db[start : start + per_page], "total": len(users_db)})


@app.route("/api/v1/users", methods=["POST"])
def create_user():
    data = request.get_json(force=True)
    if not data.get("email"):
        return jsonify({"error": "email is required"}), 400
    users_db.append(data)
    return jsonify(data), 201


@app.route("/api/v1/users/<int:user_id>", methods=["GET"])
def get_user(user_id):
    if user_id < 0 or user_id >= len(users_db):
        return jsonify({"error": "not found"}), 404
    return jsonify(users_db[user_id])


if __name__ == "__main__":
    app.run(host="0.0.0.0", port=5000, debug=True)
`

const clientTS = `/**
 * TypeScript API client for MyApp.
 */

interface User {
  id: number;
  email: string;
  name: string;
  role: string;
  created_at: string;
  updated_at: string;
}

interface ApiResponse<T> {
  data: T;
  error?: string;
}

class ApiClient {
  private baseUrl: string;
  private token: string | null = null;

  constructor(baseUrl: string) {
    this.baseUrl = baseUrl;
  }

  setToken(token: string): void {
    this.token = token;
  }

  private async request<T>(path: string, options: RequestInit = {}): Promise<ApiResponse<T>> {
    const headers: Record<string, string> = { "Content-Type": "application/json" };
    if (this.token) {
      headers["Authorization"] = "Bearer " + this.token;
    }
    const resp = await fetch(this.baseUrl + path, { ...options, headers });
    if (!resp.ok) {
      return { data: null as unknown as T, error: resp.statusText };
    }
    return { data: await resp.json() };
  }

  listUsers(): Promise<ApiResponse<User[]>> {
    return this.request<User[]>("/api/v1/users");
  }

  getUser(id: number): Promise<ApiResponse<User>> {
    return this.request<User>("/api/v1/users/" + id);
  }
}

export { ApiClient, User, ApiResponse };
`

const setupSH = `#!/usr/bin/env bash
set -euo pipefail

echo "==> Installing dependencies..."
go mod download

echo "==> Running database migrations..."
go run ./cmd/migrate up

echo "==> Generating API documentation..."
go generate ./...

echo "==> Building application..."
go build -o bin/myapp ./main.go

echo "==> Running tests..."
go test -race -cover ./...

echo "==> Setup complete."
`

const deployPS1 = `#Requires -Version 7.0
param(
    [string]$Environment = "staging",
    [string]$Registry = "ghcr.io/myorg/myapp",
    [string]$Tag = "latest"
)

$ErrorActionPreference = "Stop"

Write-Host "==> Building Docker image..." -ForegroundColor Cyan
docker build -t "${Registry}:${Tag}" .

Write-Host "==> Pushing to registry..." -ForegroundColor Cyan
docker push "${Registry}:${Tag}"

Write-Host "==> Deploying to $Environment..." -ForegroundColor Cyan
kubectl set image "deployment/myapp" "myapp=${Registry}:${Tag}" -n $Environment

Write-Host "==> Waiting for rollout..." -ForegroundColor Cyan
kubectl rollout status "deployment/myapp" -n $Environment --timeout=300s

Write-Host "==> Deployment complete." -ForegroundColor Green
`

const dockerfileContent = `# syntax=docker/dockerfile:1
FROM golang:1.23-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-s -w" \
    -o /usr/local/bin/myapp \
    ./main.go

FROM alpine:3.20

RUN apk add --no-cache ca-certificates tzdata

COPY --from=builder /usr/local/bin/myapp /usr/local/bin/myapp

EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=3s --retries=3 \
    CMD wget -qO- http://localhost:8080/health || exit 1

USER nobody
ENTRYPOINT ["myapp"]
`

const makefileContent = `.PHONY: build test lint run clean docker

build:
	go build -o bin/myapp ./main.go

test:
	go test -race -cover ./...

lint:
	golangci-lint run ./...

run:
	go run main.go

clean:
	rm -rf bin/ dist/

docker:
	docker compose up --build -d
`

const dockerComposeYML = `version: "3.9"

services:
  app:
    build: .
    ports:
      - "8080:8080"
    environment:
      - PORT=8080
      - DB_HOST=postgres
      - DB_PORT=5432
      - DB_NAME=myapp
    depends_on:
      postgres:
        condition: service_healthy

  postgres:
    image: postgres:16-alpine
    environment:
      POSTGRES_DB: myapp
      POSTGRES_USER: postgres
      POSTGRES_PASSWORD: devpass
    ports:
      - "5432:5432"
    volumes:
      - pgdata:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U postgres"]
      interval: 5s
      timeout: 3s
      retries: 5

volumes:
  pgdata:
`

const envExample = `PORT=8080
DB_HOST=localhost
DB_PORT=5432
DB_NAME=myapp
DB_USER=postgres
DB_PASSWORD=
JWT_SECRET=change-me
LOG_LEVEL=info
CORS_ORIGINS=http://localhost:3000
REDIS_URL=redis://localhost:6379
`

const schemaSql = `-- Database schema for MyApp
-- PostgreSQL 16+

CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

CREATE TABLE users (
    id          BIGSERIAL PRIMARY KEY,
    email       VARCHAR(255) NOT NULL UNIQUE,
    name        VARCHAR(255) NOT NULL,
    role        VARCHAR(50)  NOT NULL DEFAULT 'user',
    password    VARCHAR(255) NOT NULL,
    created_at  TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ  NOT NULL DEFAULT now()
);

CREATE TABLE sessions (
    id          UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id     BIGINT       NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token       VARCHAR(512) NOT NULL UNIQUE,
    expires_at  TIMESTAMPTZ  NOT NULL,
    created_at  TIMESTAMPTZ  NOT NULL DEFAULT now()
);

CREATE INDEX idx_sessions_user    ON sessions(user_id);
CREATE INDEX idx_sessions_token   ON sessions(token);
CREATE INDEX idx_sessions_expires ON sessions(expires_at);

CREATE TABLE audit_log (
    id         BIGSERIAL PRIMARY KEY,
    user_id    BIGINT       REFERENCES users(id),
    action     VARCHAR(100) NOT NULL,
    resource   VARCHAR(255),
    detail     JSONB,
    created_at TIMESTAMPTZ  NOT NULL DEFAULT now()
);

CREATE INDEX idx_audit_user ON audit_log(user_id);
`

const dashboardGo = `package admin

import (
	"encoding/json"
	"net/http"
)

type DashboardStats struct {
	TotalUsers   int64 ` + "`" + `json:"total_users"` + "`" + `
	ActiveToday  int64 ` + "`" + `json:"active_today"` + "`" + `
	RequestCount int64 ` + "`" + `json:"request_count"` + "`" + `
}

func GetDashboard(w http.ResponseWriter, r *http.Request) {
	stats := DashboardStats{
		TotalUsers:   1024,
		ActiveToday:  87,
		RequestCount: 54321,
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}
`

const reportsGo = `package admin

import (
	"encoding/json"
	"net/http"
)

type Report struct {
	Name      string ` + "`" + `json:"name"` + "`" + `
	Generated string ` + "`" + `json:"generated"` + "`" + `
}

func ListReports(w http.ResponseWriter, r *http.Request) {
	reports := []Report{{Name: "daily-active-users", Generated: "2024-01-15"}}
	json.NewEncoder(w).Encode(reports)
}
`

const authMiddlewareV2Go = `package middleware

import (
	"context"
	"net/http"
	"strings"
)

type contextKey string

const UserIDKey contextKey = "user_id"

func AuthRequired(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if !strings.HasPrefix(auth, "Bearer ") {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		token := strings.TrimPrefix(auth, "Bearer ")
		if token == "" {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		ctx := context.WithValue(r.Context(), UserIDKey, token)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
`

const helpersAppendAlice = `
// ValidateEmail performs basic email format validation.
func ValidateEmail(email string) bool {
	parts := strings.SplitN(email, "@", 2)
	if len(parts) != 2 {
		return false
	}
	return len(parts[0]) > 0 && strings.Contains(parts[1], ".")
}

// Truncate shortens s to maxLen and appends an ellipsis if truncated.
func Truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
`

const helpersAppendBob = `
// FormatBytes returns a human-readable byte size string.
func FormatBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

// SlugFromTitle converts a title string into a URL-friendly slug.
func SlugFromTitle(title string) string {
	slug := strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return unicode.ToLower(r)
		}
		if r == ' ' || r == '-' {
			return '-'
		}
		return -1
	}, title)
	return strings.Trim(slug, "-")
}
`

const helpersAppendCarol = `
// RetryWithBackoff attempts fn up to maxRetries times with exponential backoff.
func RetryWithBackoff(fn func() error, maxRetries int, baseDelay time.Duration) error {
	var err error
	for i := 0; i < maxRetries; i++ {
		if err = fn(); err == nil {
			return nil
		}
		time.Sleep(baseDelay * time.Duration(1<<uint(i)))
	}
	return fmt.Errorf("after %d retries: %w", maxRetries, err)
}

// Chunk splits a slice into groups of at most size elements.
func Chunk(items []string, size int) [][]string {
	var chunks [][]string
	for i := 0; i < len(items); i += size {
		end := i + size
		if end > len(items) {
			end = len(items)
		}
		chunks = append(chunks, items[i:end])
	}
	return chunks
}
`

const readmeMDV2 = `# MyApp

A high-performance REST API service built with Go.

## Features

- RESTful API with versioned endpoints (v1 and v2)
- JWT authentication with refresh tokens
- PostgreSQL database with migrations
- Rate limiting and request throttling
- Structured logging with correlation IDs
- Health checks and metrics endpoints
- WebSocket support for real-time updates
- Admin dashboard with analytics
- Multi-language client SDKs (Go, Python, TypeScript)
- Docker and docker-compose support
- Lua extension system via grut

## Quick Start

` + "`" + "`" + "`" + `bash
make build
./bin/myapp
` + "`" + "`" + "`" + `

Or with Docker:

` + "`" + "`" + "`" + `bash
docker compose up -d
` + "`" + "`" + "`" + `

## Configuration

Copy ` + "`" + `.env.example` + "`" + ` to ` + "`" + `.env` + "`" + ` and update the values:

` + "`" + "`" + "`" + `bash
cp .env.example .env
` + "`" + "`" + "`" + `

See ` + "`" + `configs/app.yaml` + "`" + ` for full configuration options.

## API Endpoints

| Method | Path                   | Description          |
|--------|------------------------|----------------------|
| GET    | /api/v1/users          | List users           |
| POST   | /api/v1/users          | Create user          |
| GET    | /api/v1/users/:id      | Get user by ID       |
| PUT    | /api/v1/users/:id      | Update user          |
| DELETE | /api/v1/users/:id      | Delete user          |
| GET    | /api/v2/admin/dashboard| Admin dashboard      |
| GET    | /api/v2/admin/reports  | List reports         |
| GET    | /health                | Health check         |
| GET    | /metrics               | Prometheus metrics   |
| WS     | /ws                    | WebSocket endpoint   |

## Development

` + "`" + "`" + "`" + `bash
make test    # Run tests
make lint    # Run linter
make run     # Run locally
` + "`" + "`" + "`" + `

## License

MIT
`

const grutExtensionLua = `-- hello.lua: Sample grut extension
-- Demonstrates the Lua extension system

local grut = require("grut")

grut.register_command("hello", {
    description = "Say hello from a Lua extension",
    execute = function()
        grut.notify("Hello from the Lua extension!")
    end
})

grut.register_hook("on_file_open", function(path)
    if path:match("%.go$") then
        grut.log("Opened Go file: " .. path)
    end
end)
`

const grutConfigTOML = `[ai]
provider = "openai"
model = "gpt-4o"
max_tokens = 4096

[ui]
theme = "tokyo-night"
icon_mode = "nerd"

[git]
auto_fetch = true
fetch_interval = "5m"
`

const ratelimitGoV2 = `package middleware

import (
	"net/http"
	"sync"
	"time"
)

type RateLimiter struct {
	mu       sync.Mutex
	requests map[string][]time.Time
	limit    int
	window   time.Duration
}

func NewRateLimiter(limit int, window time.Duration) *RateLimiter {
	return &RateLimiter{
		requests: make(map[string][]time.Time),
		limit:    limit,
		window:   window,
	}
}

func (rl *RateLimiter) Allow(key string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-rl.window)

	// Cleanup expired entries in place.
	entries := rl.requests[key]
	n := 0
	for _, t := range entries {
		if t.After(cutoff) {
			entries[n] = t
			n++
		}
	}
	entries = entries[:n]

	if len(entries) >= rl.limit {
		rl.requests[key] = entries
		return false
	}

	rl.requests[key] = append(entries, now)
	return true
}

func (rl *RateLimiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !rl.Allow(r.RemoteAddr) {
			http.Error(w, "Too Many Requests", http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})
}
`

const ratelimitGoFix = `package middleware

import (
	"net/http"
	"sync"
	"time"
)

type RateLimiter struct {
	mu       sync.Mutex
	requests map[string][]time.Time
	limit    int
	window   time.Duration
}

func NewRateLimiter(limit int, window time.Duration) *RateLimiter {
	return &RateLimiter{
		requests: make(map[string][]time.Time),
		limit:    limit,
		window:   window,
	}
}

func (rl *RateLimiter) Allow(key string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-rl.window)

	var valid []time.Time
	for _, t := range rl.requests[key] {
		if t.After(cutoff) {
			valid = append(valid, t)
		}
	}

	// Fix: use > instead of >= to allow exactly limit requests.
	if len(valid) > rl.limit {
		rl.requests[key] = valid
		return false
	}

	rl.requests[key] = append(valid, now)
	return true
}

func (rl *RateLimiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !rl.Allow(r.RemoteAddr) {
			http.Error(w, "Too Many Requests", http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})
}
`

const cacheGo = `package api

// CacheMiddleware provides in-memory response caching.
func CacheMiddleware() {}
`

const todoMD = `# TODO

- [ ] Add pagination to list endpoints
- [ ] Implement WebSocket reconnection logic
- [ ] Add Prometheus metrics exporter
- [ ] Write integration tests for auth flow
- [ ] Set up CI/CD pipeline for staging
`

const envLocal = `PORT=3000
DB_HOST=localhost
DB_PASSWORD=localdev
JWT_SECRET=dev-secret-do-not-use
LOG_LEVEL=debug
`

const metricsGo = `package api

// PrometheusMetrics exposes /metrics endpoint.
func PrometheusMetrics() {}
`

const wsBroadcastAppend = `
// Broadcast sends a message to all connected clients.
func (ws *WebSocketHandler) Broadcast(msg []byte) {
	for _, ch := range ws.clients {
		select {
		case ch <- msg:
		default:
		}
	}
}
`

const wsTestGo = `package tests

import "testing"

func TestWebSocketConnect(t *testing.T) {
	t.Run("establishes connection", func(t *testing.T) {
		// Test WebSocket connection handshake
	})
}

func TestWebSocketBroadcast(t *testing.T) {
	t.Run("delivers to all clients", func(t *testing.T) {
		// Test broadcast delivery
	})
}
`

const v2EndpointsGo = `package handlers

import (
	"encoding/json"
	"net/http"
)

type V2Response struct {
	Version string      ` + "`" + `json:"version"` + "`" + `
	Data    any ` + "`" + `json:"data"` + "`" + `
}

func V2ListUsers(w http.ResponseWriter, r *http.Request) {
	resp := V2Response{Version: "2.0", Data: []string{}}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func V2GetUser(w http.ResponseWriter, r *http.Request) {
	resp := V2Response{Version: "2.0", Data: nil}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}
`

const apiV2Doc = `# API v2 Documentation

## Overview

API v2 introduces structured response envelopes and pagination.

## Response Format

All v2 responses follow the envelope format:

` + "`" + "`" + "`" + `json
{
  "version": "2.0",
  "data": {},
  "meta": {
    "page": 1,
    "total": 42
  }
}
` + "`" + "`" + "`" + `

## Breaking Changes from v1

- Response envelope wraps all data
- Pagination is required for list endpoints
- Authentication uses API keys instead of Bearer tokens
`

const developDoc = `# Develop Branch

Integration branch for upcoming release.

## Merge Policy

- All feature branches merge here first
- CI must pass before merging to main
`
