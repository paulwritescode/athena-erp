package server

import (
	"context"
	"database/sql"
	"errors"
	"sort"
	"time"

	athenav1 "athena-erp/gen/athena/v1"
	"athena-erp/internal/auth"
	"athena-erp/internal/files"

	"connectrpc.com/connect/v2"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// fileService is the FileService handler. The CLI uses the same type through
// Kernel.Files, so both apply the same path, size, type and version rules.
type fileService struct {
	files *files.Store
	db    *sql.DB
}

// newFileService returns a handler over the Workspace directory. Without a
// directory or a database every call fails with a clear message.
func newFileService(workspace *Workspace, db *sql.DB) fileService {
	s := fileService{db: db}
	if workspace == nil || workspace.Root == "" || db == nil {
		return s
	}
	store, err := files.Open(workspace.Root, db, dialect)
	if err != nil {
		return s
	}
	s.files = store
	return s
}

func (s fileService) ready() error {
	if s.files == nil {
		return connect.NewError(connect.CodeFailedPrecondition, "this server has no Workspace directory; start it with athena run")
	}
	return nil
}

// fileError maps a files package error to the response a caller should see.
func fileError(action string, err error) error {
	code := connect.CodeInvalidArgument
	switch {
	case err == nil:
		return nil
	case errors.Is(err, files.ErrNotFound):
		code = connect.CodeNotFound
	case errors.Is(err, files.ErrExists):
		code = connect.CodeAlreadyExists
	case errors.Is(err, files.ErrStale):
		code = connect.CodeAborted
	case errors.Is(err, files.ErrLimit):
		code = connect.CodeResourceExhausted
	case errors.Is(err, files.ErrInvalidPath), errors.Is(err, files.ErrTooLarge), errors.Is(err, files.ErrNotText), errors.Is(err, files.ErrNotImage), errors.Is(err, files.ErrInvalidFile):
	default:
		return internalError(action, err)
	}
	return connect.NewError(code, err.Error())
}

// fileOp opens the span and the request metadata of one file operation. The
// span carries the operation, request ID, a hashed path ID, byte and file
// counts and the outcome, never a path or content.
func fileOp(ctx context.Context, operation, path string) (context.Context, files.Meta, func(err error, bytes, count int)) {
	ctx, requestID, end := startKernelSpan(ctx, operation, "")
	_, source := requestFrom(ctx)
	if path != "" {
		trace.SpanFromContext(ctx).SetAttributes(attribute.String("athena.file.path_id", files.PathID(path)))
	}
	return ctx, files.Meta{Actor: actorName(ctx), Source: source, RequestID: requestID}, func(err error, bytes, count int) {
		trace.SpanFromContext(ctx).SetAttributes(attribute.Int("athena.file.bytes", bytes), attribute.Int("athena.file.count", count))
		end(err)
	}
}

func fileInfoMessage(info files.Info, backing *athenav1.FileBacking) *athenav1.FileInfo {
	return &athenav1.FileInfo{
		Path: info.Path, Size: info.Size, Hash: info.Hash, Revision: info.Revision,
		ModifiedAt: info.ModifiedAt.UTC().Format(time.RFC3339), ModifiedBy: info.ModifiedBy,
		Recorded: info.Recorded, Backing: backing,
	}
}

// backingOf names what a file backs and whether a saved Record Type exists.
func backingOf(path string, saved map[string]bool) *athenav1.FileBacking {
	kind, key := files.Backing(path)
	if kind == "" {
		return nil
	}
	return &athenav1.FileBacking{Kind: kind, Key: key, Saved: (kind == "record_type" || kind == "generated_source") && saved[key]}
}

func (s fileService) savedRecordTypes(ctx context.Context) (map[string]bool, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT name FROM athena_record_types")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	saved := map[string]bool{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		saved[name] = true
	}
	return saved, rows.Err()
}

func (s fileService) ListFiles(ctx context.Context, _ *athenav1.ListFilesRequest) (*athenav1.ListFilesResponse, error) {
	if err := s.ready(); err != nil {
		return nil, err
	}
	if _, err := requireSystem(ctx, auth.PermissionFilesRead); err != nil {
		return nil, err
	}
	ctx, _, end := fileOp(ctx, "file_list", "")
	res, count, err := s.list(ctx)
	end(err, 0, count)
	return res, err
}

func (s fileService) list(ctx context.Context) (*athenav1.ListFilesResponse, int, error) {
	infos, err := s.files.List(ctx)
	if err != nil {
		return nil, 0, fileError("list files", err)
	}
	saved, err := s.savedRecordTypes(ctx)
	if err != nil {
		return nil, 0, internalError("list files", err)
	}
	res := &athenav1.ListFilesResponse{MaxFileBytes: files.MaxFileBytes, MaxFiles: files.MaxFiles, Folders: files.Folders, Extensions: files.Extensions}
	backed := map[string]bool{}
	for _, info := range infos {
		backing := backingOf(info.Path, saved)
		if backing.GetKind() == "record_type" {
			backed[backing.GetKey()] = true
		}
		res.Files = append(res.Files, fileInfoMessage(info, backing))
	}
	for name := range saved {
		if !backed[name] {
			res.RecordTypesWithoutFile = append(res.RecordTypesWithoutFile, name)
		}
	}
	sort.Strings(res.RecordTypesWithoutFile)
	return res, len(infos), nil
}

func (s fileService) GetFile(ctx context.Context, req *athenav1.GetFileRequest) (*athenav1.GetFileResponse, error) {
	if err := s.ready(); err != nil {
		return nil, err
	}
	if _, err := requireSystem(ctx, auth.PermissionFilesRead); err != nil {
		return nil, err
	}
	ctx, _, end := fileOp(ctx, "file_get", req.GetPath())
	info, content, err := s.files.Get(ctx, req.GetPath(), req.GetRevision())
	if err == nil && req.GetThumbnail() {
		content, err = files.Thumbnail(content)
	}
	err = fileError("read the file", err)
	end(err, len(content), 1)
	if err != nil {
		return nil, err
	}
	return &athenav1.GetFileResponse{Info: fileInfoMessage(info, nil), Content: content}, nil
}

func (s fileService) PutFile(ctx context.Context, req *athenav1.PutFileRequest) (*athenav1.PutFileResponse, error) {
	if err := s.ready(); err != nil {
		return nil, err
	}
	if _, err := requireSystem(ctx, auth.PermissionFilesWrite); err != nil {
		return nil, err
	}
	ctx, meta, end := fileOp(ctx, "file_put", req.GetPath())
	info, err := s.files.Put(ctx, req.GetPath(), req.GetContent(), files.Expect{Hash: req.GetExpectedHash()}, meta)
	err = fileError("save the file", err)
	end(err, len(req.GetContent()), 1)
	if err != nil {
		return nil, err
	}
	return &athenav1.PutFileResponse{Info: fileInfoMessage(info, nil)}, nil
}

func (s fileService) PutImage(ctx context.Context, req *athenav1.PutImageRequest) (*athenav1.PutImageResponse, error) {
	if err := s.ready(); err != nil {
		return nil, err
	}
	if _, err := requireSystem(ctx, auth.PermissionFilesWrite); err != nil {
		return nil, err
	}
	ctx, meta, end := fileOp(ctx, "file_put_image", req.GetPath())
	result, err := s.files.PutImage(ctx, req.GetPath(), req.GetContent(), files.Expect{Hash: req.GetExpectedHash()}, meta)
	err = fileError("save the image", err)
	end(err, len(req.GetContent()), 1)
	if err != nil {
		return nil, err
	}
	return &athenav1.PutImageResponse{Info: fileInfoMessage(result.Info, nil), Width: int32(result.Width), Height: int32(result.Height)}, nil
}

func (s fileService) MoveFile(ctx context.Context, req *athenav1.MoveFileRequest) (*athenav1.MoveFileResponse, error) {
	if err := s.ready(); err != nil {
		return nil, err
	}
	if _, err := requireSystem(ctx, auth.PermissionFilesWrite); err != nil {
		return nil, err
	}
	ctx, meta, end := fileOp(ctx, "file_move", req.GetFrom())
	info, err := s.files.Move(ctx, req.GetFrom(), req.GetTo(), files.Expect{Hash: req.GetExpectedHash()}, meta)
	err = fileError("move the file", err)
	end(err, int(info.Size), 1)
	if err != nil {
		return nil, err
	}
	return &athenav1.MoveFileResponse{Info: fileInfoMessage(info, nil)}, nil
}

func (s fileService) DeleteFile(ctx context.Context, req *athenav1.DeleteFileRequest) (*athenav1.DeleteFileResponse, error) {
	if err := s.ready(); err != nil {
		return nil, err
	}
	if _, err := requireSystem(ctx, auth.PermissionFilesWrite); err != nil {
		return nil, err
	}
	ctx, meta, end := fileOp(ctx, "file_delete", req.GetPath())
	err := fileError("delete the file", s.files.Delete(ctx, req.GetPath(), files.Expect{Hash: req.GetExpectedHash()}, meta))
	end(err, 0, 1)
	if err != nil {
		return nil, err
	}
	return &athenav1.DeleteFileResponse{}, nil
}

func (s fileService) ListFileVersions(ctx context.Context, req *athenav1.ListFileVersionsRequest) (*athenav1.ListFileVersionsResponse, error) {
	if err := s.ready(); err != nil {
		return nil, err
	}
	if _, err := requireSystem(ctx, auth.PermissionFilesRead); err != nil {
		return nil, err
	}
	ctx, _, end := fileOp(ctx, "file_versions", req.GetPath())
	versions, err := s.files.Versions(ctx, req.GetPath())
	err = fileError("list file versions", err)
	end(err, 0, len(versions))
	if err != nil {
		return nil, err
	}
	res := &athenav1.ListFileVersionsResponse{}
	for _, v := range versions {
		res.Versions = append(res.Versions, &athenav1.FileVersion{
			Revision: v.Revision, Action: v.Action, Path: v.Path, Hash: v.Hash, Size: v.Size, Actor: v.Actor,
			Source: v.Source, RequestId: v.RequestID, CreatedAt: v.CreatedAt.UTC().Format(time.RFC3339), HasContent: v.HasContent,
		})
	}
	return res, nil
}
