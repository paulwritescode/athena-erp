package server

import (
	"context"
	"database/sql"
	"errors"

	v1 "athena-erp/gen/athena/v1"
	"athena-erp/internal/auth"
	"athena-erp/internal/notifications"
	"connectrpc.com/connect/v2"
)

type notificationService struct{ db *sql.DB }

func inboxOwner(ctx context.Context) (string, error) {
	p := auth.PrincipalFrom(ctx)
	if p == nil || p.UserID == "" {
		return "", connect.NewError(connect.CodeUnauthenticated, "Sign in to open your notifications.")
	}
	return p.UserID, nil
}
func inboxError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, notifications.ErrInput) {
		return connect.NewError(connect.CodeInvalidArgument, err.Error())
	}
	return internalError("open notifications", err)
}
func (s notificationService) List(ctx context.Context, r *v1.ListNotificationsRequest) (*v1.ListNotificationsResponse, error) {
	id, err := inboxOwner(ctx)
	if err != nil {
		return nil, err
	}
	page, err := notifications.List(ctx, s.db, id, r.GetPageSize(), r.GetPageToken(), r.GetUnreadOnly())
	return page, inboxError(err)
}
func (s notificationService) UnreadCount(ctx context.Context, _ *v1.UnreadCountRequest) (*v1.UnreadCountResponse, error) {
	id, err := inboxOwner(ctx)
	if err != nil {
		return nil, err
	}
	count, err := notifications.UnreadCount(ctx, s.db, id)
	if err != nil {
		return nil, inboxError(err)
	}
	return &v1.UnreadCountResponse{Count: count}, nil
}
func (s notificationService) MarkRead(ctx context.Context, r *v1.MarkReadRequest) (_ *v1.MarkReadResponse, err error) {
	ctx, _, end := startKernelSpan(ctx, "mark_notifications_read", "")
	defer func() { end(err) }()
	id, err := inboxOwner(ctx)
	if err != nil {
		return nil, err
	}
	changed, err := notifications.MarkRead(ctx, s.db, id, r.GetIds())
	if err != nil {
		return nil, inboxError(err)
	}
	return &v1.MarkReadResponse{Changed: changed}, nil
}
func (s notificationService) MarkAllRead(ctx context.Context, _ *v1.MarkAllReadRequest) (_ *v1.MarkAllReadResponse, err error) {
	ctx, _, end := startKernelSpan(ctx, "mark_all_notifications_read", "")
	defer func() { end(err) }()
	id, err := inboxOwner(ctx)
	if err != nil {
		return nil, err
	}
	changed, err := notifications.MarkAllRead(ctx, s.db, id)
	if err != nil {
		return nil, inboxError(err)
	}
	return &v1.MarkAllReadResponse{Changed: changed}, nil
}
