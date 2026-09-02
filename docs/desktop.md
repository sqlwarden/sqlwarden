# Desktop operations

SQLWarden Desktop is the single-user native packaging of the same Go application and React editor
used by the server product. The executable always selects `mode=desktop`; it cannot be configured as
a multi-user or remotely served hybrid.

## Product behavior

- The editor is the home screen. The last accessible workspace is restored when possible.
- A new installation has a local identity and organization but no workspace. Create one from the
  editor empty state, workspace selector, or Settings > Workspaces.
- The organization access, users, policies, and organization workspace-list routes are unavailable.
  Workspace details, connections, environments, and local settings remain available.
- Settings contains only local product controls: appearance/editor, query/export/history/revision
  policy, workspaces, storage and recovery, diagnostics, and version/update information.
- SQL files can be opened with the system dialog, drag and drop, file association, or a second app
  launch. SQLite database files can be selected with a native dialog.

## Local storage and secrets

Settings > Storage & Recovery displays the resolved paths for the current platform. SQLWarden keeps:

- non-secret bootstrap metadata in the platform configuration directory;
- the SQLite application database and workspace file objects in the platform data directory;
- WebView and other disposable data in the platform cache directory;
- bounded rotating application logs in the platform log directory; and
- temporary snapshots and the default backup location in their own subdirectories.

Installation secrets are stored in the OS credential service (macOS Keychain, Windows Credential
Manager, or the Linux Secret Service). When it is unavailable, SQLWarden uses a mode-`0600` protected
file and shows `Protected local file (fallback)` in Settings. Never copy only the SQLite database to
a different installation: encrypted connection credentials also require the original installation
secrets. Use a SQLWarden backup for supported data transfer.

On first start after upgrading a legacy desktop installation, SQLWarden migrates plaintext secrets
out of `desktop.json` and moves legacy database, files, and logs into the split platform directories.
Migration is idempotent. Missing secrets beside existing data are treated as a startup error.

## Backup and restore

Use Settings > Storage & Recovery > Create backup. SQLWarden first creates a transactionally
consistent SQLite snapshot, then writes a versioned `.sqlwarden-backup` archive containing the
database and workspace files. The archive manifest contains a size and SHA-256 digest for every
entry.

Restore validates the archive format, paths, sizes, and digests before queueing it. After confirmation,
the app restarts and applies the restore before opening the database. A pre-restore rollback archive
is created automatically. If the replacement cannot complete, SQLWarden restores the previous
database and files and leaves diagnostic information in the logs.

Backups contain database content and workspace files. They do not contain installation secrets,
caches, WebView state, logs, or window preferences. Restore a backup into the same installation when
encrypted connection credentials must remain usable.

## Diagnostics, logs, and updates

Settings > Diagnostics can reveal the log directory and save a sanitized JSON report. Reports include
version, OS/architecture, startup state, credential-store type, and home-relative paths; they exclude
tokens, credentials, SQL content, database contents, and environment values. Logs rotate at 5 MiB and
retain three previous files. Cache pruning is size-bounded and never removes logs or user data.

Settings > About > Check for updates opens the official GitHub Releases page in the system browser.
The app does not silently download or install updates.

## Platform release matrix

Every release candidate should complete the following checks on Windows, macOS, and Linux:

| Area | Windows | macOS | Linux |
|---|---:|---:|---:|
| Clean install and first workspace | Required | Required | Required |
| Upgrade/migrate an existing desktop data directory | Required | Required | Required |
| Open SQL and SQLite through dialogs, drag/drop, association, and second launch | Required | Required | Required |
| Save SQL and exports with native dialogs | Required | Required | Required |
| Keychain/credential-store success and protected-file fallback | Required | Required | Required |
| Backup, tamper rejection, restore, and rollback | Required | Required | Required |
| Window position/state, close guard, and graceful shutdown | Required | Required | Required |
| Installer/package install, upgrade, and uninstall | Required | Required | Required |

Linux packages require GTK3 and WebKit2GTK 4.1 at runtime; distribution-specific package names are
listed in `cmd/desktop/build/linux/README.md`.

## Uninstall and data retention

Uninstalling the application package does not intentionally remove the platform configuration, data,
cache, or log directories. This preserves workspaces for reinstall or upgrade. To remove SQLWarden
completely, first create a backup if needed, uninstall the app, then use Settings path information (or
the platform application-data locations recorded before uninstall) to remove the SQLWarden
configuration, data, cache, and log directories and delete the SQLWarden credential-store entry.
Deleting those locations is irreversible.
