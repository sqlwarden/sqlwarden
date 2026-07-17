// Package app is the public composition root for Community and paid distributions.
package app

import (
	"context"
	"io"
	"log/slog"
	"net/http"

	"github.com/sqlwarden/buildinfo"
	"github.com/sqlwarden/distribution"
	"github.com/sqlwarden/internal/version"
	"github.com/sqlwarden/internal/web"
)

type Config = web.Config
type FileStorageBackend = web.FileStorageBackend

const FilesStorageBackendFilesystem = web.FilesStorageBackendFilesystem

type App struct{ host *web.App }

type EncryptionRotationReport struct {
	ConnectionsScanned  int `json:"connections_scanned"`
	ConnectionsRotated  int `json:"connections_rotated"`
	FileContentsScanned int `json:"file_contents_scanned"`
	FileContentsRotated int `json:"file_contents_rotated"`
}

type Option func(*options)
type options struct{ configure distribution.Configure }

// WithDistribution injects one trusted compile-time distribution composition.
func WithDistribution(configure distribution.Configure) Option {
	return func(options *options) { options.configure = configure }
}

func New(cfg Config, logger *slog.Logger, opts ...Option) (*App, error) {
	var resolved options
	for _, option := range opts {
		option(&resolved)
	}
	configure := func(host distribution.HostServices) (distribution.Dependencies, error) {
		var dependencies distribution.Dependencies
		var err error
		if resolved.configure != nil {
			dependencies, err = resolved.configure(host)
		}
		if err != nil {
			return distribution.Dependencies{}, err
		}
		dependencies.Build.CoreVersion = version.Get()
		dependencies.Build.CoreCommit = version.GetRevision()
		if dependencies.Build.Distribution == "" {
			dependencies.Build.Distribution = "community"
		}
		if dependencies.Build.DistributionVersion == "" {
			dependencies.Build.DistributionVersion = dependencies.Build.CoreVersion
		}
		if dependencies.Build.DistributionCommit == "" {
			dependencies.Build.DistributionCommit = dependencies.Build.CoreCommit
		}
		if dependencies.Build.Date == "" {
			dependencies.Build.Date = version.GetBuildDate()
		}
		return dependencies, nil
	}
	host, err := web.NewWithConfigure(cfg, logger, configure)
	if err != nil {
		return nil, err
	}
	return &App{host: host}, nil
}

func (a *App) Handler() http.Handler               { return a.host.Handler() }
func (a *App) ServeHTTP(ctx context.Context) error { return a.host.ServeHTTP(ctx) }
func (a *App) Close() error                        { return a.host.Close() }
func (a *App) BuildInfo() buildinfo.Info           { return a.host.BuildInfo() }
func (a *App) RotateEncryptionKeys(ctx context.Context) (EncryptionRotationReport, error) {
	report, err := a.host.RotateEncryptionKeys(ctx)
	return EncryptionRotationReport{
		ConnectionsScanned: report.ConnectionsScanned, ConnectionsRotated: report.ConnectionsRotated,
		FileContentsScanned: report.FileContentsScanned, FileContentsRotated: report.FileContentsRotated,
	}, err
}

func DefaultConfig() Config                                     { return web.DefaultConfig() }
func LoadConfig(args []string) (Config, bool, error)            { return web.LoadConfig(args) }
func NewLogger(cfg Config, out io.Writer) (*slog.Logger, error) { return web.NewLogger(cfg, out) }
