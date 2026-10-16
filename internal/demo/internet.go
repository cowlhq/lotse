package main

import (
	"bufio"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/binary"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"log/slog"
	"math/big"
	mrand "math/rand/v2"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// The made-up internet: everything the checks and channels reach is served
// by the demo itself, so that nothing leaves the machine and every run
// looks the same.
//
//   - Web sites: HTTP_PROXY and HTTPS_PROXY point to the demo, which plays
//     every site, whatever its name. Their certificates come from a CA of
//     the demo's own, which SSL_CERT_FILE makes the only one trusted.
//   - DNS: a resolver that knows the made-up names; DNS checks name it.
//   - TLS and TCP checks connect to names under .localhost, which resolve
//     to this machine.
//   - Mail goes to an SMTP server that takes everything and sends nothing,
//     Slack messages to an address of the demo.

type internet struct {
	log   *slog.Logger
	start time.Time
	// version is what the made-up release feed calls the newest release.
	version string

	ca    *x509.Certificate
	caKey *ecdsa.PrivateKey
	certs sync.Map // name → *tls.Certificate

	// web is the proxy, and the demo's own pages: http://127.0.0.1:port.
	web   string
	https string // where the proxy sends HTTPS
	dns   string
	smtp  int
	// mx and ssh are the addresses of the TLS and TCP checks.
	mx, ssh string

	mails, hooks atomic.Int64
}

// startInternet starts the services on ports from base on (any free ones
// for 0), and points this process's HTTP and TLS at them. It must run
// before anything in the process makes an HTTPS request.
func startInternet(ctx context.Context, base int, dir string, start time.Time, version string, log *slog.Logger) (*internet, error) {
	n := &internet{log: log, start: start, version: version}
	if err := n.makeCA(dir); err != nil {
		return nil, err
	}
	port := func(i int) string {
		if base == 0 {
			return "127.0.0.1:0"
		}
		return fmt.Sprintf("127.0.0.1:%d", base+i)
	}

	webLn, err := net.Listen("tcp", port(0))
	if err != nil {
		return nil, err
	}
	httpsLn, err := net.Listen("tcp", port(1))
	if err != nil {
		return nil, err
	}
	dnsConn, err := net.ListenPacket("udp", port(2))
	if err != nil {
		return nil, err
	}
	smtpLn, err := net.Listen("tcp", port(3))
	if err != nil {
		return nil, err
	}
	mxLns, err := listenBoth(port(4))
	if err != nil {
		return nil, err
	}
	sshLns, err := listenBoth(port(5))
	if err != nil {
		return nil, err
	}
	n.web = "http://" + webLn.Addr().String()
	n.https = httpsLn.Addr().String()
	n.dns = dnsConn.LocalAddr().String()
	n.smtp = smtpLn.Addr().(*net.TCPAddr).Port
	n.mx = fmt.Sprintf("mx.ops.localhost:%d", mxLns[0].Addr().(*net.TCPAddr).Port)
	n.ssh = fmt.Sprintf("backup-01.ops.localhost:%d", sshLns[0].Addr().(*net.TCPAddr).Port)

	os.Setenv("SSL_CERT_FILE", filepath.Join(dir, "demo-ca.pem"))
	os.Unsetenv("SSL_CERT_DIR")
	os.Setenv("HTTP_PROXY", n.web)
	os.Setenv("HTTPS_PROXY", n.web)
	os.Setenv("NO_PROXY", "localhost,127.0.0.1,::1")

	tlsConfig := &tls.Config{GetCertificate: func(h *tls.ClientHelloInfo) (*tls.Certificate, error) {
		return n.cert(h.ServerName)
	}}
	serve := func(ln net.Listener, h http.Handler) {
		s := &http.Server{Handler: h, ReadHeaderTimeout: 10 * time.Second, ErrorLog: slog.NewLogLogger(log.Handler(), slog.LevelDebug)}
		go s.Serve(ln)
		go func() {
			<-ctx.Done()
			s.Close()
		}()
	}
	serve(webLn, http.HandlerFunc(n.proxy))
	serve(tls.NewListener(httpsLn, tlsConfig), http.HandlerFunc(n.site))
	go n.serveDNS(ctx, dnsConn)
	accept(ctx, []net.Listener{smtpLn}, n.smtpSession)
	accept(ctx, mxLns, func(c net.Conn) {
		// A mail relay on port 465: TLS first, then its greeting.
		time.Sleep(15 * time.Millisecond)
		t := tls.Server(c, tlsConfig)
		t.SetDeadline(time.Now().Add(5 * time.Second))
		if t.Handshake() == nil {
			io.WriteString(t, "220 mx.ops.localhost ESMTP\r\n")
		}
	})
	accept(ctx, sshLns, func(c net.Conn) {
		io.WriteString(c, "SSH-2.0-OpenSSH_9.2p1 Debian-2+deb12u3\r\n")
	})
	return n, nil
}

// listenBoth listens on addr and, if it can, on the same port of ::1:
// names under .localhost resolve to both.
func listenBoth(addr string) ([]net.Listener, error) {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, err
	}
	lns := []net.Listener{ln}
	if v6, err := net.Listen("tcp", fmt.Sprintf("[::1]:%d", ln.Addr().(*net.TCPAddr).Port)); err == nil {
		lns = append(lns, v6)
	}
	return lns, nil
}

// accept runs handle for every connection, and closes it afterwards.
func accept(ctx context.Context, lns []net.Listener, handle func(net.Conn)) {
	for _, ln := range lns {
		go func() {
			<-ctx.Done()
			ln.Close()
		}()
		go func() {
			for {
				c, err := ln.Accept()
				if err != nil {
					return
				}
				go func() {
					defer c.Close()
					handle(c)
				}()
			}
		}()
	}
}

func (n *internet) makeCA(dir string) error {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return err
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "lotse demo CA", Organization: []string{"lotse demo"}},
		NotBefore:             n.start.AddDate(-1, 0, 0),
		NotAfter:              n.start.AddDate(5, 0, 0),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		return err
	}
	n.ca, _ = x509.ParseCertificate(der)
	n.caKey = key
	return os.WriteFile(filepath.Join(dir, "demo-ca.pem"), pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o644)
}

// certNotAfter is when the certificate of a name expires.
func (n *internet) certNotAfter(name string) time.Time {
	days := 90
	if s, ok := sites[name]; ok {
		days = s.CertDays
	} else if d, ok := certDays[name]; ok {
		days = d
	}
	// Certificates are renewed at odd times of day.
	return n.start.Truncate(time.Hour).Add(time.Duration(days)*24*time.Hour + 7*time.Hour + 13*time.Minute)
}

// cert is the certificate of a name, made the first time it is asked for.
func (n *internet) cert(name string) (*tls.Certificate, error) {
	if name == "" {
		name = "localhost"
	}
	if c, ok := n.certs.Load(name); ok {
		return c.(*tls.Certificate), nil
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}
	notAfter := n.certNotAfter(name)
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(int64(mix(seed(name)) >> 2)),
		Subject:      pkix.Name{CommonName: name},
		DNSNames:     []string{name},
		NotBefore:    notAfter.Add(-90 * 24 * time.Hour),
		NotAfter:     notAfter,
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, n.ca, &key.PublicKey, n.caKey)
	if err != nil {
		return nil, err
	}
	c := &tls.Certificate{Certificate: [][]byte{der, n.ca.Raw}, PrivateKey: key}
	got, _ := n.certs.LoadOrStore(name, c)
	return got.(*tls.Certificate), nil
}

// proxy is where HTTP_PROXY points: it tunnels HTTPS to the demo's sites,
// answers plain HTTP for them itself, and serves the demo's own pages.
func (n *internet) proxy(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.Method == http.MethodConnect:
		n.tunnel(w, r)
	case r.URL.IsAbs():
		n.site(w, r)
	case r.URL.Path == "/releases/latest":
		// What the hub reads to learn of new releases: this one.
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"tag_name": "v" + n.version, "body": "", "html_url": "https://github.com/cowlhq/lotse/releases/tag/v" + n.version,
			"published_at": n.start.Add(-52 * time.Hour).UTC().Format(time.RFC3339),
		})
	case r.URL.Path == "/hooks/slack":
		io.Copy(io.Discard, r.Body)
		n.hooks.Add(1)
		io.WriteString(w, "ok")
	default:
		http.NotFound(w, r)
	}
}

func (n *internet) tunnel(w http.ResponseWriter, r *http.Request) {
	upstream, err := net.DialTimeout("tcp", n.https, 5*time.Second)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	defer upstream.Close()
	hj, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "cannot tunnel", http.StatusInternalServerError)
		return
	}
	conn, buf, err := hj.Hijack()
	if err != nil {
		return
	}
	defer conn.Close()
	io.WriteString(conn, "HTTP/1.1 200 Connection established\r\n\r\n")
	done := make(chan struct{}, 2)
	go func() {
		io.Copy(upstream, buf)
		if c, ok := upstream.(*net.TCPConn); ok {
			c.CloseWrite()
		}
		done <- struct{}{}
	}()
	go func() {
		io.Copy(conn, upstream)
		done <- struct{}{}
	}()
	<-done
	<-done
}

// site plays the made-up web sites, by the name they are asked for.
func (n *internet) site(w http.ResponseWriter, r *http.Request) {
	host := r.Host
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	host = strings.ToLower(host)
	s, ok := sites[host]
	if !ok {
		http.Error(w, "the demo plays no site of this name", http.StatusBadGateway)
		return
	}
	select {
	case <-time.After(s.latency(time.Now(), mrand.Float64()*2-1)):
	case <-r.Context().Done():
		return
	}
	status := s.Status
	if status == 0 {
		status = http.StatusOK
	}
	switch {
	case strings.HasSuffix(r.URL.Path, "/health") && status == http.StatusOK:
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		if r.URL.Path == "/health" {
			io.WriteString(w, "ok\n")
		} else {
			io.WriteString(w, `{"status":"ok"}`)
		}
	case strings.HasPrefix(s.Body, "{"):
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		io.WriteString(w, s.Body)
	default:
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(status)
		fmt.Fprintf(w, "<!doctype html><title>%s</title>%s\n", host, s.Body)
	}
}

// serveDNS answers A queries for the made-up names until ctx ends.
func (n *internet) serveDNS(ctx context.Context, conn net.PacketConn) {
	go func() {
		<-ctx.Done()
		conn.Close()
	}()
	buf := make([]byte, 1500)
	for {
		k, addr, err := conn.ReadFrom(buf)
		if err != nil {
			return
		}
		q := append([]byte(nil), buf[:k]...)
		go func() {
			// A resolver some way off.
			time.Sleep(time.Duration(8+mrand.IntN(8)) * time.Millisecond)
			if a := dnsAnswer(q); a != nil {
				conn.WriteTo(a, addr)
			}
		}()
	}
}

// dnsAnswer answers a DNS query (RFC 1035) from dnsRecords: A records for
// the names it knows, no records of other types, and no such name for the
// rest.
func dnsAnswer(q []byte) []byte {
	if len(q) < 12 || binary.BigEndian.Uint16(q[4:6]) != 1 {
		return nil
	}
	i := 12
	var labels []string
	for {
		if i >= len(q) {
			return nil
		}
		l := int(q[i])
		i++
		if l == 0 {
			break
		}
		if l&0xc0 != 0 || i+l > len(q) {
			return nil
		}
		labels = append(labels, string(q[i:i+l]))
		i += l
	}
	if i+4 > len(q) {
		return nil
	}
	qtype := binary.BigEndian.Uint16(q[i : i+2])
	question := q[12 : i+4]
	ips, known := dnsRecords[strings.ToLower(strings.Join(labels, "."))]
	var rcode byte
	if !known {
		rcode = 3
	}
	var answers [][4]byte
	if known && qtype == 1 {
		for _, s := range ips {
			var a [4]byte
			copy(a[:], net.ParseIP(s).To4())
			answers = append(answers, a)
		}
	}
	out := []byte{q[0], q[1], 0x84 | q[2]&0x01, 0x80 | rcode, 0, 1, 0, byte(len(answers)), 0, 0, 0, 0}
	out = append(out, question...)
	for _, a := range answers {
		out = append(out, 0xc0, 0x0c, 0, 1, 0, 1, 0, 0, 0x01, 0x2c, 0, 4)
		out = append(out, a[:]...)
	}
	return out
}

// smtpSession takes a mail and drops it.
func (n *internet) smtpSession(c net.Conn) {
	c.SetDeadline(time.Now().Add(time.Minute))
	r := bufio.NewReader(c)
	reply := func(s string) { io.WriteString(c, s+"\r\n") }
	reply("220 mail.ops.localhost ESMTP")
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return
		}
		cmd := strings.ToUpper(strings.TrimSpace(line))
		switch {
		case strings.HasPrefix(cmd, "EHLO"):
			reply("250-mail.ops.localhost\r\n250-8BITMIME\r\n250 SIZE 10485760")
		case strings.HasPrefix(cmd, "HELO"), strings.HasPrefix(cmd, "MAIL"), strings.HasPrefix(cmd, "RCPT"),
			strings.HasPrefix(cmd, "RSET"), strings.HasPrefix(cmd, "NOOP"):
			reply("250 OK")
		case cmd == "DATA":
			reply("354 go ahead")
			for {
				l, err := r.ReadString('\n')
				if err != nil {
					return
				}
				if strings.TrimRight(l, "\r\n") == "." {
					break
				}
			}
			n.mails.Add(1)
			reply("250 OK, kept to ourselves")
		case cmd == "QUIT":
			reply("221 bye")
			return
		default:
			reply("502 not here")
		}
	}
}
