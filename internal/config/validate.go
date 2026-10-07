package config

import (
	"fmt"
	"strings"

	"github.com/sqlwarden/internal/validator"
)

// Validate rejects configurations that cannot produce a working process. It is
// safe to call on a [Config] built by hand as well as one returned by [Load];
// startup calls it again so programmatic construction cannot skip the checks.
func Validate(cfg Config) error {
	if strings.TrimSpace(cfg.BootstrapBaseURL) == "" || !validator.IsURL(cfg.BootstrapBaseURL) {
		return fmt.Errorf("base_url must be a valid URL")
	}
	if cfg.DeploymentMode != DeploymentModeServer && cfg.DeploymentMode != DeploymentModeDesktop {
		return fmt.Errorf("deployment_mode must be %q or %q", DeploymentModeServer, DeploymentModeDesktop)
	}
	if cfg.AccessMode != AccessModeMultiUser && cfg.AccessMode != AccessModeSingleUser {
		return fmt.Errorf("access_mode must be %q or %q", AccessModeMultiUser, AccessModeSingleUser)
	}
	if !IsSupportedLogFormat(cfg.Log.Format) {
		return fmt.Errorf("log.format must be %q or %q", LogFormatJSON, LogFormatText)
	}
	if err := validateProcessKinds(cfg); err != nil {
		return err
	}
	if err := validateMigrationPolicy(cfg); err != nil {
		return err
	}
	if cfg.ShutdownTimeout <= 0 {
		return fmt.Errorf("shutdown_timeout must be greater than zero")
	}
	if cfg.TLS.Enabled {
		if strings.TrimSpace(cfg.TLS.CertFile) == "" {
			return fmt.Errorf("tls.cert_file is required when tls.enabled is true")
		}
		if strings.TrimSpace(cfg.TLS.KeyFile) == "" {
			return fmt.Errorf("tls.key_file is required when tls.enabled is true")
		}
	}
	if cfg.Files.StorageMode != FilesStorageModeFile && cfg.Files.StorageMode != FilesStorageModeObject {
		return fmt.Errorf("files.storage_mode must be %q or %q", FilesStorageModeFile, FilesStorageModeObject)
	}
	if err := validateFileStorageBackends(cfg); err != nil {
		return err
	}
	return validateDesktopBackends(cfg)
}

func validateProcessKinds(cfg Config) error {
	if len(cfg.ProcessKinds) == 0 {
		return fmt.Errorf("process_kinds must not be empty")
	}
	seen := make(map[string]struct{}, len(cfg.ProcessKinds))
	for _, kind := range cfg.ProcessKinds {
		switch kind {
		case ProcessKindAll, ProcessKindAPI, ProcessKindJobs:
		case ProcessKindConnector, ProcessKindEdgeGateway, ProcessKindRealtime:
			return fmt.Errorf("process kind %q is not implemented", kind)
		default:
			return fmt.Errorf("process_kinds contains unknown kind %q", kind)
		}
		if _, dup := seen[kind]; dup {
			return fmt.Errorf("process_kinds contains %q more than once", kind)
		}
		seen[kind] = struct{}{}
	}
	if _, isAll := seen[ProcessKindAll]; isAll && len(cfg.ProcessKinds) > 1 {
		return fmt.Errorf("process_kinds %q cannot be combined with other process kinds", ProcessKindAll)
	}
	// A SQLite application database is one file owned by one process, so a
	// split topology would have several processes writing the same file.
	if cfg.DB.Driver == "sqlite" && !cfg.SelectsOnly(ProcessKindAll) {
		return fmt.Errorf("db.driver %q requires process_kinds %q", "sqlite", ProcessKindAll)
	}
	return nil
}

// validateMigrationPolicy keeps schema migrations out of split topologies.
// Those run several processes against one database, so each would race the
// others. They migrate once through the migrate command instead.
func validateMigrationPolicy(cfg Config) error {
	if cfg.DB.MigrationTimeout <= 0 {
		return fmt.Errorf("db.migration_timeout must be greater than zero")
	}
	if !cfg.DB.Automigrate {
		return nil
	}
	for _, kind := range []string{ProcessKindAPI, ProcessKindJobs} {
		if explicitlySelectsProcessKind(cfg, kind) {
			return fmt.Errorf(
				"db.automigrate must be false when process_kinds selects %q; run the %q command once before starting the processes",
				kind, MigrateCommand,
			)
		}
	}
	return nil
}

func explicitlySelectsProcessKind(cfg Config, kind string) bool {
	for _, selected := range cfg.ProcessKinds {
		if selected == kind {
			return true
		}
	}
	return false
}

func validateFileStorageBackends(cfg Config) error {
	if cfg.Files.StorageMode == FilesStorageModeObject && strings.TrimSpace(cfg.Files.ActiveStorageBackend) == "" {
		return fmt.Errorf("files.active_storage_backend is required when files.storage_mode=%q", FilesStorageModeObject)
	}
	if len(cfg.Files.StorageBackends) == 0 {
		return fmt.Errorf("files.storage_backends must contain at least one backend")
	}

	for id, backend := range cfg.Files.StorageBackends {
		if strings.TrimSpace(id) == "" {
			return fmt.Errorf("files.storage_backends contains an empty backend ID")
		}
		if backend.Type != FilesStorageBackendFilesystem {
			if backend.Type == FilesStorageBackendS3 {
				return fmt.Errorf("files.storage_backends.%s.type=%q is not implemented yet", id, FilesStorageBackendS3)
			}
			return fmt.Errorf("files.storage_backends.%s.type must be %q", id, FilesStorageBackendFilesystem)
		}
		if strings.TrimSpace(backend.RootDir) == "" {
			return fmt.Errorf("files.storage_backends.%s.root_dir is required", id)
		}
	}

	if cfg.Files.StorageMode == FilesStorageModeObject {
		if _, ok := cfg.Files.StorageBackends[cfg.Files.ActiveStorageBackend]; !ok {
			return fmt.Errorf("files.active_storage_backend %q must reference a configured storage backend", cfg.Files.ActiveStorageBackend)
		}
		return nil
	}

	if _, ok := cfg.Files.StorageBackends[defaultFilesActiveBackend]; !ok {
		return fmt.Errorf("files.storage_backends.%s is required when files.storage_mode=%q", defaultFilesActiveBackend, FilesStorageModeFile)
	}
	return nil
}

func validateDesktopBackends(cfg Config) error {
	if strings.TrimSpace(cfg.Desktop.ActiveBackend) == "" {
		return fmt.Errorf("desktop.active_backend is required")
	}

	seenBackendIDs := map[string]struct{}{}
	activeBackendFound := false
	for _, backend := range cfg.Desktop.Backends {
		if strings.TrimSpace(backend.ID) == "" {
			return fmt.Errorf("desktop.backends[].id is required")
		}
		if _, exists := seenBackendIDs[backend.ID]; exists {
			return fmt.Errorf("desktop backend %q is duplicated", backend.ID)
		}
		seenBackendIDs[backend.ID] = struct{}{}

		if strings.TrimSpace(backend.Name) == "" {
			return fmt.Errorf("desktop backend %q name is required", backend.ID)
		}
		if backend.Kind != DesktopBackendKindLocal && backend.Kind != DesktopBackendKindRemote {
			return fmt.Errorf("desktop backend %q kind must be %q or %q", backend.ID, DesktopBackendKindLocal, DesktopBackendKindRemote)
		}
		if backend.Kind == DesktopBackendKindRemote && strings.TrimSpace(backend.URL) == "" {
			return fmt.Errorf("desktop remote backend %q url is required", backend.ID)
		}
		if backend.Kind == DesktopBackendKindLocal && strings.TrimSpace(backend.URL) != "" {
			return fmt.Errorf("desktop local backend %q must not set url", backend.ID)
		}
		if backend.AccessMode != "" && backend.AccessMode != AccessModeMultiUser && backend.AccessMode != AccessModeSingleUser {
			return fmt.Errorf("desktop backend %q access_mode must be %q or %q", backend.ID, AccessModeMultiUser, AccessModeSingleUser)
		}
		if backend.ID == cfg.Desktop.ActiveBackend {
			activeBackendFound = true
		}
	}
	if !activeBackendFound {
		return fmt.Errorf("desktop.active_backend %q must reference a configured backend", cfg.Desktop.ActiveBackend)
	}
	return nil
}
