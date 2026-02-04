package main

import (
	"crypto/rand"
	"embed"
	"encoding/base64"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"log"
	"net/http"
	"os"
	"strings"

	"antigravity/internal/auth"
	"antigravity/internal/handlers"
	"antigravity/internal/services"

	"gopkg.in/yaml.v3"

	_ "github.com/lib/pq"
	_ "modernc.org/sqlite"
)

// ============================================================================
// CONFIGURATION
// ============================================================================

// Global config instance
var config handlers.Config

// ============================================================================
// EMBEDDED FILES
// ============================================================================

//go:embed templates static
var embeddedFiles embed.FS

var (
	coreStaticFS  fs.FS // Admin assets: /static/ (xterm, monaco, admin.js, etc.)
	themeStaticFS fs.FS // Theme assets: /templates/themes/default/static/ (css, js, images)
)

func init() {
	var err error
	coreStaticFS, err = fs.Sub(embeddedFiles, "static")
	if err != nil {
		log.Fatalf("Failed to create core static sub-filesystem: %v", err)
	}
	themeStaticFS, err = fs.Sub(embeddedFiles, "templates/themes/default/static")
	if err != nil {
		log.Fatalf("Failed to create theme static sub-filesystem: %v", err)
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
		log.Println("WARN: Generated missing session key - saving to config")
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

func saveConfig(cfg handlers.Config) error {
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
    <title>G0 CMS - Setup</title>
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
            margin-bottom: 30px;
            color: #00ff41;
        }
        .section {
            margin-bottom: 30px;
        }
        .section h2 {
            font-size: 14px;
            text-transform: uppercase;
            letter-spacing: 1px;
            margin-bottom: 15px;
            color: #888;
        }
        .field {
            margin-bottom: 15px;
        }
        label {
            display: block;
            margin-bottom: 5px;
            font-size: 12px;
            color: #aaa;
        }
        input, select {
            width: 100%;
            padding: 10px;
            background: #1a1a1a;
            border: 1px solid #333;
            color: #fff;
            font-size: 14px;
        }
        input:focus, select:focus {
            outline: none;
            border-color: #00ff41;
        }
        .row {
            display: flex;
            gap: 15px;
        }
        .row .field {
            flex: 1;
        }
        button {
            width: 100%;
            padding: 15px;
            background: #00ff41;
            border: none;
            color: #000;
            font-weight: bold;
            text-transform: uppercase;
            cursor: pointer;
        }
        button:hover {
            background: #00cc33;
        }
        .error {
            background: #330000;
            border: 1px solid #660000;
            padding: 10px;
            margin-bottom: 20px;
            color: #ff6666;
            font-size: 14px;
        }
        .hint {
            font-size: 11px;
            color: #666;
            margin-top: 5px;
        }
        #db-fields {
            margin-top: 15px;
            padding: 15px;
            background: #0f0f0f;
            border: 1px solid #222;
        }
        .db-fields {
            display: none;
        }
        .db-fields.active {
            display: block;
        }
    </style>
</head>
<body>
    <div class="wizard">
        <h1>G0 CMS Setup</h1>

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
                </div>
                <div class="field">
                    <label>Tagline</label>
                    <input type="text" name="site_tagline" placeholder="A lightweight CMS">
                </div>
                <div class="field">
                    <label>Author</label>
                    <input type="text" name="site_author" placeholder="Your Name">
                </div>
            </div>

            <div class="section">
                <h2>Database</h2>
                <div class="field">
                    <label>Database Type</label>
                    <select name="db_driver" id="db-driver" onchange="toggleDbFields()">
                        <option value="sqlite">SQLite (Recommended)</option>
                        <option value="postgres">PostgreSQL</option>
                    </select>
                </div>

                <div id="db-fields">
                    <div id="sqlite-fields" class="db-fields active">
                        <div class="field">
                            <label>SQLite Path</label>
                            <input type="text" name="sqlite_path" placeholder="./data/g0.db">
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
                            <input type="text" name="pg_dbname" placeholder="g0">
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

	cfg := handlers.Config{
		Site: handlers.SiteConfig{
			Name:        r.FormValue("site_name"),
			URL:         siteURL,
			Tagline:     r.FormValue("site_tagline"),
			Description: r.FormValue("site_tagline"),
			Author:      r.FormValue("site_author"),
		},
		Database: handlers.DatabaseConfig{
			Driver: r.FormValue("db_driver"),
		},
		Auth: handlers.AuthConfig{
			Provider:      "google",
			ClientID:      r.FormValue("oauth_client_id"),
			ClientSecret:  r.FormValue("oauth_client_secret"),
			RedirectURL:   siteURL + "/api/auth/callback",
			AllowedEmails: []string{r.FormValue("admin_email")},
			SessionKey:    generateSessionKey(),
		},
		Server: handlers.ServerConfig{
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
	db, err := services.InitDB(services.DBConfig{
		Driver:   cfg.Database.Driver,
		Path:     cfg.Database.Path,
		Host:     cfg.Database.Host,
		Port:     cfg.Database.Port,
		User:     cfg.Database.User,
		Password: cfg.Database.Password,
		DBName:   cfg.Database.DBName,
	})
	if err != nil {
		log.Printf("Failed to initialize database: %v", err)
		os.Remove(configPath) // Rollback
		http.Redirect(w, r, "/?error=Database+initialization+failed:+"+err.Error(), http.StatusFound)
		return
	}
	if db != nil {
		db.Close()
	}

	// Ensure admin path has leading slash
	if config.Server.AdminPath == "" {
		config.Server.AdminPath = "/admin"
	}
	if !strings.HasPrefix(config.Server.AdminPath, "/") {
		config.Server.AdminPath = "/" + config.Server.AdminPath
	}

	log.Println("Setup completed successfully")
	http.Redirect(w, r, config.Server.AdminPath, http.StatusFound)
}

func main() {
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
		log.Println("INFO: First run detected - starting setup wizard")
		http.HandleFunc("/", setupWizardHandler)
		http.HandleFunc("/setup/complete", completeSetupHandler)

		// Serve static files for setup page (theme first, then core)
		http.HandleFunc("/static/", func(w http.ResponseWriter, r *http.Request) {
			cleanPath := strings.TrimPrefix(r.URL.Path, "/static/")
			// Try theme static first
			if _, err := themeStaticFS.Open(cleanPath); err == nil {
				http.StripPrefix("/static/", http.FileServer(http.FS(themeStaticFS))).ServeHTTP(w, r)
				return
			}
			// Fall back to core static
			http.StripPrefix("/static/", http.FileServer(http.FS(coreStaticFS))).ServeHTTP(w, r)
		})

		port := os.Getenv("PORT")
		if port == "" {
			port = "8080"
		}
		fmt.Printf("Setup wizard running on :%s\n", port)
		log.Fatal(http.ListenAndServe(":"+port, nil))
		return
	}

	// Load config
	if err := loadConfig(); err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	authSvc := auth.Init(auth.Config{
		ClientID:      config.Auth.ClientID,
		ClientSecret:  config.Auth.ClientSecret,
		RedirectURL:   config.Auth.RedirectURL,
		AllowedEmails: config.Auth.AllowedEmails,
		SessionKey:    config.Auth.SessionKey,
		SessionKeyOld: config.Auth.SessionKeyOld,
	}, config.Server.AdminPath)

	db, err := services.InitDB(services.DBConfig{
		Driver:   config.Database.Driver,
		Path:     config.Database.Path,
		Host:     config.Database.Host,
		Port:     config.Database.Port,
		User:     config.Database.User,
		Password: config.Database.Password,
		DBName:   config.Database.DBName,
	})
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	defer db.Close()

	app := handlers.NewApp(embeddedFiles, coreStaticFS, themeStaticFS, db, authSvc, &config)
	mux := http.NewServeMux()
	app.RegisterRoutes(mux)

	port := config.Server.Port
	if port == "" {
		port = "8080"
	}
	host := config.Server.Host
	if host == "" {
		host = "0.0.0.0"
	}

	addr := fmt.Sprintf("%s:%s", host, port)
	fmt.Printf("G0 CMS running on %s\n", addr)
	log.Fatal(http.ListenAndServe(addr, mux))
}
