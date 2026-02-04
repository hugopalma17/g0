package handlers

import "html/template"

// SiteConfig holds site-level configuration
// (exported for use in templates and setup).
type SiteConfig struct {
	Name        string `yaml:"name"`
	URL         string `yaml:"url"`
	Tagline     string `yaml:"tagline"`
	Description string `yaml:"description"`
	Author      string `yaml:"author"`
}

// DatabaseConfig holds database configuration
type DatabaseConfig struct {
	Driver   string `yaml:"driver"`
	Path     string `yaml:"path"`
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
	AdminPath string `yaml:"admin_path"`
}

// Config is the main configuration structure
type Config struct {
	Site     SiteConfig     `yaml:"site"`
	Database DatabaseConfig `yaml:"database"`
	Auth     AuthConfig     `yaml:"auth"`
	Server   ServerConfig   `yaml:"server"`
}

// ContentEntry represents a row in the content table.
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

type PageData struct {
	Title       string
	View        string
	ActiveSlug  string
	Theme       string
	SeoDesc     string
	SeoImage    string
	SeoKeywords string
	AdminPath   string
	Site        SiteConfig
	Year        int
}

type AdminData struct {
	Title       string
	View        string
	Posts       []ContentEntry
	CurrentType string
	PostsJS     template.JS
	Theme       string
}
