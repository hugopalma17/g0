package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"log"
	"net/http"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"antigravity/internal/auth"
	"antigravity/internal/utils"

	"github.com/creack/pty"
	"github.com/gomarkdown/markdown"
	"github.com/gorilla/sessions"
	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin:     func(r *http.Request) bool { return true },
}

func (a *App) RegisterRoutes(mux *http.ServeMux) {
	// Static files - merged from theme and core
	// Theme assets: templates/themes/default/static/ (css, js for frontend, images)
	// Core assets: static/ (admin.js, monaco, xterm, etc.)
	mux.HandleFunc("/static/", func(w http.ResponseWriter, r *http.Request) {
		cleanPath := strings.TrimPrefix(r.URL.Path, "/static/")
		if cleanPath == "" || strings.HasSuffix(cleanPath, "/") {
			http.NotFound(w, r)
			return
		}

		// Determine which filesystem has the file
		var targetFS fs.FS
		if _, err := a.ThemeStaticFS.Open(cleanPath); err == nil {
			targetFS = a.ThemeStaticFS
		} else if _, err := a.CoreStaticFS.Open(cleanPath); err == nil {
			targetFS = a.CoreStaticFS
		} else {
			http.NotFound(w, r)
			return
		}

		// Protect admin assets (JS and Monaco editor) - these are in coreStaticFS
		if cleanPath == "js/admin.js" || strings.HasPrefix(cleanPath, "js/vs/") {
			if a.Auth == nil || a.Auth.Store == nil {
				http.NotFound(w, r)
				return
			}
			session, _ := a.Auth.Store.Get(r, "auth-session")
			if authVal, ok := session.Values["authenticated"].(bool); !ok || !authVal {
				http.NotFound(w, r)
				return
			}
		}

		http.StripPrefix("/static/", http.FileServer(http.FS(targetFS))).ServeHTTP(w, r)
	})

	// Auth routes
	if a.Auth != nil {
		mux.HandleFunc("/api/auth/login", a.Auth.LoginHandler)
		mux.HandleFunc("/api/auth/callback", a.Auth.CallbackHandler)
	} else {
		mux.HandleFunc("/api/auth/login", func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "Auth not configured", http.StatusInternalServerError)
		})
		mux.HandleFunc("/api/auth/callback", func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "Auth not configured", http.StatusInternalServerError)
		})
	}

	mux.HandleFunc("/api/auth/logout", func(w http.ResponseWriter, r *http.Request) {
		if a.Auth == nil || a.Auth.Store == nil {
			http.Redirect(w, r, "/", http.StatusFound)
			return
		}
		session, _ := a.Auth.Store.Get(r, "auth-session")
		session.Values["authenticated"] = false
		session.Save(r, w)
		http.Redirect(w, r, "/", http.StatusFound)
	})

	// Terminal
	mux.HandleFunc("/api/terminal", auth.WithAuth(a.authStore(), a.terminalHandler))

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
		if a.Config.Database.Driver == "postgres" {
			if idStr != "" {
				_, err = a.DB.Exec("UPDATE content SET type=$1, title=$2, date=$3, content=$4, metadata=$5 WHERE id=$6",
					cType, title, date, content, metadata, idStr)
			} else {
				_, err = a.DB.Exec("INSERT INTO content (type, title, date, content, metadata) VALUES ($1, $2, $3, $4, $5)",
					cType, title, date, content, metadata)
			}
		} else {
			if idStr != "" {
				_, err = a.DB.Exec("UPDATE content SET type=?, title=?, date=?, content=?, metadata=? WHERE id=?",
					cType, title, date, content, metadata, idStr)
			} else {
				_, err = a.DB.Exec("INSERT INTO content (type, title, date, content, metadata) VALUES (?, ?, ?, ?, ?)",
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

	mux.HandleFunc("/api/save", auth.WithAuth(a.authStore(), saveHandler))
	mux.HandleFunc("/api/posts", auth.WithAuth(a.authStore(), saveHandler))

	// Delete handler
	mux.HandleFunc("/api/delete", auth.WithAuth(a.authStore(), func(w http.ResponseWriter, r *http.Request) {
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
		if a.Config.Database.Driver == "postgres" {
			_, err = a.DB.Exec("DELETE FROM content WHERE id = $1", id)
		} else {
			_, err = a.DB.Exec("DELETE FROM content WHERE id = ?", id)
		}

		if err != nil {
			http.Error(w, "Database Error", http.StatusInternalServerError)
			return
		}
		w.Write([]byte("PROTOCOL_DELETED"))
	}))

	// Robots.txt
	mux.HandleFunc("/robots.txt", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.Write([]byte(fmt.Sprintf("User-agent: *\nAllow: /\nSitemap: %s/sitemap.xml", a.Config.Site.URL)))
	})

	// Favicon
	mux.HandleFunc("/favicon.ico", func(w http.ResponseWriter, r *http.Request) {
		data, err := a.EmbeddedFS.Open("static/favicon.ico")
		if err != nil {
			http.NotFound(w, r)
			return
		}
		defer data.Close()
		w.Header().Set("Content-Type", "image/x-icon")
		io.Copy(w, data)
	})

	// Sitemap
	mux.HandleFunc("/sitemap.xml", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")

		fmt.Fprint(w, `<?xml version="1.0" encoding="UTF-8"?>
<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">`)

		baseURL := a.Config.Site.URL
		staticPages := []string{"", "/journal", "/resume", "/contact"}
		for _, p := range staticPages {
			fmt.Fprintf(w, `
	<url>
		<loc>%s%s</loc>
		<changefreq>daily</changefreq>
		<priority>0.8</priority>
	</url>`, baseURL, p)
		}

		rows, err := a.DB.Query("SELECT title, date FROM content WHERE type='blog'")
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var title, dateStr string
				if err := rows.Scan(&title, &dateStr); err == nil {
					slug := utils.Slugify(title)
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

		rows, err := a.DB.Query("SELECT title, date, content, metadata FROM content WHERE type='blog' ORDER BY id DESC")
		if err != nil {
			http.Error(w, "Error generating feed", http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		baseURL := a.Config.Site.URL
		siteName := a.Config.Site.Name
		author := a.Config.Site.Author
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
  </author>`, siteName, a.Config.Site.Tagline, baseURL, baseURL, time.Now().Format(time.RFC3339), baseURL, author)

		for rows.Next() {
			var title, dateStr, content, metadataStr string
			if err := rows.Scan(&title, &dateStr, &content, &metadataStr); err == nil {
				slug := utils.Slugify(title)
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
					summary = utils.StripHTML(content, 200)
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

	mux.HandleFunc("/feed.atom", atomFeedHandler)
	mux.HandleFunc("/atom.xml", atomFeedHandler)
	mux.HandleFunc("/rss.xml", atomFeedHandler)

	// DB Manager APIs
	mux.HandleFunc("/api/db/table/", auth.WithAuth(a.authStore(), func(w http.ResponseWriter, r *http.Request) {
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

		rows, err := a.DB.QueryContext(ctx, query)
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

	mux.HandleFunc("/api/db/update", auth.WithAuth(a.authStore(), func(w http.ResponseWriter, r *http.Request) {
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
		if a.Config.Database.Driver == "postgres" {
			query := fmt.Sprintf("UPDATE %s SET %s = $1 WHERE id = $2", table, col)
			_, err = a.DB.ExecContext(ctx, query, val, id)
		} else {
			query := fmt.Sprintf("UPDATE %s SET %s = ? WHERE id = ?", table, col)
			_, err = a.DB.ExecContext(ctx, query, val, id)
		}

		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Write([]byte("PROTOCOL_UPDATED"))
	}))

	// Add new row to table
	mux.HandleFunc("/api/db/insert", auth.WithAuth(a.authStore(), func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		table := r.FormValue("table")
		validTables := map[string]bool{"content": true}
		if !validTables[table] {
			http.Error(w, "Invalid Table", http.StatusBadRequest)
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		var err error
		var result sql.Result
		if a.Config.Database.Driver == "postgres" {
			result, err = a.DB.ExecContext(ctx, "INSERT INTO content (type, title, date, content, metadata) VALUES ($1, $2, $3, $4, $5)",
				"blog", "New Entry", time.Now().Format("Jan 02, 2006"), "", "")
		} else {
			result, err = a.DB.ExecContext(ctx, "INSERT INTO content (type, title, date, content, metadata) VALUES (?, ?, ?, ?, ?)",
				"blog", "New Entry", time.Now().Format("Jan 02, 2006"), "", "")
		}

		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		id, _ := result.LastInsertId()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"id": id, "status": "ok"})
	}))

	// Delete row from table
	mux.HandleFunc("/api/db/delete-row", auth.WithAuth(a.authStore(), func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		table := r.FormValue("table")
		id := r.FormValue("id")

		validTables := map[string]bool{"content": true}
		if !validTables[table] {
			http.Error(w, "Invalid Table", http.StatusBadRequest)
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		var err error
		if a.Config.Database.Driver == "postgres" {
			_, err = a.DB.ExecContext(ctx, fmt.Sprintf("DELETE FROM %s WHERE id = $1", table), id)
		} else {
			_, err = a.DB.ExecContext(ctx, fmt.Sprintf("DELETE FROM %s WHERE id = ?", table), id)
		}

		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Write([]byte(`{"status": "ok"}`))
	}))

	// Add column to table
	mux.HandleFunc("/api/db/add-column", auth.WithAuth(a.authStore(), func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		table := r.FormValue("table")
		colName := r.FormValue("column")
		colType := r.FormValue("type")

		validTables := map[string]bool{"content": true}
		if !validTables[table] {
			http.Error(w, "Invalid Table", http.StatusBadRequest)
			return
		}

		// Validate column name (alphanumeric and underscore only)
		isAlphaNum := regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9_]*$`).MatchString
		if !isAlphaNum(colName) {
			http.Error(w, "Invalid column name (use letters, numbers, underscore; must start with letter)", http.StatusBadRequest)
			return
		}

		// Map simple types to SQL types
		sqlType := "TEXT"
		switch strings.ToLower(colType) {
		case "text", "string":
			sqlType = "TEXT"
		case "int", "integer", "number":
			sqlType = "INTEGER"
		case "bool", "boolean":
			if a.Config.Database.Driver == "postgres" {
				sqlType = "BOOLEAN"
			} else {
				sqlType = "INTEGER" // SQLite uses 0/1 for bool
			}
		case "date", "datetime":
			sqlType = "TEXT" // Store as ISO string for compatibility
		}

		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		query := fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s", table, colName, sqlType)
		_, err := a.DB.ExecContext(ctx, query)

		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Write([]byte(`{"status": "ok"}`))
	}))

	// Get table schema (columns info)
	mux.HandleFunc("/api/db/schema/", auth.WithAuth(a.authStore(), func(w http.ResponseWriter, r *http.Request) {
		tableName := strings.TrimPrefix(r.URL.Path, "/api/db/schema/")
		validTables := map[string]bool{"content": true}
		if !validTables[tableName] {
			http.Error(w, "Invalid Table", http.StatusBadRequest)
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		var rows *sql.Rows
		var err error

		if a.Config.Database.Driver == "postgres" {
			rows, err = a.DB.QueryContext(ctx, `
				SELECT column_name, data_type, is_nullable
				FROM information_schema.columns
				WHERE table_name = $1
				ORDER BY ordinal_position`, tableName)
		} else {
			rows, err = a.DB.QueryContext(ctx, fmt.Sprintf("PRAGMA table_info(%s)", tableName))
		}

		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		var columns []map[string]interface{}

		if a.Config.Database.Driver == "postgres" {
			for rows.Next() {
				var name, dataType, nullable string
				if err := rows.Scan(&name, &dataType, &nullable); err == nil {
					columns = append(columns, map[string]interface{}{
						"name":     name,
						"type":     dataType,
						"nullable": nullable == "YES",
					})
				}
			}
		} else {
			for rows.Next() {
				var cid int
				var name, dataType string
				var notNull, dfltValue, pk interface{}
				if err := rows.Scan(&cid, &name, &dataType, &notNull, &dfltValue, &pk); err == nil {
					columns = append(columns, map[string]interface{}{
						"name":     name,
						"type":     dataType,
						"nullable": notNull == int64(0),
					})
				}
			}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(columns)
	}))

	// Execute raw SQL (for power users)
	mux.HandleFunc("/api/db/exec", auth.WithAuth(a.authStore(), func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		query := r.FormValue("query")
		if query == "" {
			http.Error(w, "Query required", http.StatusBadRequest)
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()

		// Determine if it's a SELECT query
		trimmedQuery := strings.TrimSpace(strings.ToUpper(query))
		isSelect := strings.HasPrefix(trimmedQuery, "SELECT") || strings.HasPrefix(trimmedQuery, "PRAGMA")

		if isSelect {
			rows, err := a.DB.QueryContext(ctx, query)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
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
			json.NewEncoder(w).Encode(map[string]interface{}{
				"type": "select",
				"rows": result,
				"cols": cols,
			})
		} else {
			result, err := a.DB.ExecContext(ctx, query)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}

			affected, _ := result.RowsAffected()
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"type":         "exec",
				"rowsAffected": affected,
			})
		}
	}))

	// Admin panel - path configurable via config.yaml
	mux.HandleFunc(a.Config.Server.AdminPath, auth.WithAuth(a.authStore(), func(w http.ResponseWriter, r *http.Request) {
		filterTypes := r.URL.Query().Get("types")
		var postRows *sql.Rows
		var err error

		if filterTypes != "" && filterTypes != "all" {
			types := strings.Split(filterTypes, ",")
			if a.Config.Database.Driver == "postgres" {
				placeholders := make([]string, len(types))
				args := make([]interface{}, len(types))
				for i, t := range types {
					placeholders[i] = fmt.Sprintf("$%d", i+1)
					args[i] = t
				}
				query := fmt.Sprintf("SELECT id, type, title, date, content, metadata FROM content WHERE type IN (%s) ORDER BY id DESC", strings.Join(placeholders, ","))
				postRows, err = a.DB.Query(query, args...)
			} else {
				placeholders := make([]string, len(types))
				args := make([]interface{}, len(types))
				for i, t := range types {
					placeholders[i] = "?"
					args[i] = t
				}
				query := fmt.Sprintf("SELECT id, type, title, date, content, metadata FROM content WHERE type IN (%s) ORDER BY id DESC", strings.Join(placeholders, ","))
				postRows, err = a.DB.Query(query, args...)
			}
		} else {
			postRows, err = a.DB.Query("SELECT id, type, title, date, content, metadata FROM content ORDER BY id DESC LIMIT 100")
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

		tmpl, err := template.ParseFS(a.EmbeddedFS, "templates/admin.html")
		if err != nil {
			http.Error(w, fmt.Sprintf("Template Error: %v", err), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "text/html")
		tmpl.Execute(w, AdminData{
			Title:       "G0 CMS Admin",
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
		tmpl, err := template.ParseFS(a.EmbeddedFS, "templates/themes/default/404.html")
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

		rows, _ := a.DB.Query("SELECT id, type, title, date, content, metadata FROM content WHERE type='blog' ORDER BY id DESC")
		defer rows.Close()

		var blogs []ContentEntry
		var activeBlog *ContentEntry
		for rows.Next() {
			var c ContentEntry
			rows.Scan(&c.ID, &c.Type, &c.Title, &c.Date, &c.Content, &c.Metadata)
			c.Slug = utils.Slugify(c.Title)
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
		tmpl, err := tmpl.ParseFS(a.EmbeddedFS, "templates/themes/default/index.html", "templates/themes/default/journal.html")
		if err != nil {
			http.Error(w, "Template Parse Error", http.StatusInternalServerError)
			return
		}

		var seoMeta SEOMetadata
		seoMeta.Image = a.Config.Site.URL + "/static/images/og-image.jpg"

		pageTitle := "Journal | " + a.Config.Site.Name
		if activeBlog.Metadata != "" {
			json.Unmarshal([]byte(activeBlog.Metadata), &seoMeta)
			if seoMeta.Title != "" {
				pageTitle = seoMeta.Title
			} else if activeBlog.Title != "" {
				pageTitle = activeBlog.Title + " | " + a.Config.Site.Name
			}
		}

		if seoMeta.Image == "" {
			seoMeta.Image = a.Config.Site.URL + "/static/images/og-image.jpg"
		}

		data := map[string]interface{}{
			"Title":       pageTitle,
			"Blogs":       blogs,
			"ActiveBlog":  activeBlog,
			"ActiveSlug":  activeBlog.Slug,
			"View":        "journal",
			"Theme":       getTheme(r),
			"SeoDesc":     seoMeta.Description,
			"SeoImage":    seoMeta.Image,
			"SeoKeywords": seoMeta.Keywords,
			"AdminPath":   a.Config.Server.AdminPath,
			"Site":        a.Config.Site,
			"Year":        time.Now().Year(),
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

	mux.HandleFunc("/journal", journalHandler)
	mux.HandleFunc("/journal/", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/") && r.URL.Path != "/journal/" {
			http.Redirect(w, r, strings.TrimSuffix(r.URL.Path, "/"), http.StatusMovedPermanently)
			return
		}
		journalHandler(w, r)
	})

	// Root handler
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" && strings.HasSuffix(r.URL.Path, "/") {
			http.Redirect(w, r, strings.TrimSuffix(r.URL.Path, "/"), http.StatusMovedPermanently)
			return
		}

		path := strings.TrimPrefix(r.URL.Path, "/")

		rows, err := a.DB.Query("SELECT id, type, title, date, content, metadata FROM content ORDER BY id DESC")
		if err != nil {
			http.Error(w, "Database Error", http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		var entries []ContentEntry
		for rows.Next() {
			var c ContentEntry
			rows.Scan(&c.ID, &c.Type, &c.Title, &c.Date, &c.Content, &c.Metadata)
			c.Slug = utils.Slugify(c.Title)
			entries = append(entries, c)
		}

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
		pageTitle := a.Config.Site.Name + " | " + a.Config.Site.Tagline
		var matchedEntry *ContentEntry
		targetTemplate := "templates/themes/default/home.html"

		switch view {
		case "home":
			targetTemplate = "templates/themes/default/home.html"
			for i := range entries {
				if entries[i].Type == "home" {
					matchedEntry = &entries[i]
					break
				}
			}
		case "resume":
			targetTemplate = "templates/themes/default/resume.html"
			for i := range entries {
				if entries[i].Type == "resume" {
					matchedEntry = &entries[i]
					break
				}
			}
		case "contact":
			targetTemplate = "templates/themes/default/contact.html"
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
				pageTitle = matchedEntry.Title + " | " + a.Config.Site.Name
			}
		}

		if seoMeta.Image == "" {
			seoMeta.Image = a.Config.Site.URL + "/static/images/og-image.jpg"
		}

		type ExtendedPageData struct {
			PageData
			Content template.HTML
		}

		pd := ExtendedPageData{
			PageData: PageData{
				Title:       pageTitle,
				View:        view,
				ActiveSlug:  activeSlug,
				Theme:       getTheme(r),
				SeoDesc:     seoMeta.Description,
				SeoImage:    seoMeta.Image,
				SeoKeywords: seoMeta.Keywords,
				AdminPath:   a.Config.Server.AdminPath,
				Site:        a.Config.Site,
				Year:        time.Now().Year(),
			},
			Content: "",
		}

		if matchedEntry != nil {
			contentStr := matchedEntry.Content
			if !strings.HasPrefix(strings.TrimSpace(contentStr), "<") {
				output := markdown.ToHTML([]byte(contentStr), nil, nil)
				pd.Content = template.HTML(output)
			} else {
				pd.Content = template.HTML(contentStr)
			}
		}

		tmpl := template.New("index.html")
		tmpl, err = tmpl.ParseFS(a.EmbeddedFS, "templates/themes/default/index.html", targetTemplate)
		if err != nil {
			tmpl, _ = tmpl.ParseFS(a.EmbeddedFS, "templates/themes/default/index.html", "templates/themes/default/home.html")
		}

		w.Header().Set("Content-Type", "text/html")
		if tmpl != nil {
			tmpl.Execute(w, pd)
		} else {
			http.Error(w, "Template not found", http.StatusInternalServerError)
		}
	})
}

func (a *App) authStore() *sessions.CookieStore {
	if a.Auth == nil {
		return nil
	}
	return a.Auth.Store
}

func (a *App) terminalHandler(w http.ResponseWriter, r *http.Request) {
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

func getTheme(r *http.Request) string {
	cookie, err := r.Cookie("theme")
	if err != nil || cookie.Value == "" {
		return "dark"
	}
	return cookie.Value
}
