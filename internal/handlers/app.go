package handlers

import (
	"database/sql"
	"io/fs"

	"antigravity/internal/auth"
)

type App struct {
	EmbeddedFS    fs.FS
	CoreStaticFS  fs.FS
	ThemeStaticFS fs.FS
	DB            *sql.DB
	Auth          *auth.Auth
	Config        *Config
}

func NewApp(embeddedFS fs.FS, coreStaticFS fs.FS, themeStaticFS fs.FS, db *sql.DB, authSvc *auth.Auth, cfg *Config) *App {
	return &App{
		EmbeddedFS:    embeddedFS,
		CoreStaticFS:  coreStaticFS,
		ThemeStaticFS: themeStaticFS,
		DB:            db,
		Auth:          authSvc,
		Config:        cfg,
	}
}
