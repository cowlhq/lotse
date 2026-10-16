package hub

import (
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"strings"
	"testing"

	"lotse.sh/internal/hub/api"
)

// An admin invites a viewer, who chooses a password with the link and is
// logged in; the viewer may look but not manage users, and the link works
// once.
func TestInvitation(t *testing.T) {
	_, server := newTestHub(t)
	admin := server.Client().Jar
	for _, bad := range []string{
		`{"email":"not an address","role":"viewer"}`,
		`{"email":"a@example.com","role":"owner"}`,
	} {
		if status, body := do(t, server, "POST", "/api/users", bad); status != http.StatusBadRequest {
			t.Errorf("%s: %d %s", bad, status, body)
		}
	}
	status, body := do(t, server, "POST", "/api/users", `{"email":" Bea@Example.com ","name":"Bea","role":"viewer"}`)
	if status != http.StatusCreated {
		t.Fatalf("invite: %d %s", status, body)
	}
	var inv api.UserLink
	json.Unmarshal([]byte(body), &inv)
	if inv.User.Email != "bea@example.com" || inv.User.State != api.Invited || inv.Mailed || !strings.Contains(inv.Link, "/welcome#lotse_inv_") {
		t.Fatalf("invitation: %+v", inv)
	}
	if status, _ := do(t, server, "POST", "/api/users", `{"email":"bea@example.com","role":"admin"}`); status != http.StatusConflict {
		t.Errorf("inviting the same address again: %d", status)
	}
	token := inv.Link[strings.Index(inv.Link, "#")+1:]

	// The invited person, in a browser of their own.
	server.Client().Jar, _ = cookiejar.New(nil)
	if status, _ := do(t, server, "POST", "/api/auth/login", `{"email":"bea@example.com","password":""}`); status != http.StatusUnauthorized {
		t.Errorf("logging in before choosing a password: %d", status)
	}
	if status, body := do(t, server, "POST", "/api/auth/link", `{"token":"`+token+`"}`); status != 200 || !strings.Contains(body, `"invited":true`) {
		t.Errorf("check the link: %d %s", status, body)
	}
	if status, _ := do(t, server, "POST", "/api/auth/link/password", `{"token":"`+token+`","password":"short"}`); status != http.StatusBadRequest {
		t.Errorf("a short password: %d", status)
	}
	if status, body := do(t, server, "POST", "/api/auth/link/password", `{"token":"`+token+`","name":"Bea B.","password":"bea's password"}`); status != 200 || !strings.Contains(body, `"state":"active"`) {
		t.Fatalf("choose a password: %d %s", status, body)
	}
	if status, body := do(t, server, "GET", "/api/auth/me", ""); status != 200 || !strings.Contains(body, `"name":"Bea B."`) {
		t.Errorf("me: %d %s", status, body)
	}
	if status, _ := do(t, server, "GET", "/api/hosts", ""); status != 200 {
		t.Errorf("a viewer looks: %d", status)
	}
	if status, _ := do(t, server, "GET", "/api/users", ""); status != http.StatusForbidden {
		t.Errorf("a viewer lists users: %d", status)
	}
	if status, _ := do(t, server, "POST", "/api/users", `{"email":"c@example.com","role":"admin"}`); status != http.StatusForbidden {
		t.Errorf("a viewer invites: %d", status)
	}
	if status, _ := do(t, server, "POST", "/api/auth/link/password", `{"token":"`+token+`","password":"another password"}`); status != http.StatusNotFound {
		t.Errorf("the link a second time: %d", status)
	}

	server.Client().Jar = admin
	var users []api.User
	getJSON(t, server, "/api/users", &users)
	if len(users) != 2 || users[1].State != api.Active || users[1].LastLoginAt == nil {
		t.Errorf("users: %+v", users)
	}
}

// The hub keeps an admin who can log in, nobody changes their own role, a
// disabled user is logged out, and a new link replaces the old one.
func TestManageUsers(t *testing.T) {
	_, server := newTestHub(t)
	admin := server.Client().Jar
	var me api.User
	getJSON(t, server, "/api/auth/me", &me)
	id := func(v api.User) string { b, _ := json.Marshal(v.Id); return string(b) }

	if status, _ := do(t, server, "PATCH", "/api/users/"+id(me), `{"role":"viewer"}`); status != http.StatusBadRequest {
		t.Errorf("demoting oneself: %d", status)
	}
	if status, _ := do(t, server, "PATCH", "/api/users/"+id(me), `{"disabled":true}`); status != http.StatusBadRequest {
		t.Errorf("disabling oneself: %d", status)
	}

	// A second admin, invited and accepted.
	_, body := do(t, server, "POST", "/api/users", `{"email":"bo@example.com","role":"admin"}`)
	var inv api.UserLink
	json.Unmarshal([]byte(body), &inv)
	first := inv.Link[strings.Index(inv.Link, "#")+1:]
	_, body = do(t, server, "POST", "/api/users/"+id(inv.User)+"/link", "")
	json.Unmarshal([]byte(body), &inv)
	second := inv.Link[strings.Index(inv.Link, "#")+1:]
	server.Client().Jar, _ = cookiejar.New(nil)
	if status, _ := do(t, server, "POST", "/api/auth/link", `{"token":"`+first+`"}`); status != http.StatusNotFound {
		t.Errorf("a replaced link: %d", status)
	}
	if status, body := do(t, server, "POST", "/api/auth/link/password", `{"token":"`+second+`","password":"bo's password!"}`); status != 200 {
		t.Fatalf("accept: %d %s", status, body)
	}
	bo := server.Client().Jar

	// Bo disables the first admin, who is logged out; then Bo cannot be
	// demoted, being the last admin.
	if status, body := do(t, server, "PATCH", "/api/users/"+id(me), `{"disabled":true}`); status != 200 || !strings.Contains(body, `"state":"disabled"`) {
		t.Fatalf("disable: %d %s", status, body)
	}
	server.Client().Jar = admin
	if status, _ := do(t, server, "GET", "/api/hosts", ""); status != http.StatusUnauthorized {
		t.Errorf("a disabled user's session: %d", status)
	}
	server.Client().Jar = bo
	var bob api.User
	getJSON(t, server, "/api/auth/me", &bob)
	_, body = do(t, server, "POST", "/api/users", `{"email":"cy@example.com","role":"admin"}`)
	json.Unmarshal([]byte(body), &inv)
	if status, body := do(t, server, "PATCH", "/api/users/"+id(me), `{"role":"viewer"}`); status != 200 {
		t.Errorf("demote a disabled admin: %d %s", status, body)
	}
	// Cy is invited as admin but cannot log in yet, so does not count.
	if status, _ := do(t, server, "DELETE", "/api/users/"+id(inv.User), ""); status != http.StatusNoContent {
		t.Errorf("withdraw: %d", status)
	}
	if status, _ := do(t, server, "DELETE", "/api/users/"+id(bob), ""); status != http.StatusBadRequest {
		t.Errorf("withdraw an accepted user: %d", status)
	}

	// Bo changes the password, with the current one.
	if status, _ := do(t, server, "PATCH", "/api/auth/me", `{"password":"a new password","current_password":"wrong"}`); status != http.StatusBadRequest {
		t.Errorf("a wrong current password: %d", status)
	}
	if status, body := do(t, server, "PATCH", "/api/auth/me", `{"password":"a new password","current_password":"bo's password!","name":"Bo"}`); status != 200 || !strings.Contains(body, `"name":"Bo"`) {
		t.Errorf("change: %d %s", status, body)
	}
	if status, _ := do(t, server, "GET", "/api/hosts", ""); status != 200 {
		t.Errorf("the session that changed the password goes on: %d", status)
	}
	server.Client().Jar, _ = cookiejar.New(nil)
	if status, _ := do(t, server, "POST", "/api/auth/login", `{"email":"bo@example.com","password":"a new password"}`); status != 200 {
		t.Errorf("login with the new password: %d", status)
	}
}
