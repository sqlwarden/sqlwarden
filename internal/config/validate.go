package config

import (
	"fmt"
	"net"
	"strconv"
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
	if err := validateSessionDirectory(cfg); err != nil {
		return err
	}
	if err := validateEdition(cfg); err != nil {
		return err
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
		return fmt.Errorf("process_kinds must name at least one process kind")
	}
	seen := make(map[string]struct{}, len(cfg.ProcessKinds))
	for _, kind := range cfg.ProcessKinds {
		switch kind {
		case ProcessKindAll, ProcessKindAPI, ProcessKindConnector, ProcessKindEdgeGateway, ProcessKindJobs, ProcessKindRealtime:
		default:
			return fmt.Errorf("process_kinds contains unknown process kind %q", kind)
		}
		if _, exists := seen[kind]; exists {
			return fmt.Errorf("process_kinds contains duplicate process kind %q", kind)
		}
		seen[kind] = struct{}{}
	}
	if _, isAll := seen[ProcessKindAll]; isAll && len(cfg.ProcessKinds) > 1 {
		return fmt.Errorf("process_kinds %q cannot be combined with other process kinds", ProcessKindAll)
	}
	return nil
}

// validateSessionDirectory enforces the scaling invariant for live target
// database sessions. The static directory always returns one configured
// connector, so running more than one connector replica requires a shared
// directory backend.
func validateSessionDirectory(cfg Config) error {
	switch cfg.SessionDirectory {
	case SessionDirectoryStatic, SessionDirectoryRedis:
	default:
		return fmt.Errorf("session_directory must be %q or %q", SessionDirectoryStatic, SessionDirectoryRedis)
	}
	if cfg.Connector.Replicas < 1 {
		return fmt.Errorf("connector.replicas must be at least 1")
	}
	if cfg.SessionDirectory == SessionDirectoryStatic && cfg.Connector.Replicas > 1 {
		return fmt.Errorf(
			"connector.replicas=%d requires session_directory=%q; the %q session directory has one fixed destination and pins the connector process kind to exactly 1 replica",
			cfg.Connector.Replicas, SessionDirectoryRedis, SessionDirectoryStatic,
		)
	}
	if cfg.SessionDirectory == SessionDirectoryRedis {
		return fmt.Errorf("session_directory %q is not implemented yet", SessionDirectoryRedis)
	}
	apiSelected := explicitlySelectsProcessKind(cfg, ProcessKindAPI)
	connectorSelected := explicitlySelectsProcessKind(cfg, ProcessKindConnector)
	if apiSelected || connectorSelected {
		if err := validateConnectorAddress("connector.address", cfg.Connector.Address, false); err != nil {
			return err
		}
		if connectorSelected {
			if err := validateConnectorAddress("connector.listen_address", cfg.Connector.ListenAddress, true); err != nil {
				return err
			}
		}
		if len(cfg.Connector.GrantSigningKey) < 32 {
			return fmt.Errorf("connector.grant_signing_key must be at least 32 bytes")
		}
		switch cfg.Connector.Transport {
		case ConnectorTransportInsecure:
		case ConnectorTransportTLS:
			if connectorSelected && (strings.TrimSpace(cfg.Connector.TLS.CertFile) == "" || strings.TrimSpace(cfg.Connector.TLS.KeyFile) == "") {
				return fmt.Errorf("connector.tls.cert_file and connector.tls.key_file are required when connector.transport=%q", ConnectorTransportTLS)
			}
		default:
			return fmt.Errorf("connector.transport must be %q or %q", ConnectorTransportInsecure, ConnectorTransportTLS)
		}
	}
	return nil
}

func validateConnectorAddress(name, value string, allowEmptyHost bool) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return fmt.Errorf("%s is required", name)
	}
	host, portText, err := net.SplitHostPort(value)
	if err != nil {
		return fmt.Errorf("%s must be a plain host:port: %w", name, err)
	}
	if !allowEmptyHost && strings.TrimSpace(host) == "" {
		return fmt.Errorf("%s host is required", name)
	}
	port, err := strconv.Atoi(portText)
	if err != nil || port < 1 || port > 65535 {
		return fmt.Errorf("%s port must be between 1 and 65535", name)
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

func validateEdition(cfg Config) error {
	switch cfg.Edition.Name {
	case EditionCommunity:
		if strings.TrimSpace(cfg.Edition.LicenseFile) != "" {
			return fmt.Errorf("edition.license_file must not be set when edition.name is %q", EditionCommunity)
		}
	case EditionEnterprise:
		if strings.TrimSpace(cfg.Edition.LicenseFile) == "" {
			return fmt.Errorf("edition.license_file is required when edition.name is %q", EditionEnterprise)
		}
	default:
		return fmt.Errorf("edition.name must be %q or %q", EditionCommunity, EditionEnterprise)
	}
	return nil
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
