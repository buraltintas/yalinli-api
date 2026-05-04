package email

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
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
	addr := fmt.Sprintf("%s:%d", s.host, s.port)
	headers := "From: " + s.fromName + " <" + s.from + ">\r\n" + "To: " + to + "\r\n" + "Subject: " + subject + "\r\n\r\n"
	msg := []byte(headers + body)
	auth := smtp.PlainAuth("", s.username, s.password, s.host)
	return smtp.SendMail(addr, auth, s.from, []string{to}, msg)
}
