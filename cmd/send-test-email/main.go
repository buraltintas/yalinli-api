package main

import (
	"flag"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"yalinli-api/internal/config"
	"yalinli-api/internal/email"
)

func main() {
	to := flag.String("to", "", "recipient email address")
	flag.Parse()

	if strings.TrimSpace(*to) == "" {
		fmt.Fprintln(os.Stderr, "error: --to is required")
		os.Exit(1)
	}

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: failed to load config: %v\n", err)
		os.Exit(1)
	}
	if strings.TrimSpace(cfg.SMTPHost) == "" {
		fmt.Fprintln(os.Stderr, "error: SMTP_HOST is empty; configure SMTP env vars before sending test email")
		os.Exit(1)
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: cfg.LogLevel}))
	svc := email.NewService(logger, cfg.SMTPHost, cfg.SMTPPort, cfg.SMTPUsername, cfg.SMTPPassword, cfg.EmailFrom, cfg.EmailFromName)

	if err := svc.SendSync(strings.TrimSpace(*to), email.TestEmailSubject(), email.TestEmailBody()); err != nil {
		fmt.Fprintf(os.Stderr, "send failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("send successful: %s\n", strings.TrimSpace(*to))
}
