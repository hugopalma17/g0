package main

import (
	"bufio"
	"context"
	"crypto/rand"
	"database/sql"
	"embed"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"time"

	"regexp"
	"strings"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/creack/pty"
	"github.com/gomarkdown/markdown"
	"github.com/gorilla/sessions"
	"github.com/gorilla/websocket"
	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
	"golang.org/x/oauth2"
	"gopkg.in/yaml.v3"
	_ "modernc.org/sqlite"
)

// ============================================================================
// CONFIGURATION
// ============================================================================

// SiteConfig holds site-level configuration
type SiteConfig struct {
	Name        string `yaml:"name"`
	URL         string `yaml:"url"`
	Tagline     string `yaml:"tagline"`
	Description string `yaml:"description"`
	Author      string `yaml:"author"`
}

// DatabaseConfig holds database configuration
type DatabaseConfig struct {
	Driver   string `yaml:"driver"` // "sqlite" or "postgres"
	Path     string `yaml:"path"`   // SQLite file path
	Host     string `yaml:"host"`
	Port     string `yaml:"port"`
	User     string `yaml:"user"`
	Password string `yaml:"password"`
	DBName   string `yaml:"dbname"`
}

// AuthConfig holds OAuth configuration
type AuthConfig struct {
	Provider      string   `yaml:"provider"`
	ClientID      string   `yaml:"client_id"`
	ClientSecret  string   `yaml:"client_secret"`
	RedirectURL   string   `yaml:"redirect_url"`
	AllowedEmails []string `yaml:"allowed_emails"`
	SessionKey    string   `yaml:"session_key"`
	SessionKeyOld string   `yaml:"session_key_old,omitempty"`
}

// ServerConfig holds server configuration
type ServerConfig struct {
	Port      string `yaml:"port"`
	Host      string `yaml:"host"`
	AdminPath string `yaml:"admin_path"` // Custom admin panel URL path
}

// Config is the main configuration structure
type Config struct {
	Site     SiteConfig     `yaml:"site"`
	Database DatabaseConfig `yaml:"database"`
	Auth     AuthConfig     `yaml:"auth"`
	Server   ServerConfig   `yaml:"server"`
}

// Global config instance
var config Config

// ============================================================================
// EMBEDDED FILES
// ============================================================================

//go:embed templates static
var embeddedFiles embed.FS

var (
	staticFS     fs.FS
	db           *sql.DB
	store        *sessions.CookieStore
	oauthConfig  *oauth2.Config
	oidcProvider *oidc.Provider
	verifier     *oidc.IDTokenVerifier
)

func init() {
	var err error
	staticFS, err = fs.Sub(embeddedFiles, "static")
	if err != nil {
		log.Fatalf("Failed to create static sub-filesystem: %v", err)
	}
}

// ============================================================================
// FIRST RUN DETECTION & SETUP WIZARD
// ============================================================================

const dataDir = "./data"
const configPath = "./data/config.yaml"

func isFirstRun() bool {
	// Ensure data directory exists
	os.MkdirAll(dataDir, 0755)
	_, err := os.Stat(configPath)
	return os.IsNotExist(err)
}

func loadConfig() error {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return err
	}
	if err := yaml.Unmarshal(data, &config); err != nil {
		return err
	}

	// Auto-generate session key if empty (user created config manually)
	if config.Auth.SessionKey == "" {
		config.Auth.SessionKey = generateSessionKey()
		log.Println("⚠️  Generated missing session key - saving to config")
		saveConfig(config)
	}

	// Default database to SQLite if not specified
	if config.Database.Driver == "" {
		config.Database.Driver = "sqlite"
	}
	if config.Database.Driver == "sqlite" && config.Database.Path == "" {
		config.Database.Path = "./data/g0.db"
	}

	// Default server settings
	if config.Server.Port == "" {
		config.Server.Port = "8080"
	}
	if config.Server.Host == "" {
		config.Server.Host = "0.0.0.0"
	}
	if config.Server.AdminPath == "" {
		config.Server.AdminPath = "/admin"
	}
	// Ensure admin path starts with /
	if !strings.HasPrefix(config.Server.AdminPath, "/") {
		config.Server.AdminPath = "/" + config.Server.AdminPath
	}

	return nil
}

func saveConfig(cfg Config) error {
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	return os.WriteFile(configPath, data, 0600)
}

func generateSessionKey() string {
	b := make([]byte, 32)
	rand.Read(b)
	return base64.StdEncoding.EncodeToString(b)
}

// setupWizardHandler serves the setup wizard page
func setupWizardHandler(w http.ResponseWriter, r *http.Request) {
	if !isFirstRun() {
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}

	html := `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Antigravity CMS - Setup</title>
    <style>
        * { box-sizing: border-box; margin: 0; padding: 0; }
        body {
            font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif;
            background: #0a0a0a;
            color: #e0e0e0;
            min-height: 100vh;
            display: flex;
            align-items: center;
            justify-content: center;
            padding: 20px;
        }
        .wizard {
            background: #111;
            border: 1px solid #333;
            max-width: 600px;
            width: 100%;
            padding: 40px;
        }
        h1 {
            font-size: 24px;
            margin-bottom: 10px;
            color: #00ff41;
            letter-spacing: -0.5px;
        }
        .subtitle {
            color: #666;
            font-size: 14px;
            margin-bottom: 30px;
        }
        .section {
            margin-bottom: 30px;
            padding-bottom: 20px;
            border-bottom: 1px solid #222;
        }
        .section h2 {
            font-size: 12px;
            text-transform: uppercase;
            letter-spacing: 0.1em;
            color: #888;
            margin-bottom: 15px;
        }
        .field {
            margin-bottom: 15px;
        }
        label {
            display: block;
            font-size: 11px;
            text-transform: uppercase;
            letter-spacing: 0.05em;
            color: #666;
            margin-bottom: 5px;
        }
        input, select {
            width: 100%;
            background: #000;
            border: 1px solid #333;
            color: #e0e0e0;
            padding: 10px;
            font-size: 14px;
            font-family: monospace;
        }
        input:focus, select:focus {
            outline: none;
            border-color: #00ff41;
        }
        .row {
            display: flex;
            gap: 15px;
        }
        .row .field { flex: 1; }
        .hint {
            font-size: 11px;
            color: #555;
            margin-top: 5px;
        }
        .db-fields { display: none; }
        .db-fields.active { display: block; }
        button[type="submit"] {
            width: 100%;
            background: #00ff41;
            color: #000;
            border: none;
            padding: 15px;
            font-size: 14px;
            font-weight: bold;
            text-transform: uppercase;
            letter-spacing: 0.1em;
            cursor: pointer;
            margin-top: 20px;
        }
        button[type="submit"]:hover {
            background: #00cc33;
        }
        .error {
            background: #330000;
            border: 1px solid #660000;
            color: #ff6666;
            padding: 10px;
            margin-bottom: 20px;
            font-size: 13px;
        }
    </style>
</head>
<body>
    <div class="wizard">
        <h1>🚀 G0 CMS SETUP</h1>
        <p class="subtitle">Lightweight CMS solution. Configure in under 2 minutes.</p>

        {{if .Error}}
        <div class="error">{{.Error}}</div>
        {{end}}

        <form method="POST" action="/setup/complete">
            <div class="section">
                <h2>Site Configuration</h2>
                <div class="field">
                    <label>Site Name</label>
                    <input type="text" name="site_name" placeholder="My Awesome Site" required>
                </div>
                <div class="field">
                    <label>Site URL</label>
                    <input type="url" name="site_url" placeholder="https://example.com" required>
                    <p class="hint">The public URL where your site will be accessible</p>
                </div>
                <div class="row">
                    <div class="field">
                        <label>Tagline</label>
                        <input type="text" name="site_tagline" placeholder="A brief description">
                    </div>
                    <div class="field">
                        <label>Author</label>
                        <input type="text" name="site_author" placeholder="Your Name">
                    </div>
                </div>
            </div>

            <div class="section">
                <h2>Database</h2>
                <div class="field">
                    <label>Database Type</label>
                    <select name="db_driver" id="db-driver" onchange="toggleDbFields()">
                        <option value="sqlite">SQLite (Recommended for single server)</option>
                        <option value="postgres">PostgreSQL (For high traffic)</option>
                    </select>
                </div>
                <div id="sqlite-fields" class="db-fields active">
                    <div class="field">
                        <label>Database Path</label>
                        <input type="text" name="sqlite_path" value="./data/g0.db">
                        <p class="hint">File path for SQLite database</p>
                    </div>
                </div>
                <div id="postgres-fields" class="db-fields">
                    <div class="row">
                        <div class="field">
                            <label>Host</label>
                            <input type="text" name="pg_host" placeholder="localhost">
                        </div>
                        <div class="field">
                            <label>Port</label>
                            <input type="text" name="pg_port" placeholder="5432" value="5432">
                        </div>
                    </div>
                    <div class="field">
                        <label>Database Name</label>
                        <input type="text" name="pg_dbname" placeholder="antigravity">
                    </div>
                    <div class="row">
                        <div class="field">
                            <label>Username</label>
                            <input type="text" name="pg_user" placeholder="postgres">
                        </div>
                        <div class="field">
                            <label>Password</label>
                            <input type="password" name="pg_password">
                        </div>
                    </div>
                </div>
            </div>

            <div class="section">
                <h2>Google OAuth (Required for Admin Access)</h2>
                <div class="field">
                    <label>Admin Email</label>
                    <input type="email" name="admin_email" placeholder="you@gmail.com" required>
                    <p class="hint">Must match your Google account email</p>
                </div>
                <div class="field">
                    <label>Google Client ID</label>
                    <input type="text" name="oauth_client_id" placeholder="xxx.apps.googleusercontent.com" required>
                </div>
                <div class="field">
                    <label>Google Client Secret</label>
                    <input type="password" name="oauth_client_secret" required>
                </div>
                <p class="hint">Get credentials at: <a href="https://console.cloud.google.com/apis/credentials" target="_blank" style="color:#00ff41">console.cloud.google.com/apis/credentials</a></p>
            </div>

            <div class="section">
                <h2>Server</h2>
                <div class="row">
                    <div class="field">
                        <label>Host</label>
                        <input type="text" name="server_host" value="0.0.0.0">
                    </div>
                    <div class="field">
                        <label>Port</label>
                        <input type="text" name="server_port" value="8080">
                    </div>
                </div>
                <div class="field">
                    <label>Admin Panel Path</label>
                    <input type="text" name="admin_path" value="/admin" placeholder="/admin">
                    <p class="hint">Custom URL for admin panel (e.g., /admin, /dashboard, /secret-panel)</p>
                </div>
            </div>

            <button type="submit">Complete Setup</button>
        </form>
    </div>

    <script>
        function toggleDbFields() {
            const driver = document.getElementById('db-driver').value;
            document.getElementById('sqlite-fields').classList.toggle('active', driver === 'sqlite');
            document.getElementById('postgres-fields').classList.toggle('active', driver === 'postgres');
        }
    </script>
</body>
</html>`

	tmpl, err := template.New("setup").Parse(html)
	if err != nil {
		http.Error(w, "Template error", http.StatusInternalServerError)
		return
	}

	data := struct{ Error string }{Error: r.URL.Query().Get("error")}
	tmpl.Execute(w, data)
}

// completeSetupHandler processes the setup form
func completeSetupHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}

	if !isFirstRun() {
		http.Error(w, "Setup already completed", http.StatusForbidden)
		return
	}

	// Build config from form
	siteURL := strings.TrimSuffix(r.FormValue("site_url"), "/")

	cfg := Config{
		Site: SiteConfig{
			Name:        r.FormValue("site_name"),
			URL:         siteURL,
			Tagline:     r.FormValue("site_tagline"),
			Description: r.FormValue("site_tagline"),
			Author:      r.FormValue("site_author"),
		},
		Database: DatabaseConfig{
			Driver: r.FormValue("db_driver"),
		},
		Auth: AuthConfig{
			Provider:      "google",
			ClientID:      r.FormValue("oauth_client_id"),
			ClientSecret:  r.FormValue("oauth_client_secret"),
			RedirectURL:   siteURL + "/api/auth/callback",
			AllowedEmails: []string{r.FormValue("admin_email")},
			SessionKey:    generateSessionKey(),
		},
		Server: ServerConfig{
			Host:      r.FormValue("server_host"),
			Port:      r.FormValue("server_port"),
			AdminPath: r.FormValue("admin_path"),
		},
	}

	// Database config based on driver (default to SQLite if not specified)
	if cfg.Database.Driver == "" {
		cfg.Database.Driver = "sqlite"
	}
	if cfg.Database.Driver == "sqlite" {
		cfg.Database.Path = r.FormValue("sqlite_path")
		if cfg.Database.Path == "" {
			cfg.Database.Path = "./data/g0.db"
		}
	} else {
		cfg.Database.Host = r.FormValue("pg_host")
		cfg.Database.Port = r.FormValue("pg_port")
		cfg.Database.User = r.FormValue("pg_user")
		cfg.Database.Password = r.FormValue("pg_password")
		cfg.Database.DBName = r.FormValue("pg_dbname")
	}

	// Validate
	if cfg.Site.Name == "" || cfg.Site.URL == "" {
		http.Redirect(w, r, "/?error=Site+name+and+URL+are+required", http.StatusFound)
		return
	}
	if cfg.Auth.ClientID == "" || cfg.Auth.ClientSecret == "" {
		http.Redirect(w, r, "/?error=OAuth+credentials+are+required", http.StatusFound)
		return
	}

	// Save config
	if err := saveConfig(cfg); err != nil {
		log.Printf("Failed to save config: %v", err)
		http.Redirect(w, r, "/?error=Failed+to+save+configuration", http.StatusFound)
		return
	}

	// Load config globally
	config = cfg

	// Initialize database
	if err := initDB(); err != nil {
		log.Printf("Failed to initialize database: %v", err)
		os.Remove(configPath) // Rollback
		http.Redirect(w, r, "/?error=Database+initialization+failed:+"+err.Error(), http.StatusFound)
		return
	}

	// Ensure admin path has leading slash
	if config.Server.AdminPath == "" {
		config.Server.AdminPath = "/admin"
	}
	if !strings.HasPrefix(config.Server.AdminPath, "/") {
		config.Server.AdminPath = "/" + config.Server.AdminPath
	}

	log.Println("✅ Setup completed successfully!")
	http.Redirect(w, r, config.Server.AdminPath, http.StatusFound)
}

// ============================================================================
// DATABASE
// ============================================================================

func initDB() error {
	var err error

	if config.Database.Driver == "postgres" {
		connStr := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
			config.Database.Host, config.Database.Port, config.Database.User,
			config.Database.Password, config.Database.DBName)
		db, err = sql.Open("postgres", connStr)
	} else {
		// SQLite
		db, err = sql.Open("sqlite", config.Database.Path)
	}

	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}

	// Run migrations
	return runMigrations()
}

func runMigrations() error {
	var sqlStmt string

	if config.Database.Driver == "postgres" {
		sqlStmt = `
		CREATE TABLE IF NOT EXISTS content (
			id SERIAL PRIMARY KEY,
			type TEXT,
			title TEXT,
			date TEXT,
			content TEXT,
			metadata TEXT
		);`
	} else {
		sqlStmt = `
		CREATE TABLE IF NOT EXISTS content (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			type TEXT,
			title TEXT,
			date TEXT,
			content TEXT,
			metadata TEXT
		);`
	}

	_, err := db.Exec(sqlStmt)
	if err != nil {
		return fmt.Errorf("migration failed: %w", err)
	}

	log.Println("📦 Database migrations completed")
	return nil
}

// ============================================================================
// AUTH
// ============================================================================

func initAuth() {
	if config.Auth.ClientID == "" {
		log.Println("⚠️ OAuth not configured, admin panel will be inaccessible")
		return
	}

	ctx := context.Background()
	provider, err := oidc.NewProvider(ctx, "https://accounts.google.com")
	if err != nil {
		log.Printf("OIDC Provider init failed: %v", err)
		return
	}
	oidcProvider = provider

	var keyPairs [][]byte
	keyPairs = append(keyPairs, []byte(config.Auth.SessionKey))
	if config.Auth.SessionKeyOld != "" {
		keyPairs = append(keyPairs, []byte(config.Auth.SessionKeyOld))
	}

	store = sessions.NewCookieStore(keyPairs...)
	store.Options = &sessions.Options{
		Path:     "/",
		MaxAge:   3600 * 8,
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteStrictMode,
	}

	oauthConfig = &oauth2.Config{
		ClientID:     config.Auth.ClientID,
		ClientSecret: config.Auth.ClientSecret,
		RedirectURL:  config.Auth.RedirectURL,
		Endpoint:     provider.Endpoint(),
		Scopes:       []string{oidc.ScopeOpenID, "profile", "email"},
	}

	verifier = provider.Verifier(&oidc.Config{ClientID: config.Auth.ClientID})
	log.Println("🔐 OAuth initialized")
}

func withAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if store == nil {
			http.Error(w, "Auth not configured", http.StatusServiceUnavailable)
			return
		}
		session, _ := store.Get(r, "auth-session")
		if auth, ok := session.Values["authenticated"].(bool); !ok || !auth {
			http.Redirect(w, r, "/api/auth/login", http.StatusFound)
			return
		}
		next(w, r)
	}
}

func authLoginHandler(w http.ResponseWriter, r *http.Request) {
	if oauthConfig == nil {
		http.Error(w, "Auth not configured", http.StatusInternalServerError)
		return
	}
	state := make([]byte, 32)
	rand.Read(state)
	stateStr := base64.StdEncoding.EncodeToString(state)

	session, _ := store.Get(r, "auth-session")
	session.Values["state"] = stateStr
	session.Save(r, w)

	http.Redirect(w, r, oauthConfig.AuthCodeURL(stateStr), http.StatusFound)
}

func authCallbackHandler(w http.ResponseWriter, r *http.Request) {
	session, _ := store.Get(r, "auth-session")
	state, ok := session.Values["state"].(string)

	if !ok || r.URL.Query().Get("state") != state {
		http.Error(w, "Invalid state", http.StatusBadRequest)
		return
	}

	token, err := oauthConfig.Exchange(r.Context(), r.URL.Query().Get("code"))
	if err != nil {
		http.Error(w, "Token exchange failed", http.StatusInternalServerError)
		return
	}

	rawIDToken, ok := token.Extra("id_token").(string)
	if !ok {
		http.Error(w, "No id_token", http.StatusInternalServerError)
		return
	}

	idToken, err := verifier.Verify(r.Context(), rawIDToken)
	if err != nil {
		http.Error(w, "Token verification failed", http.StatusInternalServerError)
		return
	}

	var claims struct {
		Email string `json:"email"`
	}
	if err := idToken.Claims(&claims); err != nil {
		http.Error(w, "Claims parse failed", http.StatusInternalServerError)
		return
	}

	// Check allowed emails
	allowed := false
	for _, email := range config.Auth.AllowedEmails {
		if strings.TrimSpace(email) == claims.Email {
			allowed = true
			break
		}
	}

	if !allowed {
		http.Error(w, "Unauthorized: Email not allowed", http.StatusUnauthorized)
		return
	}

	session.Values["authenticated"] = true
	session.Values["email"] = claims.Email
	delete(session.Values, "state")
	session.Save(r, w)

	http.Redirect(w, r, config.Server.AdminPath, http.StatusFound)
}

// ============================================================================
// WEBSOCKET TERMINAL
// ============================================================================

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin:     func(r *http.Request) bool { return true },
}

func terminalHandler(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("Terminal Upgrade Error: %v", err)
		return
	}
	defer conn.Close()

	cmd := exec.Command("bash", "-l")
	f, err := pty.Start(cmd)
	if err != nil {
		log.Printf("PTY Error: %v", err)
		return
	}
	defer f.Close()

	pty.Setsize(f, &pty.Winsize{Rows: 40, Cols: 120})

	go func() {
		buf := make([]byte, 1024)
		for {
			n, err := f.Read(buf)
			if err != nil {
				return
			}
			if err := conn.WriteMessage(websocket.TextMessage, buf[:n]); err != nil {
				return
			}
		}
	}()

	for {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			break
		}

		var wsMsg struct {
			Type string `json:"type"`
			Data string `json:"data"`
			Cols int    `json:"cols"`
			Rows int    `json:"rows"`
		}

		if err := json.Unmarshal(msg, &wsMsg); err != nil {
			f.Write(msg)
			continue
		}

		switch wsMsg.Type {
		case "resize":
			if wsMsg.Cols > 0 && wsMsg.Rows > 0 {
				pty.Setsize(f, &pty.Winsize{Rows: uint16(wsMsg.Rows), Cols: uint16(wsMsg.Cols)})
			}
		case "input":
			f.Write([]byte(wsMsg.Data))
		default:
			f.Write(msg)
		}
	}
}

// ============================================================================
// CONTENT TYPES
// ============================================================================

type ContentEntry struct {
	ID       int    `json:"id"`
	Type     string `json:"type"`
	Title    string `json:"title"`
	Date     string `json:"date"`
	Content  string `json:"content"`
	Metadata string `json:"metadata"`
	Slug     string `json:"slug"`
}

type SEOMetadata struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	Image       string `json:"og_image"`
	Keywords    string `json:"keywords"`
}

// ============================================================================
// HELPERS
// ============================================================================

func slugify(title string) string {
	s := strings.ToLower(title)
	s = strings.ReplaceAll(s, " ", "-")
	reg, _ := regexp.Compile("[^a-z0-9-]+")
	s = reg.ReplaceAllString(s, "")
	reg2, _ := regexp.Compile("-+")
	s = reg2.ReplaceAllString(s, "-")
	return strings.Trim(s, "-")
}

func stripHTML(content string, limit int) string {
	re := regexp.MustCompile("<[^>]*>")
	plain := re.ReplaceAllString(content, " ")
	plain = strings.Join(strings.Fields(plain), " ")
	if len(plain) > limit {
		return plain[:limit] + "..."
	}
	return plain
}

func getFallbackContent(contentType string) string {
	filename := fmt.Sprintf("templates/%s.html", contentType)
	content, err := embeddedFiles.ReadFile(filename)
	if err != nil {
		return ""
	}
	s := string(content)
	s = strings.ReplaceAll(s, `{{define "content"}}`, "")
	s = strings.ReplaceAll(s, `{{end}}`, "")
	return strings.TrimSpace(s)
}

func fetchSPAContent() string {
	rows, err := db.Query("SELECT id, type, title, date, content, metadata FROM content WHERE type IN ('home', 'resume', 'breakdown', 'blog') ORDER BY id DESC")
	if err != nil {
		return "[]"
	}
	defer rows.Close()

	var entries []ContentEntry
	for rows.Next() {
		var c ContentEntry
		var meta sql.NullString
		if err := rows.Scan(&c.ID, &c.Type, &c.Title, &c.Date, &c.Content, &meta); err != nil {
			continue
		}
		c.Metadata = meta.String
		c.Slug = slugify(c.Title)
		entries = append(entries, c)
	}

	coreTypes := []string{"home", "resume", "contact"}
	for _, t := range coreTypes {
		found := false
		for _, e := range entries {
			if e.Type == t {
				found = true
				break
			}
		}
		if !found {
			content := getFallbackContent(t)
			if content != "" {
				entries = append(entries, ContentEntry{
					Type:    t,
					Title:   strings.Title(t),
					Content: content,
					Date:    time.Now().Format("Jan 02, 2006"),
					Slug:    t,
				})
			}
		}
	}

	finalJSON, _ := json.Marshal(entries)
	return string(finalJSON)
}

func getTheme(r *http.Request) string {
	cookie, err := r.Cookie("theme")
	if err != nil || cookie.Value == "" {
		return "dark"
	}
	return cookie.Value
}

func getPlaceholder(key, fallback string) string {
	switch key {
	case "site_name":
		if config.Site.Name != "" {
			return config.Site.Name
		}
	case "site_url":
		if config.Site.URL != "" {
			return config.Site.URL
		}
	case "site_tagline":
		if config.Site.Tagline != "" {
			return config.Site.Tagline
		}
	case "site_author":
		if config.Site.Author != "" {
			return config.Site.Author
		}
	}
	return fallback
}

// ============================================================================
// MAIN
// ============================================================================

func main() {
	godotenv.Load()

	// Setup logging
	logPath := "/app/server.log"
	if _, err := os.Stat("/app"); os.IsNotExist(err) {
		logPath = "server.log"
	}
	logFile, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		fmt.Printf("Failed to open server.log: %v\n", err)
	} else {
		defer logFile.Close()
		log.SetOutput(io.MultiWriter(os.Stdout, logFile))
	}

	// Check if first run
	if isFirstRun() {
		log.Println("🆕 First run detected - starting setup wizard")
		http.HandleFunc("/", setupWizardHandler)
		http.HandleFunc("/setup/complete", completeSetupHandler)

		// Serve static files for setup page
		staticHandler := http.FileServer(http.FS(staticFS))
		http.Handle("/static/", http.StripPrefix("/static/", staticHandler))

		port := os.Getenv("PORT")
		if port == "" {
			port = "8080"
		}
		fmt.Printf("🚀 Setup wizard running on :%s\n", port)
		log.Fatal(http.ListenAndServe(":"+port, nil))
		return
	}

	// Load config
	if err := loadConfig(); err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Initialize
	initAuth()
	if err := initDB(); err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	defer db.Close()

	// ========================================================================
	// ROUTES
	// ========================================================================

	// Static files
	staticHandler := http.StripPrefix("/static/", http.FileServer(http.FS(staticFS)))
	http.HandleFunc("/static/", func(w http.ResponseWriter, r *http.Request) {
		cleanPath := strings.TrimPrefix(r.URL.Path, "/static/")
		if cleanPath == "" || strings.HasSuffix(cleanPath, "/") {
			http.NotFound(w, r)
			return
		}

		if _, err := staticFS.Open(cleanPath); err != nil {
			http.NotFound(w, r)
			return
		}

		// Protect admin JS
		if cleanPath == "js/admin.js" || cleanPath == "js/custom-editor.js" {
			session, _ := store.Get(r, "auth-session")
			if auth, ok := session.Values["authenticated"].(bool); !ok || !auth {
				http.NotFound(w, r)
				return
			}
		}

		staticHandler.ServeHTTP(w, r)
	})

	// Auth routes
	http.HandleFunc("/api/auth/login", authLoginHandler)
	http.HandleFunc("/api/auth/callback", authCallbackHandler)
	http.HandleFunc("/api/auth/logout", func(w http.ResponseWriter, r *http.Request) {
		session, _ := store.Get(r, "auth-session")
		session.Values["authenticated"] = false
		session.Save(r, w)
		http.Redirect(w, r, "/", http.StatusFound)
	})

	// Terminal
	http.HandleFunc("/api/terminal", withAuth(terminalHandler))

	// Content save handler
	saveHandler := func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		idStr := r.FormValue("id")
		cType := r.FormValue("type")
		title := r.FormValue("title")
		content := r.FormValue("content")
		metadata := r.FormValue("metadata")
		date := r.FormValue("date")

		if cType == "" {
			cType = "blog"
		}
		if date == "" {
			date = time.Now().Format("Jan 02, 2006")
		}

		var err error
		if config.Database.Driver == "postgres" {
			if idStr != "" {
				_, err = db.Exec("UPDATE content SET type=$1, title=$2, date=$3, content=$4, metadata=$5 WHERE id=$6",
					cType, title, date, content, metadata, idStr)
			} else {
				_, err = db.Exec("INSERT INTO content (type, title, date, content, metadata) VALUES ($1, $2, $3, $4, $5)",
					cType, title, date, content, metadata)
			}
		} else {
			if idStr != "" {
				_, err = db.Exec("UPDATE content SET type=?, title=?, date=?, content=?, metadata=? WHERE id=?",
					cType, title, date, content, metadata, idStr)
			} else {
				_, err = db.Exec("INSERT INTO content (type, title, date, content, metadata) VALUES (?, ?, ?, ?, ?)",
					cType, title, date, content, metadata)
			}
		}

		if err != nil {
			log.Printf("Persistence error: %v", err)
			w.Write([]byte("ERR: PERSISTENCE_FAILURE"))
			return
		}

		if r.Header.Get("HX-Request") == "true" {
			action := "SAVED"
			if idStr != "" {
				action = "UPDATED"
			}
			w.Write([]byte(fmt.Sprintf("PROTOCOL_%s [%s]", action, time.Now().Format("15:04:05"))))
			return
		}
		w.Write([]byte("PROTOCOL_SAVED"))
	}

	http.HandleFunc("/api/save", withAuth(saveHandler))
	http.HandleFunc("/api/posts", withAuth(saveHandler))

	// Delete handler
	http.HandleFunc("/api/delete", withAuth(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost && r.Method != http.MethodDelete {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		id := r.URL.Query().Get("id")
		if id == "" {
			http.Error(w, "Missing ID", http.StatusBadRequest)
			return
		}

		var err error
		if config.Database.Driver == "postgres" {
			_, err = db.Exec("DELETE FROM content WHERE id = $1", id)
		} else {
			_, err = db.Exec("DELETE FROM content WHERE id = ?", id)
		}

		if err != nil {
			http.Error(w, "Database Error", http.StatusInternalServerError)
			return
		}
		w.Write([]byte("PROTOCOL_DELETED"))
	}))

	// Robots.txt
	http.HandleFunc("/robots.txt", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.Write([]byte(fmt.Sprintf("User-agent: *\nAllow: /\nSitemap: %s/sitemap.xml", config.Site.URL)))
	})

	// Favicon
	http.HandleFunc("/favicon.ico", func(w http.ResponseWriter, r *http.Request) {
		data, err := embeddedFiles.ReadFile("static/favicon.ico")
		if err != nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "image/x-icon")
		w.Write(data)
	})

	// Sitemap
	http.HandleFunc("/sitemap.xml", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")

		fmt.Fprint(w, `<?xml version="1.0" encoding="UTF-8"?>
<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">`)

		baseURL := config.Site.URL
		staticPages := []string{"", "/journal", "/resume", "/contact"}
		for _, p := range staticPages {
			fmt.Fprintf(w, `
	<url>
		<loc>%s%s</loc>
		<changefreq>daily</changefreq>
		<priority>0.8</priority>
	</url>`, baseURL, p)
		}

		rows, err := db.Query("SELECT title, date FROM content WHERE type='blog'")
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var title, dateStr string
				if err := rows.Scan(&title, &dateStr); err == nil {
					slug := slugify(title)
					parsedDate, parseErr := time.Parse("Jan 02, 2006", dateStr)
					isoDate := time.Now().Format("2006-01-02")
					if parseErr == nil {
						isoDate = parsedDate.Format("2006-01-02")
					}
					fmt.Fprintf(w, `
	<url>
		<loc>%s/journal/%s</loc>
		<lastmod>%s</lastmod>
		<changefreq>weekly</changefreq>
		<priority>0.6</priority>
	</url>`, baseURL, slug, isoDate)
				}
			}
		}

		fmt.Fprint(w, "\n</urlset>")
	})

	// Atom Feed
	atomFeedHandler := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/atom+xml")

		rows, err := db.Query("SELECT title, date, content, metadata FROM content WHERE type='blog' ORDER BY id DESC")
		if err != nil {
			http.Error(w, "Error generating feed", http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		baseURL := config.Site.URL
		siteName := config.Site.Name
		author := config.Site.Author
		if author == "" {
			author = siteName
		}

		fmt.Fprintf(w, `<?xml version="1.0" encoding="utf-8"?>
<feed xmlns="http://www.w3.org/2005/Atom">
  <title>%s | Journal</title>
  <subtitle>%s</subtitle>
  <link href="%s/feed.atom" rel="self"/>
  <link href="%s/"/>
  <updated>%s</updated>
  <id>%s/</id>
  <author>
    <name>%s</name>
  </author>`, siteName, config.Site.Tagline, baseURL, baseURL, time.Now().Format(time.RFC3339), baseURL, author)

		for rows.Next() {
			var title, dateStr, content, metadataStr string
			if err := rows.Scan(&title, &dateStr, &content, &metadataStr); err == nil {
				slug := slugify(title)
				parsedDate, parseErr := time.Parse("Jan 02, 2006", dateStr)
				isoDate := time.Now().Format("2006-01-02T15:04:05Z")
				if parseErr == nil {
					isoDate = parsedDate.Format("2006-01-02T15:04:05Z")
				}

				summary := ""
				var metadata map[string]interface{}
				if err := json.Unmarshal([]byte(metadataStr), &metadata); err == nil {
					if desc, ok := metadata["description"].(string); ok && desc != "" {
						summary = desc
					}
				}
				if summary == "" {
					summary = stripHTML(content, 200)
				}

				fmt.Fprintf(w, `
  <entry>
    <title>%s</title>
    <link href="%s/journal/%s"/>
    <id>%s/journal/%s</id>
    <updated>%s</updated>
    <summary>%s</summary>
  </entry>`, template.HTMLEscapeString(title), baseURL, slug, baseURL, slug, isoDate, template.HTMLEscapeString(summary))
			}
		}

		fmt.Fprint(w, "\n</feed>")
	}

	http.HandleFunc("/feed.atom", atomFeedHandler)
	http.HandleFunc("/atom.xml", atomFeedHandler)
	http.HandleFunc("/rss.xml", atomFeedHandler)

	// DB Manager APIs
	http.HandleFunc("/api/db/table/", withAuth(func(w http.ResponseWriter, r *http.Request) {
		tableName := strings.TrimPrefix(r.URL.Path, "/api/db/table/")
		validTables := map[string]bool{"content": true}
		if !validTables[tableName] {
			http.Error(w, "Invalid Table", http.StatusBadRequest)
			return
		}

		filter := r.URL.Query().Get("filter")
		upperFilter := strings.ToUpper(filter)
		if strings.Contains(upperFilter, "DROP") || strings.Contains(upperFilter, "DELETE") ||
			strings.Contains(upperFilter, "UPDATE") || strings.Contains(upperFilter, "INSERT") {
			http.Error(w, "Read-only filters allowed", http.StatusBadRequest)
			return
		}

		orderCol := "id"
		query := fmt.Sprintf("SELECT * FROM %s ORDER BY %s DESC", tableName, orderCol)
		if filter != "" {
			query = fmt.Sprintf("SELECT * FROM %s %s", tableName, filter)
		}

		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()

		rows, err := db.QueryContext(ctx, query)
		if err != nil {
			http.Error(w, fmt.Sprintf("Query Error: %v", err), http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		cols, _ := rows.Columns()
		var result []map[string]interface{}

		for rows.Next() {
			columns := make([]interface{}, len(cols))
			columnPointers := make([]interface{}, len(cols))
			for i := range columns {
				columnPointers[i] = &columns[i]
			}

			if err := rows.Scan(columnPointers...); err != nil {
				continue
			}

			m := make(map[string]interface{})
			for i, colName := range cols {
				val := columns[i]
				if b, ok := val.([]byte); ok {
					m[colName] = string(b)
				} else {
					m[colName] = val
				}
			}
			result = append(result, m)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(result)
	}))

	http.HandleFunc("/api/db/update", withAuth(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		table := r.FormValue("table")
		id := r.FormValue("id")
		col := r.FormValue("col")
		val := r.FormValue("val")

		validTables := map[string]bool{"content": true}
		if !validTables[table] {
			http.Error(w, "Invalid Table", http.StatusBadRequest)
			return
		}

		isAlphaNum := regexp.MustCompile(`^[a-zA-Z0-9_]+$`).MatchString
		if !isAlphaNum(col) {
			http.Error(w, "Invalid Column", http.StatusBadRequest)
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		var err error
		if config.Database.Driver == "postgres" {
			query := fmt.Sprintf("UPDATE %s SET %s = $1 WHERE id = $2", table, col)
			_, err = db.ExecContext(ctx, query, val, id)
		} else {
			query := fmt.Sprintf("UPDATE %s SET %s = ? WHERE id = ?", table, col)
			_, err = db.ExecContext(ctx, query, val, id)
		}

		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Write([]byte("PROTOCOL_UPDATED"))
	}))

	// Admin panel
	type AdminData struct {
		Title       string
		View        string
		Posts       []ContentEntry
		CurrentType string
		PostsJS     template.JS
		Theme       string
	}

	// Admin panel - path configurable via config.yaml
	http.HandleFunc(config.Server.AdminPath, withAuth(func(w http.ResponseWriter, r *http.Request) {
		filterTypes := r.URL.Query().Get("types")
		var postRows *sql.Rows
		var err error

		if filterTypes != "" && filterTypes != "all" {
			types := strings.Split(filterTypes, ",")
			if config.Database.Driver == "postgres" {
				placeholders := make([]string, len(types))
				args := make([]interface{}, len(types))
				for i, t := range types {
					placeholders[i] = fmt.Sprintf("$%d", i+1)
					args[i] = t
				}
				query := fmt.Sprintf("SELECT id, type, title, date, content, metadata FROM content WHERE type IN (%s) ORDER BY id DESC", strings.Join(placeholders, ","))
				postRows, err = db.Query(query, args...)
			} else {
				placeholders := make([]string, len(types))
				args := make([]interface{}, len(types))
				for i, t := range types {
					placeholders[i] = "?"
					args[i] = t
				}
				query := fmt.Sprintf("SELECT id, type, title, date, content, metadata FROM content WHERE type IN (%s) ORDER BY id DESC", strings.Join(placeholders, ","))
				postRows, err = db.Query(query, args...)
			}
		} else {
			postRows, err = db.Query("SELECT id, type, title, date, content, metadata FROM content ORDER BY id DESC LIMIT 100")
		}

		if err != nil {
			http.Error(w, "Database Error", http.StatusInternalServerError)
			return
		}

		var posts []ContentEntry
		if postRows != nil {
			defer postRows.Close()
			for postRows.Next() {
				var p ContentEntry
				postRows.Scan(&p.ID, &p.Type, &p.Title, &p.Date, &p.Content, &p.Metadata)
				posts = append(posts, p)
			}
		}
		postsJSON, _ := json.Marshal(posts)

		tmpl, err := template.ParseFS(embeddedFiles, "templates/admin.html")
		if err != nil {
			http.Error(w, fmt.Sprintf("Template Error: %v", err), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "text/html")
		tmpl.Execute(w, AdminData{
			Title:       "Protocol Control Center",
			View:        "admin",
			Posts:       posts,
			CurrentType: "all",
			PostsJS:     template.JS(postsJSON),
			Theme:       getTheme(r),
		})
	}))

	// 404 renderer
	render404 := func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		tmpl, err := template.ParseFS(embeddedFiles, "templates/404.html")
		if err != nil {
			http.Error(w, "Page Not Found", http.StatusNotFound)
			return
		}
		tmpl.Execute(w, struct{ Theme string }{Theme: getTheme(r)})
	}

	// Journal handler
	journalHandler := func(w http.ResponseWriter, r *http.Request) {
		fullPath := r.URL.Path
		slug := ""
		if strings.HasPrefix(fullPath, "/journal/") {
			slug = strings.TrimPrefix(fullPath, "/journal/")
		}
		slug = strings.Trim(slug, "/")

		rows, _ := db.Query("SELECT id, type, title, date, content, metadata FROM content WHERE type='blog' ORDER BY id DESC")
		defer rows.Close()

		var blogs []ContentEntry
		var activeBlog *ContentEntry
		for rows.Next() {
			var c ContentEntry
			rows.Scan(&c.ID, &c.Type, &c.Title, &c.Date, &c.Content, &c.Metadata)
			c.Slug = slugify(c.Title)
			if c.Slug == slug {
				activeBlog = &c
			}
			blogs = append(blogs, c)
		}

		if activeBlog == nil && len(blogs) > 0 {
			if slug == "" {
				activeBlog = &blogs[0]
			} else {
				http.NotFound(w, r)
				return
			}
		}

		if activeBlog == nil {
			http.Error(w, "No blog posts found", http.StatusNotFound)
			return
		}

		tmpl := template.New("index.html")
		tmpl, err = tmpl.ParseFS(embeddedFiles, "templates/index.html", "templates/journal.html")
		if err != nil {
			http.Error(w, "Template Parse Error", http.StatusInternalServerError)
			return
		}

		var seoMeta SEOMetadata
		seoMeta.Image = config.Site.URL + "/static/images/og-image.jpg"

		pageTitle := "Journal | " + config.Site.Name
		if activeBlog.Metadata != "" {
			json.Unmarshal([]byte(activeBlog.Metadata), &seoMeta)
			if seoMeta.Title != "" {
				pageTitle = seoMeta.Title
			} else if activeBlog.Title != "" {
				pageTitle = activeBlog.Title + " | " + config.Site.Name
			}
		}

		if seoMeta.Image == "" {
			seoMeta.Image = config.Site.URL + "/static/images/og-image.jpg"
		}

		data := map[string]interface{}{
			"Title":       pageTitle,
			"Blogs":       blogs,
			"ActiveBlog":  activeBlog,
			"ActiveSlug":  activeBlog.Slug,
			"View":        "journal",
			"Data":        template.JS(fetchSPAContent()),
			"Theme":       getTheme(r),
			"SeoDesc":     seoMeta.Description,
			"SeoImage":    seoMeta.Image,
			"SeoKeywords": seoMeta.Keywords,
			"AdminPath":   config.Server.AdminPath,
		}

		contentStr := activeBlog.Content
		if !strings.HasPrefix(strings.TrimSpace(contentStr), "<") {
			output := markdown.ToHTML([]byte(contentStr), nil, nil)
			data["ActiveBlogContent"] = template.HTML(output)
		} else {
			data["ActiveBlogContent"] = template.HTML(contentStr)
		}

		w.Header().Set("Content-Type", "text/html")
		tmpl.ExecuteTemplate(w, "index.html", data)
	}

	http.HandleFunc("/journal", journalHandler)
	http.HandleFunc("/journal/", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/") && r.URL.Path != "/journal/" {
			http.Redirect(w, r, strings.TrimSuffix(r.URL.Path, "/"), http.StatusMovedPermanently)
			return
		}
		journalHandler(w, r)
	})

	// Page data types
	type PageData struct {
		Title       string
		Data        template.JS
		View        string
		ActiveSlug  string
		Theme       string
		SeoDesc     string
		SeoImage    string
		SeoKeywords string
		AdminPath   string
	}

	// Root handler
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" && strings.HasSuffix(r.URL.Path, "/") {
			http.Redirect(w, r, strings.TrimSuffix(r.URL.Path, "/"), http.StatusMovedPermanently)
			return
		}

		path := strings.TrimPrefix(r.URL.Path, "/")

		rows, err := db.Query("SELECT id, type, title, date, content, metadata FROM content ORDER BY id DESC")
		if err != nil {
			http.Error(w, "Database Error", http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		var entries []ContentEntry
		for rows.Next() {
			var c ContentEntry
			rows.Scan(&c.ID, &c.Type, &c.Title, &c.Date, &c.Content, &c.Metadata)
			c.Slug = slugify(c.Title)
			entries = append(entries, c)
		}
		rows.Close()

		jsonData, _ := json.Marshal(entries)

		parts := strings.Split(path, "/")
		view := "home"
		activeSlug := ""
		if len(parts) > 0 && parts[0] != "" {
			view = parts[0]
			if view == "journal" && len(parts) > 1 {
				activeSlug = parts[1]
			}
		}

		var seoMeta SEOMetadata
		pageTitle := config.Site.Name + " | " + config.Site.Tagline
		var matchedEntry *ContentEntry
		targetTemplate := "templates/home.html"

		switch view {
		case "home":
			targetTemplate = "templates/home.html"
			for i := range entries {
				if entries[i].Type == "home" {
					matchedEntry = &entries[i]
					break
				}
			}
		case "resume":
			targetTemplate = "templates/resume.html"
			for i := range entries {
				if entries[i].Type == "resume" {
					matchedEntry = &entries[i]
					break
				}
			}
		case "contact":
			targetTemplate = "templates/contact.html"
			for i := range entries {
				if entries[i].Type == "contact" {
					matchedEntry = &entries[i]
					break
				}
			}
		case "journal":
			found := false
			for i := range entries {
				if entries[i].Type == "blog" && (activeSlug == "" || entries[i].Slug == activeSlug) {
					matchedEntry = &entries[i]
					found = true
					break
				}
			}
			if !found && activeSlug != "" {
				render404(w, r)
				return
			}
		default:
			render404(w, r)
			return
		}

		if matchedEntry != nil {
			if matchedEntry.Metadata != "" {
				json.Unmarshal([]byte(matchedEntry.Metadata), &seoMeta)
			}
			if seoMeta.Title != "" {
				pageTitle = seoMeta.Title
			} else if matchedEntry.Title != "" {
				pageTitle = matchedEntry.Title + " | " + config.Site.Name
			}
		}

		if seoMeta.Image == "" {
			seoMeta.Image = config.Site.URL + "/static/images/og-image.jpg"
		}

		type ExtendedPageData struct {
			PageData
			Content template.HTML
		}

		pd := ExtendedPageData{
			PageData: PageData{
				Title:       pageTitle,
				Data:        template.JS(jsonData),
				View:        view,
				ActiveSlug:  activeSlug,
				Theme:       getTheme(r),
				SeoDesc:     seoMeta.Description,
				SeoImage:    seoMeta.Image,
				SeoKeywords: seoMeta.Keywords,
				AdminPath:   config.Server.AdminPath,
			},
			Content: "",
		}

		if matchedEntry != nil {
			pd.Content = template.HTML(matchedEntry.Content)
		}

		tmpl := template.New("index.html")
		tmpl, err = tmpl.ParseFS(embeddedFiles, "templates/index.html", targetTemplate)
		if err != nil {
			tmpl, _ = tmpl.ParseFS(embeddedFiles, "templates/index.html", "templates/home.html")
		}

		w.Header().Set("Content-Type", "text/html")
		if tmpl != nil {
			tmpl.Execute(w, pd)
		} else {
			http.Error(w, "Template not found", http.StatusInternalServerError)
		}
	})

	// Start server
	port := config.Server.Port
	if port == "" {
		port = "8080"
	}
	host := config.Server.Host
	if host == "" {
		host = "0.0.0.0"
	}

	addr := fmt.Sprintf("%s:%s", host, port)
	fmt.Printf("🚀 G0 CMS running on %s\n", addr)
	log.Fatal(http.ListenAndServe(addr, nil))
}

// ============================================================================
// HTTP RESPONSE WRAPPER
// ============================================================================

type statusResponseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (w *statusResponseWriter) WriteHeader(code int) {
	w.statusCode = code
	w.ResponseWriter.WriteHeader(code)
}

func (w *statusResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	h, ok := w.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, fmt.Errorf("hijacker interface not supported")
	}
	return h.Hijack()
}

func (w *statusResponseWriter) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}
