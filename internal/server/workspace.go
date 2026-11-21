package server

import (
	"context"
	"database/sql"
	"errors"

	v1 "athena-erp/gen/athena/v1"
	"athena-erp/internal/appearance"
	"athena-erp/internal/auth"
	"athena-erp/internal/ledger"
	"connectrpc.com/connect/v2"
	"connectrpc.com/connect/v2/connectproto"
)

type workspaceService struct{ db *sql.DB }

func (s workspaceService) GetAppearance(ctx context.Context, _ *v1.GetAppearanceRequest) (*v1.GetAppearanceResponse, error) {
	svc := ledger.Service{DB: s.db, Dialect: dialect}
	a, palette, err := svc.GetAppearance(ctx)
	if err != nil {
		return nil, internalError("read Workspace appearance", err)
	}
	return &v1.GetAppearanceResponse{Appearance: a, DarkAccent: palette.Dark, LightAccent: palette.Light}, nil
}
func (s workspaceService) SetAppearance(ctx context.Context, req *v1.SetAppearanceRequest) (_ *v1.SetAppearanceResponse, err error) {
	ctx, _, end := startKernelSpan(ctx, "set_appearance", "")
	defer func() { end(err) }()
	if _, err := requireSystem(ctx, auth.PermissionAppearance); err != nil {
		return nil, err
	}
	svc := ledger.Service{DB: s.db, Dialect: dialect}
	palette, err := svc.SetAppearance(ctx, req.GetAppearance(), actorName(ctx))
	var contrast *appearance.ContrastError
	switch {
	case errors.As(err, &contrast):
		failure := connect.NewError(connect.CodeInvalidArgument, contrast.Error())
		detail, e := connectproto.NewErrorDetail(&v1.AppearanceContrastFailure{SuggestedAccent: contrast.Suggested, DarkAccent: contrast.Dark})
		if e != nil {
			return nil, internalError("describe contrast", e)
		}
		return nil, failure.WithDetail(detail)
	case errors.Is(err, appearance.ErrInvalid):
		return nil, connect.NewError(connect.CodeInvalidArgument, err.Error())
	case err != nil:
		return nil, internalError("save Workspace appearance", err)
	}
	return &v1.SetAppearanceResponse{Appearance: req.Appearance, DarkAccent: palette.Dark, LightAccent: palette.Light}, nil
}
