# g0 Helper Daemon - Architecture Draft

## Overview

The g0 CMS uses a two-process architecture for security:

| Component | Role | Network | Privileges |
|-----------|------|---------|------------|
| **main.go** (g0) | Web server | HTTP exposed | Read-only, serves from RAM |
| **helper** | System daemon | Unix socket only | Write access, manages config |

## Why This Architecture?

1. **Attack surface reduction** - Web-facing process has no write access
2. **Privilege separation** - Helper runs as different user with specific permissions
3. **Memory-first** - Static assets embedded and served from RAM (no disk I/O for requests)
4. **Clear boundaries** - Web server can't modify config, database schema, or system state

## Component Responsibilities

### main.go (Web Server)
- Serve HTTP/HTTPS requests
- Embed and serve static assets from memory
- Read from database (content queries)
- Handle OAuth authentication flow
- Render templates
- **NO write access to**: config.yaml, system files, environment

### helper (System Daemon)
- Manage environment variables (export at boot)
- Write to config.yaml
- Database migrations and schema changes
- Backup/restore operations
- Log rotation
- Certificate management (Let's Encrypt)
- Health monitoring
- Restart/reload web server
- **NOT exposed to network**

## Communication

```
┌─────────────┐     Unix Socket      ┌─────────────┐
│   main.go   │ ◄──────────────────► │   helper    │
│  (g0 user)  │   /var/run/g0.sock   │ (g0-admin)  │
└─────────────┘                      └─────────────┘
       │                                    │
       ▼                                    ▼
   HTTP :8080                         System access
   (public)                           (no network)
```

## User/Group Structure

```bash
# Users
g0          # Web server process (minimal privileges)
g0-admin    # Helper daemon (system management)

# Groups
g0-web      # Can read static assets, templates
g0-data     # Can read database
g0-config   # Can read config.yaml
g0-system   # Can write config, manage services (helper only)
```

## File Permissions

```
/opt/g0/
├── g0                      # Binary (755, root:root)
├── helper                  # Binary (750, root:g0-admin)
├── templates/              # Embedded at compile, but also:
│   └── default/            # (755, root:g0-web)
├── static/                 # (755, root:g0-web)
└── data/
    ├── config.yaml         # (640, g0-admin:g0-config)
    └── g0.db               # (660, g0-admin:g0-data)
```

## Boot Sequence

```
1. systemd starts helper.service
2. helper reads config.yaml
3. helper exports environment variables to /run/g0/env
4. helper starts g0.service (web server)
5. g0 inherits environment, loads config, serves HTTP
6. helper monitors health via Unix socket
```

## Environment Variable Flow

```
config.yaml
    │
    ▼
helper reads config
    │
    ▼
helper writes /run/g0/env
    │
    ▼
systemd EnvironmentFile=/run/g0/env
    │
    ▼
g0 process inherits env vars
```

## Helper Commands (IPC via Unix Socket)

| Command | Description |
|---------|-------------|
| `reload` | Reload config without restart |
| `restart` | Graceful restart of web server |
| `backup` | Create database backup |
| `migrate` | Run database migrations |
| `cert-renew` | Renew SSL certificates |
| `status` | Health check response |
| `env-set KEY=VAL` | Update environment variable |
| `config-set key.path=val` | Update config.yaml value |

## Systemd Units

### /etc/systemd/system/g0-helper.service
```ini
[Unit]
Description=g0 CMS Helper Daemon
Before=g0.service

[Service]
Type=notify
User=g0-admin
Group=g0-system
ExecStart=/opt/g0/helper
ExecReload=/bin/kill -HUP $MAINPID
Restart=always
RuntimeDirectory=g0
RuntimeDirectoryMode=0750

[Install]
WantedBy=multi-user.target
```

### /etc/systemd/system/g0.service
```ini
[Unit]
Description=g0 CMS Web Server
After=g0-helper.service
Requires=g0-helper.service

[Service]
Type=simple
User=g0
Group=g0-web
EnvironmentFile=/run/g0/env
ExecStart=/opt/g0/g0
Restart=always
ReadOnlyPaths=/
ReadWritePaths=/var/log/g0

[Install]
WantedBy=multi-user.target
```

## Implementation Phases

### Phase 1: Environment Management
- [ ] Helper reads config.yaml on startup
- [ ] Helper writes /run/g0/env file with exported vars
- [ ] Helper creates Unix socket at /run/g0/helper.sock
- [ ] Helper signals ready (systemd notify)
- [ ] main.go works with env vars (already done - removed godotenv)

### Phase 2: IPC Protocol
- [ ] Define message format (JSON over Unix socket)
- [ ] Implement basic commands (status, reload, restart)
- [ ] Authentication between processes (file-based shared secret)
- [ ] Timeout and error handling

### Phase 3: Config Management
- [ ] `config-set` command to update config.yaml
- [ ] `config-get` command to read values
- [ ] Validation before writing
- [ ] Atomic writes (write to tmp, rename)

### Phase 4: Database Operations
- [ ] `backup` command - creates timestamped backup
- [ ] `restore` command - restores from backup
- [ ] `migrate` command - runs pending migrations
- [ ] Backup rotation (keep N most recent)

### Phase 5: System Management
- [ ] Log rotation integration
- [ ] Certificate renewal hooks (certbot)
- [ ] Health check endpoint for monitoring
- [ ] Metrics export (optional prometheus)

## Security Considerations

1. **Unix socket permissions** - 0660, only g0 and g0-admin can access
2. **No network exposure** - Helper never binds to TCP/UDP
3. **Capability dropping** - Helper drops unnecessary Linux capabilities after startup
4. **Seccomp filtering** - Restrict system calls for both processes
5. **Read-only root** - Web server runs with read-only filesystem view
6. **No shell** - Neither process spawns shells (no exec vulnerabilities)
7. **Separate secrets** - Each process only knows what it needs

## Config-to-Env Mapping

```yaml
# config.yaml
site:
  name: "My Site"
  url: "https://example.com"
auth:
  client_id: "xxx"
  client_secret: "yyy"
database:
  driver: "sqlite"
  path: "./data/g0.db"
server:
  port: "8080"
```

```bash
# /run/g0/env (generated by helper)
G0_SITE_NAME="My Site"
G0_SITE_URL="https://example.com"
G0_AUTH_CLIENT_ID="xxx"
G0_AUTH_CLIENT_SECRET="yyy"
G0_DATABASE_DRIVER="sqlite"
G0_DATABASE_PATH="./data/g0.db"
G0_SERVER_PORT="8080"
```

## Data Access Model (DECIDED)

### Web Server (main.go) Access

**READ (required for runtime):**
- `content` table - slug-based routing, page rendering
- `config.yaml` - site settings, OAuth config, server settings
- Templates/theme - embedded at compile, or external theme directory

**WRITE (admin panel operations):**
- `content` table only - INSERT, UPDATE, DELETE for blog posts, pages
- Session cookies (not DB)

**NO ACCESS:**
- Schema modifications (ALTER, CREATE, DROP)
- config.yaml writes
- System files
- Other tables (future: users, analytics, etc.)

### Helper Access

**FULL WRITE:**
- `config.yaml` - all modifications
- Database schema - migrations, ALTER TABLE, CREATE TABLE
- Environment files - /run/g0/env
- Backups - database dumps
- Future tables (users, settings, etc.)

### Why This Split?

1. **Slug routing requires DB reads** - `/journal/my-post-slug` must query content table
2. **Admin CRUD must be responsive** - can't proxy every save through Unix socket
3. **Content is user data, not system config** - different threat model
4. **Schema changes are rare and dangerous** - helper-only makes sense

### Enforcement

SQLite doesn't have table-level permissions, so enforcement is at application layer:
- main.go only has functions for content CRUD
- No raw SQL execution in web server (removed /api/db/exec for production)
- Helper owns all migration code

---

## Setup Wizard (DECIDED)

The setup wizard cannot live in main.go under this architecture (main.go shouldn't write config).

**Solution: Wizard is part of helper**

```
First Run Flow:
1. User runs: ./helper --setup
2. Helper serves temporary HTTP on :8080
3. User completes wizard form
4. Helper writes config.yaml
5. Helper writes /run/g0/env
6. Helper exits setup mode
7. systemd starts both services normally
```

This keeps all write operations in helper, even during setup.

---

## Open Questions

- [ ] Helper crashes - what happens to web server?
  - Option A: Web server continues with stale config (current leaning)
  - Option B: Web server terminates (Requires=helper)
  - Option C: Web server enters degraded mode

- [ ] Multi-node deployment
  - Option A: Helper per node, shared database
  - Option B: Centralized helper, distributed web servers
  - Option C: Each node fully independent

- [ ] Container deployment
  - Option A: Single container with supervisor (s6, tini)
  - Option B: Separate containers, shared volume for socket
  - Option C: Kubernetes sidecar pattern

- [ ] External themes
  - Option A: Themes must be embedded at compile time
  - Option B: Helper can hot-reload themes, signals web server
  - Option C: Web server watches theme directory

## Directory Structure (Future)

```
/opt/g0/
├── bin/
│   ├── g0              # Web server binary
│   └── helper          # Helper daemon binary
├── etc/
│   └── config.yaml     # Configuration
├── var/
│   ├── db/
│   │   └── g0.db       # SQLite database
│   ├── log/
│   │   ├── access.log
│   │   └── error.log
│   └── backup/
│       └── *.sql.gz    # Database backups
└── share/
    └── templates/      # Optional external themes
```

## References

- systemd service hardening: https://www.freedesktop.org/software/systemd/man/systemd.exec.html
- Unix socket security: https://man7.org/linux/man-pages/man7/unix.7.html
- Go Unix socket IPC: https://pkg.go.dev/net#UnixConn
- Privilege separation patterns: OpenSSH, Postfix
