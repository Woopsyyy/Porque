# Implementation Plan - Server Crash Event Logging & Cleanup

This plan outlines the steps, database schema changes, backend implementation, and frontend updates required to log Minecraft server crashes, automatically prune those logs after 24 hours, and display them in a dedicated logs page accessible from the sidebar.

## User Review Required

No breaking changes are anticipated. The SQLite schema will automatically upgrade to support the new `app_logs` table upon the next application launch.

## Proposed Changes

---

### 1. Database Schema & Models

#### [MODIFY] [schema.go](file:///c:/Users/woopsy/Project/Random/Porque/internal/db/schema.go)
- Add the `app_logs` table definition to the `Schema` constant:
  ```sql
  CREATE TABLE IF NOT EXISTS app_logs (
      id          TEXT PRIMARY KEY,
      server_id   TEXT REFERENCES servers(id) ON DELETE CASCADE,
      server_name TEXT NOT NULL,
      message     TEXT NOT NULL,
      created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
  );
  CREATE INDEX IF NOT EXISTS idx_app_logs_created ON app_logs(created_at DESC);
  ```

#### [MODIFY] [models.go](file:///c:/Users/woopsy/Project/Random/Porque/internal/db/models.go)
- Define the `AppLog` struct representing the database model:
  ```go
  type AppLog struct {
      ID         string    `db:"id" json:"id"`
      ServerID   string    `db:"server_id" json:"server_id"`
      ServerName string    `db:"server_name" json:"server_name"`
      Message    string    `db:"message" json:"message"`
      CreatedAt  time.Time `db:"created_at" json:"created_at"`
  }
  ```

---

### 2. Database Store Actions

#### [MODIFY] [store.go](file:///c:/Users/woopsy/Project/Random/Porque/internal/db/store.go)
- Implement backend queries:
  - `AddAppLog(ctx, serverID, serverName, message)`: Inserts a log entry.
  - `ListAppLogs(ctx)`: Returns all logs ordered by `created_at DESC`.
  - `PruneAppLogs(ctx)`: Deletes any logs created older than 24 hours ago using SQLite's native `datetime('now', '-24 hours')`.

---

### 3. Server Crash Event & Retention Cleanups

#### [MODIFY] [lifecycle.go](file:///c:/Users/woopsy/Project/Random/Porque/internal/mcserver/lifecycle.go)
- In the `transition` method (or inside the process wait monitor loop), check if the server is transitioning to `db.StateCrashed`.
- If a crash occurs, call `c.store.AddAppLog(...)` with the crash details (e.g., exit code).

#### [MODIFY] [worker.go](file:///c:/Users/woopsy/Project/Random/Porque/internal/worker/worker.go)
- Inside the background `backupScheduler` loop (which ticks every minute), call `w.store.PruneAppLogs(ctx)` to automatically clean up expired logs.

#### [MODIFY] [app.go](file:///c:/Users/woopsy/Project/Random/Porque/app.go)
- Expose `ListAppLogs() ([]db.AppLog, error)` to Wails.

---

### 4. Frontend Logs UI & Routing

#### [MODIFY] [api.ts](file:///c:/Users/woopsy/Project/Random/Porque/web/src/lib/api.ts)
- Bind the `ListAppLogs` Go function to the TypeScript client API.

#### [MODIFY] [app-shell.tsx](file:///c:/Users/woopsy/Project/Random/Porque/web/src/components/app-shell.tsx)
- Add "Logs" to the sidebar navigation configuration using a console/terminal icon.

#### [MODIFY] [App.tsx](file:///c:/Users/woopsy/Project/Random/Porque/web/src/App.tsx)
- Add a route `/logs` rendering `<LogsPage />`.

#### [NEW] [logs.tsx](file:///c:/Users/woopsy/Project/Random/Porque/web/src/pages/logs.tsx)
- Create a new logs page that fetches logs using `react-query` and renders them in a beautiful, premium, search-filterable layout showing:
  - Server badge (name).
  - Time elapsed (timestamp, e.g. "5 minutes ago").
  - Crash/error description.
  - A clean, modern "No logs recorded" state if there are no crashes in the last 24 hours.

---

## Verification Plan

### Automated Tests
- Run `go build` to verify Go types and database operations compile.
- Run `npm run typecheck` inside `web` to verify frontend routing and page state code.

### Manual Verification
1. Launch the app using `wails dev`.
2. Check that the "Logs" navigation link is visible in the sidebar.
3. Simulate a server crash (e.g. by stopping a server abruptly, or force killing it, or making it exit with non-zero code) and verify it registers immediately in the Logs page.
4. Verify that logs older than 24 hours are deleted by the background task.
