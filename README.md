# G0 CMS

> A lightweight, single-binary content management system. Because 25MB is enough.

## Philosophy

G0 CMS is built on a simple premise: **modern CMS solutions are bloated.**

WordPress requires PHP, MySQL, and prayer. Ghost needs Node.js and 500MB of npm packages. Even "lightweight" solutions demand Docker orchestration.

G0 rejects this. **One binary. One config file. One database. Done.**

## Features

- **Single Binary** - Everything embedded at compile time (~27MB)
- **Zero Dependencies** - No PHP, no Node.js, no containers required
- **SQLite or PostgreSQL** - SQLite for simplicity, Postgres for scale
- **Google OAuth** - Secure admin access, no password management
- **Setup Wizard** - First-run configuration UI, no manual config editing
- **SEO Built-in** - Sitemap, Atom feed, Open Graph tags
- **Dark/Light Theme** - User preference respected
- **Markdown + HTML** - Write content your way

## Quick Start

```bash
# Download the binary (or build from source)
./g0

# Visit http://localhost:8080
# Complete the setup wizard
# Start creating content
```

## Building from Source

```bash
git clone https://github.com/hugopalma17/g0
cd g0
go build -o g0 .
./g0
```

Requires Go 1.22+

## Google OAuth Setup

G0 uses Google OAuth for admin authentication. You need to create your own credentials:

### Step 1: Create a Google Cloud Project

1. Go to [Google Cloud Console](https://console.cloud.google.com/)
2. Create a new project (or select existing)
3. Note your project name

### Step 2: Configure OAuth Consent Screen

1. Navigate to **APIs & Services → OAuth consent screen**
2. Select **External** (unless you have Google Workspace)
3. Fill in required fields:
   - App name: `Your Site Name`
   - User support email: `your@email.com`
   - Developer contact: `your@email.com`
4. Click **Save and Continue**
5. Skip Scopes (we only need basic profile)
6. Add your email as a **Test user** (required while in testing mode)
7. Save

### Step 3: Create OAuth Credentials

1. Navigate to **APIs & Services → Credentials**
2. Click **+ CREATE CREDENTIALS → OAuth client ID**
3. Application type: **Web application**
4. Name: `G0 CMS`
5. **Authorized redirect URIs** - Add your callback URL:
   ```
   https://yourdomain.com/api/auth/callback
   ```
   For local development:
   ```
   http://localhost:8080/api/auth/callback
   ```
6. Click **Create**
7. Copy your **Client ID** and **Client Secret**

### Step 4: Enter Credentials in Setup Wizard

When you run G0 for the first time, enter:
- **Admin Email**: Must match a Google account (the one you added as test user)
- **Client ID**: `xxxx.apps.googleusercontent.com`
- **Client Secret**: `GOCSPX-xxxx`

> **Important**: Your redirect URI in Google Console must exactly match your site URL + `/api/auth/callback`

### Publishing Your App (Optional)

While in "Testing" mode, only emails you've added as test users can log in. To allow any email in your `allowed_emails` config:

1. Go to **OAuth consent screen**
2. Click **PUBLISH APP**
3. Complete verification if required (for public apps)

For personal/private sites, keeping it in Testing mode and adding your email as a test user is sufficient.

---

## Configuration

On first run, G0 presents a setup wizard. All persistent data is stored in `./data/`:

```
data/
├── config.yaml    # Configuration (created by wizard)
└── g0.db          # SQLite database
```

Configuration example (`data/config.yaml`):

```yaml
site:
  name: "My Site"
  url: "https://example.com"
  tagline: "A site built with G0"
  author: "Your Name"

database:
  driver: "sqlite"      # or "postgres"
  path: "./data/g0.db"  # SQLite path

auth:
  provider: "google"
  client_id: "xxx.apps.googleusercontent.com"
  client_secret: "xxx"
  redirect_url: "https://example.com/api/auth/callback"
  allowed_emails:
    - "admin@example.com"
  session_key: ""    # Auto-generated if empty

server:
  host: "0.0.0.0"
  port: "8080"
  admin_path: "/admin"                  # Custom admin URL
```

## Routes

| Route | Description |
|-------|-------------|
| `/` | Homepage |
| `/journal` | Blog listing |
| `/journal/{slug}` | Blog post |
| `/resume` | Resume page |
| `/contact` | Contact form |
| `/admin` (customizable) | Admin panel (OAuth protected) |
| `/sitemap.xml` | Dynamic sitemap |
| `/feed.atom` | Atom feed |

## Admin Panel

Access your admin panel (default `/admin`, customizable in setup) to:

- Create/edit/delete content
- Manage blog posts, pages, and sections
- Edit SEO metadata
- Access database manager
- Use built-in terminal (for server access)

## Content Types

| Type | Purpose |
|------|---------|
| `blog` | Journal/blog entries |
| `home` | Homepage content |
| `resume` | Resume page |
| `contact` | Contact page content |

## Deployment

### Option 1: Direct Binary

```bash
./g0
# Runs on :8080 by default
```

### Option 2: Systemd Service

```ini
[Unit]
Description=G0 CMS
After=network.target

[Service]
Type=simple
User=www-data
WorkingDirectory=/opt/g0
ExecStart=/opt/g0/g0
Restart=always

[Install]
WantedBy=multi-user.target
```

### Option 3: Docker

```bash
docker build -t g0 .
docker run -p 8080:8080 -v $(pwd)/data:/app/data g0
```

### Reverse Proxy (Caddy)

```
example.com {
    reverse_proxy localhost:8080
}
```

## Stack

| Layer | Technology |
|-------|------------|
| Language | Go |
| Database | SQLite / PostgreSQL |
| Templates | Go html/template |
| Interactivity | htmx + Vanilla JS |
| Styling | Tailwind (compiled) |
| Auth | Google OAuth OIDC |

## Development Status

### Completed
- [x] Core CMS functionality
- [x] Setup wizard
- [x] SQLite/PostgreSQL support
- [x] Google OAuth admin
- [x] SEO (sitemap, feeds, OG tags)
- [x] Dark/light theme

### In Progress
- [ ] Admin panel editor rewrite (proper WYSIWYG + iframe preview)
- [ ] Theme system (runtime theme loading)
- [ ] Plugin architecture

### Planned
- [ ] Image upload/management
- [ ] Multiple admin users
- [ ] Content versioning
- [ ] One-click deploy templates

## Project Structure

```
g0/
├── internal/
│   ├── auth/         # OAuth + session auth
│   ├── handlers/     # HTTP routes + handlers
│   ├── middleware/   # Host/track placeholders
│   ├── services/     # DB + services
│   └── utils/        # Helpers
├── main.go           # Core server
├── config.yaml       # Runtime config (created by wizard)
├── go.mod
├── go.sum
├── static/
│   ├── css/          # Stylesheets
│   ├── js/           # JavaScript
│   ├── fonts/        # Web fonts
│   └── images/       # Static images
├── templates/
│   ├── admin.html          # Admin panel
│   └── themes/
│       └── default/
│           ├── index.html  # Base layout
│           ├── home.html   # Homepage
│           ├── journal.html # Blog template
│           ├── resume.html # Resume template
│           ├── contact.html # Contact template
│           ├── 404.html    # Error page
│           └── static/     # Theme assets (css, js, images)
└── data/
    └── g0.db         # SQLite database (runtime)
```

## Template Tutorial (Index + Body)

G0 renders every page through `templates/themes/default/index.html`, which defines a `{{block "content" .}}` section. Each page template only needs to define that block.

1. Create a new template, for example `templates/themes/default/about.html`:

```html
{{define "content"}}
<section class="max-w-4xl">
  <div class="prose max-w-none">
    <h2>About</h2>
    <p>Write your content here.</p>
  </div>
</section>
{{end}}
```

1. Add a route in `internal/handlers/routes.go` by copying the `home`/`resume`/`contact` cases and pointing to your new template.
1. Create content in the admin panel with the matching type (for example `about`).

The base layout, navigation, and footer stay in `index.html`; your page body lives in its own template.

## Custom Page Guide

Use this workflow to add a new page type end-to-end. The base layout lives in `templates/themes/default/index.html`, and each page only provides the `{{define "content"}}` block.

1. Create the page body in `templates/themes/default/<page>.html`:

```html
{{define "content"}}
<section class="max-w-4xl">
  {{if .Content}}
  <div class="prose max-w-none">
    {{.Content}}
  </div>
  {{else}}
  <div class="prose max-w-none">
    <h2>About</h2>
    <p>Write your content here.</p>
  </div>
  {{end}}
</section>
{{end}}
```

1. Register the route in `internal/handlers/routes.go` inside the `switch view` block:

```go
case "about":
    targetTemplate = "templates/themes/default/about.html"
    for i := range entries {
        if entries[i].Type == "about" {
            matchedEntry = &entries[i]
            break
        }
    }
```

1. Create content in the admin panel with `type = about`. That content becomes `{{.Content}}` on the page.

1. If the page should appear in the navigation, add a link in `templates/themes/default/index.html`.

1. If you want SEO tags for this page, add JSON metadata in the admin panel (Metadata field). The layout reads it and fills `{{.SeoDesc}}`, `{{.SeoImage}}`, and `{{.SeoKeywords}}`.

### Template Data You Can Use

- `{{.Content}}` HTML content for the page
- `{{.Title}}` page title
- `{{.Site.Name}}`, `{{.Site.Tagline}}`, `{{.Site.Author}}`
- `{{.SeoDesc}}`, `{{.SeoImage}}`, `{{.SeoKeywords}}`
- `{{.AdminPath}}`, `{{.Theme}}`, `{{.Year}}`

### SEO Tags (per page)

Add JSON metadata in the admin panel (Metadata field):

```json
{"title": "About | My Site", "description": "Short summary", "og_image": "/static/images/og-image.jpg", "keywords": "about, company"}
```

How it works:
- The JSON above is stored in the database in the `content.metadata` column for that page.
- On render, the server parses that JSON and maps it to `Title`, `SeoDesc`, `SeoImage`, and `SeoKeywords` for the template.
- `og:title` uses `metadata.title` if present, otherwise the entry title, otherwise the site name.
- `og:description` uses `metadata.description` if present, otherwise `site.description` from `config.yaml`.
- `og:image` uses `metadata.og_image` if present, otherwise `site.url + /static/images/og-image.jpg`. Set `site.url` if you want absolute OG URLs.

Optional auto-generation:
- If you add a metadata generation script such as `generatemetadate.js`, you must install and configure a provider (Copilot, Gemini, or similar) before running it. This repo does not ship that dependency by default.

## Security

- OAuth tokens never stored, session-based auth
- Config file written with 0600 permissions
- Admin JS files protected behind auth
- No directory listing on static files
- SQL injection prevention via parameterized queries

## Troubleshooting

### "redirect_uri_mismatch" Error
Your callback URL in Google Console doesn't match your site. Ensure:
- Google Console has: `https://yourdomain.com/api/auth/callback`
- Matches exactly (http vs https, trailing slashes matter)

### "Access blocked: App not verified"
Your OAuth app is in testing mode. Either:
1. Add your email as a test user in Google Console
2. Or publish the app (requires verification for public use)

### "Unauthorized: Email not allowed"
The Google account you logged in with isn't in the `allowed_emails` list in `config.yaml`.

### Database Errors on Startup
- **SQLite**: Ensure the `data/` directory is writable
- **PostgreSQL**: Check connection details and that the database exists

---

## License

MIT

---

*G0 CMS: What if your CMS respected your server?*
