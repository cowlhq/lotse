package hub

import (
	"context"
	"errors"
	"fmt"
	"net/mail"
	"strings"
	"time"

	"lotse.sh/internal/auth"
	"lotse.sh/internal/hub/api"
	"lotse.sh/internal/notify"
	"lotse.sh/internal/store"
)

// Users (decision 0023): admins invite people with a link to choose a
// password with, and make new links for those who forgot theirs.

const (
	linkPrefix = "lotse_inv_"
	// A link works this long.
	linkLifetime = 7 * 24 * time.Hour
	maxUserName  = 80
)

func (s apiServer) ListUsers(ctx context.Context, _ api.ListUsersRequestObject) (api.ListUsersResponseObject, error) {
	if !isAdmin(ctx) {
		return api.ListUsers403JSONResponse{Error: forbidden}, nil
	}
	list, err := s.db.Users(ctx)
	if err != nil {
		return nil, err
	}
	out := make(api.ListUsers200JSONResponse, len(list))
	for i, u := range list {
		out[i] = toUser(u)
	}
	return out, nil
}

func checkEmail(email string) string {
	a, err := mail.ParseAddress(email)
	if err != nil || a.Address != email || strings.ContainsAny(email, "\r\n") {
		return "this is not an email address"
	}
	return ""
}

func checkUserName(name string) string {
	if len([]rune(name)) > maxUserName {
		return "a name has at most 80 characters"
	}
	return ""
}

func (s apiServer) InviteUser(ctx context.Context, req api.InviteUserRequestObject) (api.InviteUserResponseObject, error) {
	if !isAdmin(ctx) {
		return api.InviteUser403JSONResponse{Error: forbidden}, nil
	}
	b := req.Body
	email := strings.ToLower(strings.TrimSpace(b.Email))
	name := strings.TrimSpace(ptrValue(b.Name))
	if msg := cmpOr(checkEmail(email), checkUserName(name)); msg != "" {
		return api.InviteUser400JSONResponse{Error: msg}, nil
	}
	role := string(b.Role)
	if role != "admin" && role != "viewer" {
		return api.InviteUser400JSONResponse{Error: "the role is admin or viewer"}, nil
	}
	token, hash := auth.NewToken(linkPrefix)
	now := time.Now()
	u, err := s.db.InviteUser(ctx, actor(ctx), store.User{Email: email, Name: name, Role: role, CreatedAt: now.Unix()},
		hash, now.Add(linkLifetime).Unix())
	if errors.Is(err, store.ErrExists) {
		return api.InviteUser409JSONResponse{Error: "there is a user with this address already"}, nil
	}
	if err != nil {
		return nil, err
	}
	return api.InviteUser201JSONResponse(s.sendLink(ctx, u, token)), nil
}

func (s apiServer) NewUserLink(ctx context.Context, req api.NewUserLinkRequestObject) (api.NewUserLinkResponseObject, error) {
	if !isAdmin(ctx) {
		return api.NewUserLink403JSONResponse{Error: forbidden}, nil
	}
	if u, err := s.db.UserByID(ctx, req.Id); err == nil && u.DisabledAt != nil {
		return api.NewUserLink400JSONResponse{Error: "this user is disabled; enable them first"}, nil
	}
	token, hash := auth.NewToken(linkPrefix)
	u, err := s.db.SetUserLink(ctx, actor(ctx), req.Id, hash, time.Now().Add(linkLifetime).Unix())
	if errors.Is(err, store.ErrNotFound) {
		return api.NewUserLink404JSONResponse{Error: "no such user"}, nil
	}
	if err != nil {
		return nil, err
	}
	return api.NewUserLink200JSONResponse(s.sendLink(ctx, u, token)), nil
}

// sendLink makes the link for a user's token and, if the hub has a mail
// server, sends it to them.
func (s apiServer) sendLink(ctx context.Context, u store.User, token string) api.UserLink {
	link := s.URL() + "/welcome#" + token
	out := api.UserLink{User: toUser(u), Link: link}
	m, err := s.mailServer(ctx)
	if errors.Is(err, errNoMailServer) {
		return out
	}
	if err == nil {
		err = notify.Email{Server: m.SMTP, To: []string{u.Email}, Timeout: 20 * time.Second}.Send(ctx, linkMessage(s.URL(), actor(ctx).Name, u, link))
	}
	if err != nil {
		s.log.Warn("cannot send a link by email", "to", u.Email, "err", err)
		out.MailError = ptr(err.Error())
		return out
	}
	out.Mailed = true
	return out
}

func linkMessage(hub, from string, u store.User, link string) notify.Message {
	until := time.Unix(ptrValue(u.LinkExpiresAt), 0).UTC().Format("2 January 2006, 15:04 UTC")
	if u.Invited() {
		return notify.Message{
			Hub:     hub,
			Subject: "[lotse] You are invited to " + hub,
			Body: fmt.Sprintf("%s invites you to lotse at %s, as %s.\n\n"+
				"Choose your password here; the link works once, until %s:\n\n%s\n",
				from, hub, u.Role, until, link),
		}
	}
	return notify.Message{
		Hub:     hub,
		Subject: "[lotse] A new password for " + hub,
		Body: fmt.Sprintf("%s made you a link to choose a new password for %s at %s.\n\n"+
			"It works once, until %s:\n\n%s\n\nUntil you use it, your password stays as it is.\n",
			from, u.Email, hub, until, link),
	}
}

func (s apiServer) UpdateUser(ctx context.Context, req api.UpdateUserRequestObject) (api.UpdateUserResponseObject, error) {
	if !isAdmin(ctx) {
		return api.UpdateUser403JSONResponse{Error: forbidden}, nil
	}
	b := req.Body
	me, _ := userOf(ctx)
	if req.Id == me.ID && (b.Role != nil || b.Disabled != nil) {
		return api.UpdateUser400JSONResponse{Error: "you cannot change your own role, or disable yourself"}, nil
	}
	var c store.UserChange
	if b.Name != nil {
		name := strings.TrimSpace(*b.Name)
		if msg := checkUserName(name); msg != "" {
			return api.UpdateUser400JSONResponse{Error: msg}, nil
		}
		c.Name = &name
	}
	if b.Role != nil {
		role := string(*b.Role)
		if role != "admin" && role != "viewer" {
			return api.UpdateUser400JSONResponse{Error: "the role is admin or viewer"}, nil
		}
		c.Role = &role
	}
	c.Disabled = b.Disabled
	u, err := s.db.UpdateUser(ctx, actor(ctx), req.Id, c, time.Now().Unix())
	switch {
	case errors.Is(err, store.ErrNotFound):
		return api.UpdateUser404JSONResponse{Error: "no such user"}, nil
	case errors.Is(err, store.ErrLastAdmin):
		return api.UpdateUser400JSONResponse{Error: "the hub needs at least one admin who can log in"}, nil
	case err != nil:
		return nil, err
	}
	return api.UpdateUser200JSONResponse(toUser(u)), nil
}

func (s apiServer) WithdrawInvitation(ctx context.Context, req api.WithdrawInvitationRequestObject) (api.WithdrawInvitationResponseObject, error) {
	if !isAdmin(ctx) {
		return api.WithdrawInvitation403JSONResponse{Error: forbidden}, nil
	}
	err := s.db.WithdrawInvitation(ctx, actor(ctx), req.Id)
	switch {
	case errors.Is(err, store.ErrNotFound):
		return api.WithdrawInvitation404JSONResponse{Error: "no such user"}, nil
	case errors.Is(err, store.ErrNotInvited):
		return api.WithdrawInvitation400JSONResponse{Error: "this user has accepted the invitation; disable them instead"}, nil
	}
	return api.WithdrawInvitation204Response{}, err
}

// linkKeys are what the brake on guessing counts for links: the address
// the guesses come from.
func linkKeys(ctx context.Context) []string { return []string{"ip:" + requestOf(ctx).ip} }

func (s apiServer) CheckLink(ctx context.Context, req api.CheckLinkRequestObject) (api.CheckLinkResponseObject, error) {
	keys := linkKeys(ctx)
	if !s.logins.Allowed(keys...) {
		return api.CheckLink429JSONResponse{Error: auth.ErrTooManyAttempts.Error()}, nil
	}
	u, err := s.db.UserByLink(ctx, auth.HashToken(strings.TrimSpace(req.Body.Token)), time.Now().Unix())
	if errors.Is(err, store.ErrNotFound) {
		s.logins.Failed(keys...)
		return api.CheckLink404JSONResponse{Error: deadLink}, nil
	}
	if err != nil {
		return nil, err
	}
	return api.CheckLink200JSONResponse{Email: u.Email, Name: u.Name, Invited: u.Invited()}, nil
}

const deadLink = "this link does not work: it was used, or replaced by a newer one, or is older than seven days"

func (s apiServer) UseLink(ctx context.Context, req api.UseLinkRequestObject) (api.UseLinkResponseObject, error) {
	keys := linkKeys(ctx)
	if !s.logins.Allowed(keys...) {
		return api.UseLink429JSONResponse{Error: auth.ErrTooManyAttempts.Error()}, nil
	}
	b := req.Body
	name := strings.TrimSpace(ptrValue(b.Name))
	if msg := checkUserName(name); msg != "" {
		return api.UseLink400JSONResponse{Error: msg}, nil
	}
	if err := auth.CheckNewPassword(b.Password); err != nil {
		return api.UseLink400JSONResponse{Error: err.Error()}, nil
	}
	hash, err := auth.HashPassword(b.Password)
	if err != nil {
		return nil, err
	}
	now := time.Now().Unix()
	u, err := s.db.UseLink(ctx, auth.HashToken(strings.TrimSpace(b.Token)), name, hash, now)
	if errors.Is(err, store.ErrNotFound) {
		s.logins.Failed(keys...)
		return api.UseLink404JSONResponse{Error: deadLink}, nil
	}
	if err != nil {
		return nil, err
	}
	s.logins.Succeeded(append(keys, "email:"+u.Email)...)
	cookie, err := s.startSession(ctx, u.ID)
	if err != nil {
		return nil, err
	}
	if err := s.db.NoteLogin(ctx, u.ID, now); err != nil {
		return nil, err
	}
	return api.UseLink200JSONResponse{Body: toUser(u), Headers: api.UseLink200ResponseHeaders{SetCookie: &cookie}}, nil
}

func (s apiServer) UpdateMe(ctx context.Context, req api.UpdateMeRequestObject) (api.UpdateMeResponseObject, error) {
	me, _ := userOf(ctx)
	b := req.Body
	if b.Password != nil {
		keys := []string{"email:" + me.Email}
		if !s.logins.Allowed(keys...) {
			return api.UpdateMe400JSONResponse{Error: auth.ErrTooManyAttempts.Error()}, nil
		}
		if !auth.CheckPassword(me.PasswordHash, ptrValue(b.CurrentPassword)) {
			s.logins.Failed(keys...)
			return api.UpdateMe400JSONResponse{Error: "the current password is not right"}, nil
		}
		if err := auth.CheckNewPassword(*b.Password); err != nil {
			return api.UpdateMe400JSONResponse{Error: err.Error()}, nil
		}
		hash, err := auth.HashPassword(*b.Password)
		if err != nil {
			return nil, err
		}
		if err := s.db.SetOwnPassword(ctx, me.ID, hash, requestOf(ctx).session); err != nil {
			return nil, err
		}
	}
	if b.Name != nil {
		name := strings.TrimSpace(*b.Name)
		if msg := checkUserName(name); msg != "" {
			return api.UpdateMe400JSONResponse{Error: msg}, nil
		}
		if _, err := s.db.UpdateUser(ctx, actor(ctx), me.ID, store.UserChange{Name: &name}, time.Now().Unix()); err != nil {
			return nil, err
		}
	}
	u, err := s.db.UserByID(ctx, me.ID)
	if err != nil {
		return nil, err
	}
	return api.UpdateMe200JSONResponse(toUser(u)), nil
}
