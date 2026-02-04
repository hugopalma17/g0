package services

import (
	"database/sql"
	"fmt"
	"log"
)

type DBConfig struct {
	Driver   string
	Path     string
	Host     string
	Port     string
	User     string
	Password string
	DBName   string
}

func InitDB(cfg DBConfig) (*sql.DB, error) {
	var (
		db  *sql.DB
		err error
	)

	if cfg.Driver == "postgres" {
		connStr := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
			cfg.Host, cfg.Port, cfg.User, cfg.Password, cfg.DBName)
		db, err = sql.Open("postgres", connStr)
	} else {
		// SQLite
		db, err = sql.Open("sqlite", cfg.Path)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	if err := runMigrations(db, cfg.Driver); err != nil {
		return nil, err
	}

	return db, nil
}

func runMigrations(db *sql.DB, driver string) error {
	var sqlStmt string

	if driver == "postgres" {
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

	log.Println("Database migrations completed")
	return nil
}
