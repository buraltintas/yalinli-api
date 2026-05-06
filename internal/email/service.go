package email

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/smtp"
	"strings"
)

type Sender interface {
	SendAsync(ctx context.Context, to, subject, body string)
}

type Service struct {
	logger   *slog.Logger
	host     string
	port     int
	username string
	password string
	from     string
	fromName string
	jobs     chan job
}

type job struct{ to, subject, body string }

func NewService(logger *slog.Logger, host string, port int, username, password, from, fromName string) *Service {
	s := &Service{logger: logger, host: host, port: port, username: username, password: password, from: from, fromName: fromName, jobs: make(chan job, 200)}
	go s.worker()
	return s
}

func (s *Service) SendAsync(_ context.Context, to, subject, body string) {
	select {
	case s.jobs <- job{to: to, subject: subject, body: body}:
	default:
		s.logger.Warn("email queue full")
	}
}

func (s *Service) worker() {
	for j := range s.jobs {
		if err := s.SendSync(j.to, j.subject, j.body); err != nil {
			s.logger.Error("email send failed", "to", j.to, "error", err.Error())
		}
	}
}

func (s *Service) SendSync(to, subject, body string) error {
	if strings.TrimSpace(s.host) == "" {
		return errors.New("SMTP_HOST is empty")
	}
	if strings.TrimSpace(s.username) == "" {
		return errors.New("SMTP_USERNAME is empty")
	}
	if strings.TrimSpace(s.password) == "" {
		return errors.New("SMTP_PASSWORD is empty")
	}
	fromHeader := strings.TrimSpace(s.from)
	if fromHeader == "" {
		fromHeader = strings.TrimSpace(s.username)
	}
	replyTo := ""
	if fromHeader != s.username && !strings.Contains(strings.ToLower(fromHeader), strings.ToLower(s.username)) {
		replyTo = fromHeader
	}
	var msg strings.Builder
	msg.WriteString(fmt.Sprintf("From: %s <%s>\r\n", s.fromName, fromHeader))
	msg.WriteString(fmt.Sprintf("To: %s\r\n", to))
	if replyTo != "" {
		msg.WriteString(fmt.Sprintf("Reply-To: %s\r\n", replyTo))
	}
	msg.WriteString(fmt.Sprintf("Subject: %s\r\n", subject))
	msg.WriteString("MIME-Version: 1.0\r\n")
	msg.WriteString("Content-Type: text/plain; charset=\"utf-8\"\r\n")
	msg.WriteString("\r\n")
	msg.WriteString(body)

	addr := fmt.Sprintf("%s:%d", s.host, s.port)
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		return fmt.Errorf("failed to connect to SMTP server: %w", err)
	}
	defer conn.Close()

	client, err := smtp.NewClient(conn, s.host)
	if err != nil {
		return fmt.Errorf("failed to create SMTP client: %w", err)
	}
	defer client.Close()

	if err = client.Hello("localhost"); err != nil {
		return fmt.Errorf("SMTP HELO failed: %w", err)
	}

	if ok, _ := client.Extension("STARTTLS"); ok {
		tlsConfig := &tls.Config{
			ServerName: s.host,
			MinVersion: tls.VersionTLS12,
		}
		if err = client.StartTLS(tlsConfig); err != nil {
			return fmt.Errorf("STARTTLS failed: %w", err)
		}
	}

	auth := smtp.PlainAuth("", s.username, s.password, s.host)
	if err = client.Auth(auth); err != nil {
		return fmt.Errorf("SMTP authentication failed: %w", err)
	}

	// Envelope sender should be authenticated account for best provider alignment.
	if err = client.Mail(s.username); err != nil {
		return fmt.Errorf("SMTP MAIL FROM failed: %w", err)
	}
	if err = client.Rcpt(to); err != nil {
		return fmt.Errorf("SMTP RCPT TO failed: %w", err)
	}

	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("SMTP DATA command failed: %w", err)
	}
	if _, err = w.Write([]byte(msg.String())); err != nil {
		_ = w.Close()
		return fmt.Errorf("failed to write email body: %w", err)
	}
	if err = w.Close(); err != nil {
		return fmt.Errorf("failed to close SMTP data writer: %w", err)
	}
	if err = client.Quit(); err != nil {
		return fmt.Errorf("SMTP QUIT failed: %w", err)
	}
	return nil
}
