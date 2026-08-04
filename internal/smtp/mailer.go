package smtp

import (
	"bytes"
	"errors"
	"time"

	"github.com/sqlwarden/assets"
	"github.com/sqlwarden/internal/funcs"

	"github.com/wneessen/go-mail"

	htmlTemplate "html/template"
	textTemplate "text/template"
)

const defaultTimeout = 10 * time.Second

var ErrDisabled = errors.New("smtp is disabled")

type Mailer struct {
	client *mail.Client
	from   string

	mockSend     bool
	disabled     bool
	SentMessages []string
}

func NewMailer(host string, port int, username, password, from string) (*Mailer, error) {
	client, err := mail.NewClient(host, mail.WithTimeout(defaultTimeout), mail.WithSMTPAuth(mail.SMTPAuthLogin), mail.WithPort(port), mail.WithUsername(username), mail.WithPassword(password))
	if err != nil {
		return nil, err
	}

	mailer := &Mailer{
		client: client,
		from:   from,
	}

	return mailer, nil
}

func NewMockMailer(from string) *Mailer {
	mailer := &Mailer{
		from:     from,
		mockSend: true,
	}

	return mailer
}

func NewDisabledMailer(from string) *Mailer {
	return &Mailer{from: from, disabled: true}
}

func (m *Mailer) Send(recipient string, data any, patterns ...string) error {
	return m.send(3, recipient, data, patterns...)
}

// SendOnce performs one bounded SMTP attempt. Request handlers use this so a
// temporarily unavailable mail server cannot multiply the client timeout.
func (m *Mailer) SendOnce(recipient string, data any, patterns ...string) error {
	return m.send(1, recipient, data, patterns...)
}

func (m *Mailer) send(attempts int, recipient string, data any, patterns ...string) error {
	if m.disabled {
		return ErrDisabled
	}
	for i := range patterns {
		patterns[i] = "emails/" + patterns[i]
	}
	msg := mail.NewMsg()

	err := msg.To(recipient)
	if err != nil {
		return err
	}

	err = msg.From(m.from)
	if err != nil {
		return err
	}

	ts, err := textTemplate.New("").Funcs(funcs.TemplateFuncs).ParseFS(assets.EmbeddedFiles, patterns...)
	if err != nil {
		return err
	}

	subject := new(bytes.Buffer)
	err = ts.ExecuteTemplate(subject, "subject", data)
	if err != nil {
		return err
	}

	msg.Subject(subject.String())

	plainBody := new(bytes.Buffer)
	err = ts.ExecuteTemplate(plainBody, "plainBody", data)
	if err != nil {
		return err
	}

	msg.SetBodyString(mail.TypeTextPlain, plainBody.String())

	if ts.Lookup("htmlBody") != nil {
		ts, err := htmlTemplate.New("").Funcs(funcs.TemplateFuncs).ParseFS(assets.EmbeddedFiles, patterns...)
		if err != nil {
			return err
		}

		htmlBody := new(bytes.Buffer)
		err = ts.ExecuteTemplate(htmlBody, "htmlBody", data)
		if err != nil {
			return err
		}

		msg.AddAlternativeString(mail.TypeTextHTML, htmlBody.String())
	}

	if m.mockSend {
		var buf bytes.Buffer

		_, err := msg.WriteTo(&buf)
		if err != nil {
			return err
		}

		m.SentMessages = append(m.SentMessages, buf.String())
		return nil
	}

	for i := 1; i <= attempts; i++ {
		err = m.client.DialAndSend(msg)

		if nil == err {
			return nil
		}

		if i != attempts {
			time.Sleep(2 * time.Second)
		}
	}

	return err
}
