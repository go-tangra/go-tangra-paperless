package data

import (
	"crypto/tls"
	"fmt"
	"net/smtp"
	"os"
	"strconv"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/tx7do/kratos-bootstrap/bootstrap"
)

// SMTPConfig holds SMTP connection configuration
type SMTPConfig struct {
	Host     string
	Port     int
	Username string
	Password string
	From     string
	TLSMode  string // "none", "starttls", "tls"
}

// SMTPClient sends emails via SMTP
type SMTPClient struct {
	config *SMTPConfig
	log    *log.Helper
}

// NewSMTPClient creates a new SMTP client from environment variables
func NewSMTPClient(ctx *bootstrap.Context) *SMTPClient {
	port, _ := strconv.Atoi(getEnvOrDefault("SMTP_PORT", "587"))

	tlsMode := os.Getenv("SMTP_TLS_MODE")
	if tlsMode == "" {
		useTLS, _ := strconv.ParseBool(getEnvOrDefault("SMTP_USE_TLS", "true"))
		if !useTLS {
			tlsMode = "none"
		} else if port == 465 {
			tlsMode = "tls"
		} else {
			tlsMode = "starttls"
		}
	}

	config := &SMTPConfig{
		Host:     getEnvOrDefault("SMTP_HOST", "localhost"),
		Port:     port,
		Username: getEnvOrDefault("SMTP_USERNAME", ""),
		Password: getEnvOrDefault("SMTP_PASSWORD", ""),
		From:     getEnvOrDefault("SMTP_FROM", "noreply@example.com"),
		TLSMode:  tlsMode,
	}

	return &SMTPClient{
		config: config,
		log:    ctx.NewLoggerHelper("paperless/smtp"),
	}
}

// Send sends an email with HTML body
func (c *SMTPClient) Send(to, subject, htmlBody string) error {
	addr := fmt.Sprintf("%s:%d", c.config.Host, c.config.Port)

	headers := map[string]string{
		"From":         c.config.From,
		"To":           to,
		"Subject":      subject,
		"MIME-Version": "1.0",
		"Content-Type": "text/html; charset=UTF-8",
	}

	var message string
	for k, v := range headers {
		message += fmt.Sprintf("%s: %s\r\n", k, v)
	}
	message += "\r\n" + htmlBody

	var auth smtp.Auth
	if c.config.Username != "" {
		auth = smtp.PlainAuth("", c.config.Username, c.config.Password, c.config.Host)
	}

	switch c.config.TLSMode {
	case "tls":
		return c.sendWithImplicitTLS(addr, auth, to, []byte(message))
	case "starttls":
		return c.sendWithSTARTTLS(addr, auth, to, []byte(message))
	default:
		return smtp.SendMail(addr, auth, c.config.From, []string{to}, []byte(message))
	}
}

func (c *SMTPClient) sendWithImplicitTLS(addr string, auth smtp.Auth, to string, msg []byte) error {
	tlsConfig := &tls.Config{
		ServerName: c.config.Host,
		MinVersion: tls.VersionTLS12,
	}

	conn, err := tls.Dial("tcp", addr, tlsConfig)
	if err != nil {
		return fmt.Errorf("failed to connect to SMTP server: %w", err)
	}

	client, err := smtp.NewClient(conn, c.config.Host)
	if err != nil {
		return fmt.Errorf("failed to create SMTP client: %w", err)
	}
	defer client.Close()

	return c.sendViaSMTPClient(client, auth, to, msg)
}

func (c *SMTPClient) sendWithSTARTTLS(addr string, auth smtp.Auth, to string, msg []byte) error {
	client, err := smtp.Dial(addr)
	if err != nil {
		return fmt.Errorf("failed to connect to SMTP server: %w", err)
	}
	defer client.Close()

	tlsConfig := &tls.Config{
		ServerName: c.config.Host,
		MinVersion: tls.VersionTLS12,
	}
	if err := client.StartTLS(tlsConfig); err != nil {
		return fmt.Errorf("STARTTLS failed: %w", err)
	}

	return c.sendViaSMTPClient(client, auth, to, msg)
}

func (c *SMTPClient) sendViaSMTPClient(client *smtp.Client, auth smtp.Auth, to string, msg []byte) error {
	if auth != nil {
		if err := client.Auth(auth); err != nil {
			return fmt.Errorf("SMTP auth failed: %w", err)
		}
	}

	if err := client.Mail(c.config.From); err != nil {
		return fmt.Errorf("SMTP MAIL FROM failed: %w", err)
	}
	if err := client.Rcpt(to); err != nil {
		return fmt.Errorf("SMTP RCPT TO failed: %w", err)
	}

	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("SMTP DATA failed: %w", err)
	}
	if _, err := w.Write(msg); err != nil {
		return fmt.Errorf("SMTP write failed: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("SMTP close failed: %w", err)
	}

	return client.Quit()
}
