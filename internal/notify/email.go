package notify

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/hex"
	"errors"
	"fmt"
	"mime"
	"mime/quotedprintable"
	"net"
	"net/mail"
	"net/smtp"
	"strconv"
	"strings"
	"time"
)

// SMTP is a mail server to send through.
type SMTP struct {
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Username string `json:"username,omitempty"`
	Password string `json:"-"`
	From     string `json:"from"`
	// TLS is how the connection is encrypted: starttls (the default), tls
	// from the start, as on port 465, or none, which also means no login
	// unless the server is on this machine.
	TLS string `json:"tls"`
}

// Check says what is wrong with a mail server's settings, if anything.
func (s SMTP) Check() error {
	switch {
	case s.Host == "":
		return errors.New("the mail server needs a host")
	case s.Port < 1 || s.Port > 65535:
		return errors.New("the mail server needs a port, such as 587")
	case s.TLS != "starttls" && s.TLS != "tls" && s.TLS != "none":
		return errors.New("tls is starttls, tls or none")
	}
	if _, err := mail.ParseAddress(s.From); err != nil {
		return fmt.Errorf("the sender %q is not an email address", s.From)
	}
	return nil
}

// Email sends messages by email.
type Email struct {
	Server SMTP
	To     []string
	// Timeout bounds the whole conversation with the server.
	Timeout time.Duration
}

// CheckAddresses says which of a list of recipients is not an address.
func CheckAddresses(to []string) error {
	if len(to) == 0 {
		return errors.New("an email channel needs at least one recipient")
	}
	for _, a := range to {
		if _, err := mail.ParseAddress(a); err != nil || strings.ContainsAny(a, "\r\n") {
			return fmt.Errorf("%q is not an email address", a)
		}
	}
	return nil
}

func (e Email) Send(ctx context.Context, m Message) error {
	s := e.Server
	timeout := e.Timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	addr := net.JoinHostPort(s.Host, strconv.Itoa(s.Port))
	tlsConfig := &tls.Config{ServerName: s.Host, MinVersion: tls.VersionTLS12}
	var conn net.Conn
	var err error
	if s.TLS == "tls" {
		conn, err = (&tls.Dialer{Config: tlsConfig}).DialContext(ctx, "tcp", addr)
	} else {
		conn, err = (&net.Dialer{}).DialContext(ctx, "tcp", addr)
	}
	if err != nil {
		return err
	}
	defer conn.Close()
	if deadline, ok := ctx.Deadline(); ok {
		conn.SetDeadline(deadline)
	}
	c, err := smtp.NewClient(conn, s.Host)
	if err != nil {
		return err
	}
	defer c.Close()
	if err := c.Hello("lotse"); err != nil {
		return err
	}
	if s.TLS == "starttls" {
		if ok, _ := c.Extension("STARTTLS"); !ok {
			return errors.New("the mail server does not offer STARTTLS; set tls to tls for port 465, or none")
		}
		if err := c.StartTLS(tlsConfig); err != nil {
			return err
		}
	}
	if s.Username != "" {
		// PlainAuth refuses to send the password unencrypted, unless to
		// this machine.
		if err := c.Auth(smtp.PlainAuth("", s.Username, s.Password, s.Host)); err != nil {
			return err
		}
	}
	from, err := mail.ParseAddress(s.From)
	if err != nil {
		return err
	}
	if err := c.Mail(from.Address); err != nil {
		return err
	}
	for _, to := range e.To {
		a, err := mail.ParseAddress(to)
		if err != nil {
			return err
		}
		if err := c.Rcpt(a.Address); err != nil {
			return err
		}
	}
	w, err := c.Data()
	if err != nil {
		return err
	}
	if _, err := w.Write(e.message(from, m)); err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	return c.Quit()
}

// message writes the email: plain text, quoted-printable.
func (e Email) message(from *mail.Address, m Message) []byte {
	var b bytes.Buffer
	id := make([]byte, 12)
	rand.Read(id)
	domain := from.Address[strings.LastIndex(from.Address, "@")+1:]
	header := func(k, v string) { fmt.Fprintf(&b, "%s: %s\r\n", k, v) }
	header("From", from.String())
	header("To", strings.Join(e.To, ", "))
	header("Subject", mime.QEncoding.Encode("utf-8", m.Title()))
	header("Date", time.Now().Format(time.RFC1123Z))
	header("Message-ID", "<"+hex.EncodeToString(id)+"@"+domain+">")
	header("Auto-Submitted", "auto-generated")
	header("MIME-Version", "1.0")
	header("Content-Type", "text/plain; charset=utf-8")
	header("Content-Transfer-Encoding", "quoted-printable")
	b.WriteString("\r\n")
	qp := quotedprintable.NewWriter(&b)
	qp.Write([]byte(strings.ReplaceAll(m.Text(), "\n", "\r\n")))
	qp.Close()
	return b.Bytes()
}
