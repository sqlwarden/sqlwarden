package settings

import (
	"context"
	"fmt"
	"sync"

	"github.com/sqlwarden/internal/database"
	"github.com/sqlwarden/internal/smtp"
)

// Decrypter opens instance secrets sealed by a Sealer.
type Decrypter interface {
	Decrypt(ciphertext string) (string, error)
}

// KeyRotator re-seals instance secrets with the primary key.
type KeyRotator interface {
	Sealer
	Decrypter
	NeedsRotation(ciphertext string) bool
}

// NewMailer builds the mailer the instance SMTP settings describe. With SMTP
// disabled it returns a mailer whose sends fail with smtp.ErrDisabled.
func NewMailer(in Instance, keys Decrypter) (*smtp.Mailer, error) {
	if !in.SMTPEnabled {
		return smtp.NewDisabledMailer(in.SMTPFrom), nil
	}
	password := ""
	if in.SMTPPasswordEncrypted != "" {
		var err error
		password, err = keys.Decrypt(in.SMTPPasswordEncrypted)
		if err != nil {
			return nil, fmt.Errorf("decrypt SMTP password: %w", err)
		}
	}
	mailer, err := smtp.NewMailer(in.SMTPHost, in.SMTPPort, in.SMTPUsername, password, in.SMTPFrom)
	if err != nil {
		return nil, fmt.Errorf("configure SMTP: %w", err)
	}
	return mailer, nil
}

// LiveMailer holds the mailer built from the current instance settings. It is
// swapped when the SMTP settings change, so senders never keep a stale one.
type LiveMailer struct {
	mu      sync.RWMutex
	current *smtp.Mailer
}

// NewLiveMailer returns a holder whose sends fail with smtp.ErrDisabled until
// a mailer is set.
func NewLiveMailer() *LiveMailer {
	return &LiveMailer{}
}

// Set replaces the current mailer.
func (l *LiveMailer) Set(m *smtp.Mailer) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.current = m
}

// Current returns the current mailer, or nil when none is set.
func (l *LiveMailer) Current() *smtp.Mailer {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.current
}

// Send delivers a templated email, retrying transient failures.
func (l *LiveMailer) Send(recipient string, data any, templates ...string) error {
	l.mu.RLock()
	defer l.mu.RUnlock()
	if l.current == nil {
		return smtp.ErrDisabled
	}
	return l.current.Send(recipient, data, templates...)
}

// SendOnce delivers a templated email with a single attempt.
func (l *LiveMailer) SendOnce(recipient string, data any, templates ...string) error {
	l.mu.RLock()
	defer l.mu.RUnlock()
	if l.current == nil {
		return smtp.ErrDisabled
	}
	return l.current.SendOnce(recipient, data, templates...)
}

// RotateSMTPPassword re-seals a stored SMTP password that is not sealed with
// the primary key of keys and notifies subscribers when it does. scanned
// reports whether a password is stored at all.
func (s *Service) RotateSMTPPassword(ctx context.Context, keys KeyRotator) (scanned, rotated bool, err error) {
	in, err := s.Instance(ctx)
	if err != nil {
		return false, false, fmt.Errorf("rotate SMTP password: get settings: %w", err)
	}
	if in.SMTPPasswordEncrypted == "" {
		return false, false, nil
	}
	if !keys.NeedsRotation(in.SMTPPasswordEncrypted) {
		return true, false, nil
	}
	plaintext, err := keys.Decrypt(in.SMTPPasswordEncrypted)
	if err != nil {
		return true, false, fmt.Errorf("rotate SMTP password: decrypt: %w", err)
	}
	in.SMTPPasswordEncrypted, err = keys.Encrypt(plaintext)
	if err != nil {
		return true, false, fmt.Errorf("rotate SMTP password: encrypt: %w", err)
	}
	stored, err := s.store.UpsertInstanceSettings(ctx, database.InstanceSettings(in))
	if err != nil {
		return true, false, fmt.Errorf("rotate SMTP password: update: %w", err)
	}
	s.publish(Instance(stored))
	return true, true, nil
}
