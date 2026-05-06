package email

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log/slog"
	"mime"
	"net"
	"net/mail"
	"net/smtp"
	"strings"
	"time"
)

type Sender interface {
	SendAsync(ctx context.Context, to, subject string, content Content)
}

type Content struct {
	Text string
	HTML string
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

type job struct{ to, subject, body, html string }

func NewService(logger *slog.Logger, host string, port int, username, password, from, fromName string) *Service {
	s := &Service{logger: logger, host: host, port: port, username: username, password: password, from: from, fromName: fromName, jobs: make(chan job, 200)}
	go s.worker()
	return s
}

func (s *Service) SendAsync(_ context.Context, to, subject string, content Content) {
	select {
	case s.jobs <- job{to: to, subject: subject, body: content.Text, html: content.HTML}:
	default:
		s.logger.Warn("email queue full")
	}
}

func (s *Service) worker() {
	for j := range s.jobs {
		if err := s.SendSync(j.to, j.subject, Content{Text: j.body, HTML: j.html}); err != nil {
			s.logger.Error("email send failed", "to", j.to, "error", err.Error())
			continue
		}
		s.logger.Info("email sent successfully", "to", j.to, "subject", j.subject)
	}
}

func (s *Service) SendSync(to, subject string, content Content) error {
	if strings.TrimSpace(s.host) == "" {
		return errors.New("SMTP_HOST is empty")
	}
	if strings.TrimSpace(s.username) == "" {
		return errors.New("SMTP_USERNAME is empty")
	}
	if strings.TrimSpace(s.password) == "" {
		return errors.New("SMTP_PASSWORD is empty")
	}
	fromAddr := strings.TrimSpace(s.from)
	if fromAddr == "" {
		fromAddr = strings.TrimSpace(s.username)
	}
	heloName := heloDomain(fromAddr)
	replyTo := ""
	if fromAddr != s.username && !strings.Contains(strings.ToLower(fromAddr), strings.ToLower(s.username)) {
		replyTo = fromAddr
	}
	fromHeader := (&mail.Address{Name: strings.TrimSpace(s.fromName), Address: fromAddr}).String()
	var msg strings.Builder
	msg.WriteString(fmt.Sprintf("From: %s\r\n", fromHeader))
	msg.WriteString(fmt.Sprintf("To: %s\r\n", to))
	if replyTo != "" {
		msg.WriteString(fmt.Sprintf("Reply-To: %s\r\n", replyTo))
	}
	msg.WriteString(fmt.Sprintf("Subject: %s\r\n", mime.QEncoding.Encode("utf-8", subject)))
	msg.WriteString(fmt.Sprintf("Date: %s\r\n", time.Now().Format(time.RFC1123Z)))
	msg.WriteString(fmt.Sprintf("Message-ID: <%d.%s@yalinli.org>\r\n", time.Now().UnixNano(), sanitizeMessageIDLocal(to)))
	msg.WriteString("Auto-Submitted: auto-generated\r\n")
	msg.WriteString("X-Auto-Response-Suppress: All\r\n")
	msg.WriteString("MIME-Version: 1.0\r\n")
	if strings.TrimSpace(content.HTML) != "" {
		boundary := fmt.Sprintf("yalinli-%d", time.Now().UnixNano())
		msg.WriteString(fmt.Sprintf("Content-Type: multipart/alternative; boundary=\"%s\"\r\n", boundary))
		msg.WriteString("\r\n")
		msg.WriteString(fmt.Sprintf("--%s\r\n", boundary))
		msg.WriteString("Content-Type: text/plain; charset=\"utf-8\"\r\n")
		msg.WriteString("Content-Transfer-Encoding: 8bit\r\n\r\n")
		msg.WriteString(content.Text)
		msg.WriteString("\r\n")
		msg.WriteString(fmt.Sprintf("--%s\r\n", boundary))
		msg.WriteString("Content-Type: text/html; charset=\"utf-8\"\r\n")
		msg.WriteString("Content-Transfer-Encoding: 8bit\r\n\r\n")
		msg.WriteString(content.HTML)
		msg.WriteString("\r\n")
		msg.WriteString(fmt.Sprintf("--%s--\r\n", boundary))
	} else {
		msg.WriteString("Content-Type: text/plain; charset=\"utf-8\"\r\n")
		msg.WriteString("Content-Transfer-Encoding: 8bit\r\n")
		msg.WriteString("\r\n")
		msg.WriteString(content.Text)
	}

	addr := net.JoinHostPort(s.host, fmt.Sprintf("%d", s.port))
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

	if err = client.Hello(heloName); err != nil {
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

func sanitizeMessageIDLocal(v string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(strings.TrimSpace(v)) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	if b.Len() == 0 {
		return "mail"
	}
	return b.String()
}

func heloDomain(fromAddr string) string {
	addr := strings.TrimSpace(strings.ToLower(fromAddr))
	if at := strings.LastIndex(addr, "@"); at != -1 && at+1 < len(addr) {
		return addr[at+1:]
	}
	return "localhost"
}
