package smtp

import (
	"bytes"
	"time"

	"github.com/sqlwarden/assets"
	"github.com/sqlwarden/internal/funcs"

	"github.com/wneessen/go-mail"

	htmlTemplate "html/template"
	textTemplate "text/template"
)

const defaultTimeout = 10 * time.Second

type Mailer struct {
	client *mail.Client
	from   string

	mockSend     bool
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

func (m *Mailer) Send(recipient string, data any, patterns ...string) error {
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

	for i := 1; i <= 3; i++ {
		err = m.client.DialAndSend(msg)

		if nil == err {
			return nil
		}

		if i != 3 {
			time.Sleep(2 * time.Second)
		}
	}

	return err
}
