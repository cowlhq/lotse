package hub

import (
	"context"
	"errors"
	"time"

	"github.com/coder/websocket"

	"lotse.sh/internal/auth"
	"lotse.sh/internal/hub/api"
	"lotse.sh/internal/store"
)

func (s apiServer) environment(ctx context.Context, id int64) (store.Scope, error) {
	env, err := s.db.Scope(ctx, id)
	if err == nil && env.Kind != "environment" {
		err = store.ErrNotFound
	}
	return env, err
}

func (s apiServer) enrollment(t store.EnrollmentToken) (api.Enrollment, error) {
	token, err := s.box.Open(t.Sealed)
	if err != nil {
		return api.Enrollment{}, err
	}
	return api.Enrollment{Token: token, CreatedAt: t.CreatedAt, CreatedBy: t.CreatedBy, Hub: s.URL()}, nil
}

func (s apiServer) GetEnrollment(ctx context.Context, req api.GetEnrollmentRequestObject) (api.GetEnrollmentResponseObject, error) {
	if !isAdmin(ctx) {
		return api.GetEnrollment403JSONResponse{Error: forbidden}, nil
	}
	if _, err := s.environment(ctx, req.Id); errors.Is(err, store.ErrNotFound) {
		return api.GetEnrollment404JSONResponse{Error: "no such environment"}, nil
	}
	t, err := s.db.EnrollmentToken(ctx, req.Id)
	if errors.Is(err, store.ErrNotFound) {
		return api.GetEnrollment404JSONResponse{Error: "this environment has no enrollment token yet"}, nil
	}
	if err != nil {
		return nil, err
	}
	e, err := s.enrollment(t)
	if err != nil {
		return nil, err
	}
	return api.GetEnrollment200JSONResponse(e), nil
}

func (s apiServer) NewEnrollment(ctx context.Context, req api.NewEnrollmentRequestObject) (api.NewEnrollmentResponseObject, error) {
	if !isAdmin(ctx) {
		return api.NewEnrollment403JSONResponse{Error: forbidden}, nil
	}
	if _, err := s.environment(ctx, req.Id); errors.Is(err, store.ErrNotFound) {
		return api.NewEnrollment404JSONResponse{Error: "no such environment"}, nil
	}
	token, hash := auth.NewToken("lotse_enr_")
	if err := s.db.NewEnrollmentToken(ctx, actor(ctx), req.Id, hash, s.box.Seal(token), time.Now().Unix()); err != nil {
		return nil, err
	}
	t, err := s.db.EnrollmentToken(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	e, err := s.enrollment(t)
	if err != nil {
		return nil, err
	}
	return api.NewEnrollment200JSONResponse(e), nil
}

func (s apiServer) RevokeAgent(ctx context.Context, req api.RevokeAgentRequestObject) (api.RevokeAgentResponseObject, error) {
	if !isAdmin(ctx) {
		return api.RevokeAgent403JSONResponse{Error: forbidden}, nil
	}
	err := s.db.RevokeAgents(ctx, actor(ctx), req.Id, time.Now().Unix())
	if errors.Is(err, store.ErrNotFound) {
		return api.RevokeAgent404JSONResponse{Error: "no such host"}, nil
	}
	if err != nil {
		return nil, err
	}
	for _, c := range s.agents.of(req.Id) {
		c.Close(websocket.StatusPolicyViolation, "this agent's credential was revoked")
	}
	return api.RevokeAgent204Response{}, nil
}
