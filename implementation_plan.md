# Implementation Plan - Convert Porque to Wails Desktop Application

This plan outlines the steps, pre-requisites, and architectural changes required to convert the current web-based project (Go API + Worker + React + Postgres + Docker) into a single, standalone **Wails desktop application**.

## User Review Required

> [!WARNING]
> **Host Developer Environment Dependencies**
> Wails requires building native desktop code on your host machine. Because of this, you cannot develop or test a Wails app entirely inside a standard Docker container. Before executing this plan, **you must install the following on your Windows host machine**:
> 1. **Go (v1.21+)**: [Download Go](https://go.dev/dl/) and ensure `go version` works in PowerShell.
> 2. **Node.js**: (Already installed on your machine).
> 3. **C compiler (gcc)**: Wails requires CGO for native OS bindings. On Windows, install [MSYS2 / MinGW-w64](https://www.msys2.org/) and add `gcc` to your system environment variables.
> 4. **Wails CLI**: Install it on your host by running `go install github.com/wailsapp/wails/v2/cmd/wails@latest`.

---

## Open Questions

> [!IMPORTANT]
> **Docker vs. Native Java Orchestration**
> We have two options for managing the Minecraft servers in the Wails app:
> *   **Option A: Keep Docker Desktop Requirement**: The desktop app still communicates with Docker Desktop running on the user's machine (using `//./pipe/docker_engine` on Windows). The code stays very similar to what it is now, but requires Docker Desktop to be running.
> *   **Option B: Native Java Spawning (No Docker)**: We rewrite the server management logic to run `.jar` files directly on the host using Go's `os/exec` process management. We would also download and run native `playit` binaries directly. This is much lighter and doesn't require Docker at all, but requires the user to have Java installed (or we bundle a portable Java runtime).
> 
> **Which option would you prefer to move forward with?**

---

## Proposed Changes

---

### 1. Database Migration: Postgres to SQLite

#### [MODIFY] [store.go](file:///c:/Users/woopsy/Project/Random/Porque/internal/db/store.go)
- Swap PostgreSQL driver (`github.com/lib/pq`) with SQLite driver (`modernc.org/sqlite` or `github.com/mattn/go-sqlite3`).
- Re-write SQL queries to use SQLite `?` placeholders instead of Postgres `$1, $2` variables.
- Adapt database migrations to run on SQLite (SQLite supports most basic SQL statements, but `gen_random_uuid()` and some `CHECK` constraints are handled differently).

---

### 2. Backend Wails Bindings & Initialization

#### [NEW] [main.go](file:///c:/Users/woopsy/Project/Random/Porque/main.go)
- Create the core entry point for Wails:
  - Initialize the SQLite database connection.
  - Instantiate the `mcserver.Controller` and `playit.Manager`.
  - Configure Wails options (Window width/height, title, start menu, tray icon).
  - Bind the backend `App` struct to Wails.

#### [NEW] [app.go](file:///c:/Users/woopsy/Project/Random/Porque/app.go)
- Expose methods directly to the frontend. For example:
  ```go
  type App struct {
      ctx     context.Context
      store   *db.Store
      life    *mcserver.Controller
      tunnels *playit.Manager
  }
  
  func (a *App) ListServers() ([]db.Server, error) {
      return a.store.ListServers(a.ctx)
  }
  
  func (a *App) CreateServer(name string, serverType string, version string) (*db.Server, error) {
      // Calls controller.Create...
  }
  ```

---

### 3. Frontend Wails Bindings Integration

#### [MODIFY] [api.ts](file:///c:/Users/woopsy/Project/Random/Porque/web/src/lib/api.ts)
- Replace HTTP fetch calls (`fetch("/api/servers")`) with generated Wails bindings.
- When running `wails dev`, Wails generates a JavaScript module (`wailsjs/go/main/App`) representing all bound Go functions with TypeScript definitions automatically.
- Example update in `api.ts`:
  ```typescript
  import * as App from "../../wailsjs/go/main/App";
  
  export const api = {
    listServers: () => App.ListServers(),
    createServer: (input) => App.CreateServer(input.name, input.type, input.version),
  };
  ```

---

## Verification Plan

### Manual Verification
1. Run `wails doctor` on host machine to verify Go, gcc, and Wails requirements are satisfied.
2. Run `wails dev` to boot up the hot-reloading desktop application.
3. Test creating, starting, stopping, and importing Minecraft servers.
4. Verify database persistence in the local `.db` SQLite file.
5. Run `wails build` to bundle into a single production installer.
