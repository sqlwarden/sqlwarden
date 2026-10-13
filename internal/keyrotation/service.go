package keyrotation

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/sqlwarden/internal/access"
	"github.com/sqlwarden/internal/credentials"
)

// Report summarizes a rotation pass. It carries counts only, never values.
type Report struct {
	ConnectionsSplit         int `json:"connections_split"`
	ConnectionSecretsRotated int `json:"connection_secrets_rotated"`
	ConnectionSecretsSkipped int `json:"connection_secrets_skipped"`
	FileContentsScanned      int `json:"file_contents_scanned"`
	FileContentsRotated      int `json:"file_contents_rotated"`
	SMTPPasswordsScanned     int `json:"smtp_passwords_scanned"`
	SMTPPasswordsRotated     int `json:"smtp_passwords_rotated"`
}

// Options tunes a rotation pass. The zero value rotates every stage.
type Options struct{}

// ConnectionRotator converts legacy connection rows and re-seals stored
// connection secrets.
type ConnectionRotator interface {
	Run(ctx context.Context) (credentials.RotationReport, error)
}

// SMTPRotator re-seals the stored SMTP password. scanned reports whether a
// password is stored at all.
type SMTPRotator interface {
	RotateSMTPPassword(ctx context.Context) (scanned, rotated bool, err error)
}

// FileRotator re-seals application-encrypted file contents.
type FileRotator interface {
	RotateContents(ctx context.Context) (scanned, rotated int, err error)
}

// Deps are the stage ports and logger the service runs with.
type Deps struct {
	Connections ConnectionRotator
	SMTP        SMTPRotator
	Files       FileRotator
	Logger      *slog.Logger
}

// Service runs key rotation for authorized callers.
type Service struct {
	connections ConnectionRotator
	smtp        SMTPRotator
	files       FileRotator
	logger      *slog.Logger
}

func New(deps Deps) *Service {
	logger := deps.Logger
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}
	return &Service{connections: deps.Connections, smtp: deps.SMTP, files: deps.Files, logger: logger}
}

// OperatorPrincipal is the principal of the rotate-keys command. Anyone who
// can run the binary against the deployment's configuration and database
// already holds the keys.
func OperatorPrincipal() access.Principal {
	return access.Principal{Credential: access.CredentialInfo{Kind: access.CredentialLocal, Method: "cli"}}
}

func isInstanceAdmin(p access.Principal) bool {
	return p.Subject.Kind == access.SubjectAccount && p.InstanceAdmin
}

func isOperator(p access.Principal) bool {
	return p.Credential.Kind == access.CredentialLocal &&
		p.Subject == (access.SubjectRef{}) &&
		p.OnBehalfOf == nil
}

// Rotate re-encrypts every value not sealed with the primary key. Instance
// admins and the operator command may run it.
//
// Rotation is idempotent. Each connection, file, and settings row commits on
// its own, so a failure leaves earlier items rotated. A failing stage stops the
// run; the returned report holds the counts reached so far.
func (s *Service) Rotate(ctx context.Context, p access.Principal, _ Options) (Report, error) {
	if !isInstanceAdmin(p) && !isOperator(p) {
		s.logger.WarnContext(ctx, "encryption key rotation denied")
		return Report{}, access.ErrForbidden
	}

	var report Report
	s.logger.InfoContext(ctx, "encryption key rotation started")

	rotated, err := s.connections.Run(ctx)
	report.ConnectionsSplit = rotated.Split
	report.ConnectionSecretsRotated = rotated.Reencrypted
	report.ConnectionSecretsSkipped = rotated.Skipped
	if err != nil {
		return report, fmt.Errorf("rotate connection credentials: %w", err)
	}

	scanned, rotatedFiles, err := s.files.RotateContents(ctx)
	report.FileContentsScanned = scanned
	report.FileContentsRotated = rotatedFiles
	if err != nil {
		return report, err
	}
	s.logger.InfoContext(ctx, "file content encryption rotation pass complete",
		slog.Int("file_contents_scanned", report.FileContentsScanned),
		slog.Int("file_contents_rotated", report.FileContentsRotated),
	)

	smtpScanned, smtpRotated, err := s.smtp.RotateSMTPPassword(ctx)
	if smtpScanned {
		report.SMTPPasswordsScanned++
	}
	if smtpRotated {
		report.SMTPPasswordsRotated++
	}
	if err != nil {
		return report, err
	}

	s.logger.InfoContext(ctx, "encryption key rotation complete",
		slog.Int("connections_split", report.ConnectionsSplit),
		slog.Int("connection_secrets_rotated", report.ConnectionSecretsRotated),
		slog.Int("connection_secrets_skipped", report.ConnectionSecretsSkipped),
		slog.Int("file_contents_scanned", report.FileContentsScanned),
		slog.Int("file_contents_rotated", report.FileContentsRotated),
	)
	return report, nil
}
