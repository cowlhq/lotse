package auth

import (
	"strings"
	"testing"
	"time"
)

func TestPassword(t *testing.T) {
	hash, err := HashPassword("correct horse battery")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(hash, "$argon2id$v=19$m=19456,t=2,p=1$") {
		t.Errorf("hash %q", hash)
	}
	if !CheckPassword(hash, "correct horse battery") {
		t.Error("the right password is refused")
	}
	if CheckPassword(hash, "correct horse battery ") || CheckPassword(hash, "") {
		t.Error("a wrong password is accepted")
	}
	other, _ := HashPassword("correct horse battery")
	if other == hash {
		t.Error("two hashes of one password are the same: no salt")
	}
	for _, bad := range []string{"", "$argon2id$", "$bcrypt$x$y$z$w", hash[:len(hash)-4]} {
		if CheckPassword(bad, "correct horse battery") {
			t.Errorf("hash %q accepted", bad)
		}
	}
	if CheckNewPassword("short") == nil || CheckNewPassword("long enough") != nil {
		t.Error("CheckNewPassword")
	}
}

func TestToken(t *testing.T) {
	a, ha := NewToken("lotse_ses_")
	b, _ := NewToken("lotse_ses_")
	if a == b || !strings.HasPrefix(a, "lotse_ses_") || HashToken(a) != ha || ha == a {
		t.Errorf("tokens %q %q, hash %q", a, b, ha)
	}
}

func TestLimiter(t *testing.T) {
	now := time.Unix(0, 0)
	l := NewLimiter(3, time.Minute)
	l.Now = func() time.Time { return now }
	for range 3 {
		if !l.Allowed("ip", "mail") {
			t.Fatal("refused before the limit")
		}
		l.Failed("ip", "mail")
	}
	if l.Allowed("ip") || l.Allowed("other", "mail") {
		t.Error("allowed after three failures")
	}
	if !l.Allowed("other") {
		t.Error("an unrelated key is refused")
	}
	now = now.Add(61 * time.Second)
	if !l.Allowed("ip", "mail") {
		t.Error("refused after the window")
	}
	l.Failed("ip")
	l.Succeeded("ip")
	if len(l.failures["ip"]) != 0 {
		t.Error("success did not clear")
	}
}
