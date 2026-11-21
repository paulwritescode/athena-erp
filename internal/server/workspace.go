package server

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"unicode/utf8"

	v1 "athena-erp/gen/athena/v1"
	"athena-erp/internal/appearance"
	"athena-erp/internal/auth"
	"athena-erp/internal/brand"
	"athena-erp/internal/files"
	"athena-erp/internal/ledger"
	"connectrpc.com/connect/v2"
	"connectrpc.com/connect/v2/connectproto"
)

type workspaceService struct {
	db    *sql.DB
	name  string
	files *files.Store
}

// newWorkspaceService returns a handler over the database and the Workspace
// directory. Without a directory, logo reads and writes fail with a clear
// message while the display name still works.
func newWorkspaceService(workspace *Workspace, db *sql.DB) workspaceService {
	s := workspaceService{db: db}
	if workspace == nil {
		return s
	}
	s.name = workspace.Name
	if workspace.Root == "" || db == nil {
		return s
	}
	store, err := files.Open(workspace.Root, db, dialect)
	if err != nil {
		return s
	}
	s.files = store
	return s
}

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

func (s workspaceService) GetIdentity(ctx context.Context, _ *v1.GetIdentityRequest) (*v1.GetIdentityResponse, error) {
	svc := ledger.Service{DB: s.db, Dialect: dialect}
	doc, err := svc.GetIdentityDoc(ctx)
	if err != nil {
		return nil, internalError("read Workspace identity", err)
	}
	return &v1.GetIdentityResponse{Identity: identityMessage(s.name, doc)}, nil
}

// identityMessage builds the public identity: the stored name (or the
// Workspace name when none was set) and the content-hashed brand URLs. The
// favicon falls back to the Athena mark unless the light logo is a PNG.
func identityMessage(fallback string, doc ledger.IdentityDoc) *v1.Identity {
	name := doc.DisplayName
	if name == "" {
		name = fallback
	}
	id := &v1.Identity{DisplayName: name}
	if doc.Light != nil {
		id.LogoLightUrl = brand.LogoURL(doc.Light.Hash, doc.Light.Ext)
	}
	if doc.Dark != nil {
		id.LogoDarkUrl = brand.LogoURL(doc.Dark.Hash, doc.Dark.Ext)
	}
	favHash := brand.MarkHash
	if doc.Light != nil && doc.Light.Ext == "png" {
		favHash = doc.Light.Hash
	}
	id.Favicon_32Url = brand.FaviconURL(favHash, brand.FaviconSmall)
	id.Favicon_180Url = brand.FaviconURL(favHash, brand.FaviconLarge)
	return id
}

// maxDisplayName is the longest Workspace display name in characters.
const maxDisplayName = 80

func (s workspaceService) SetIdentity(ctx context.Context, req *v1.SetIdentityRequest) (_ *v1.SetIdentityResponse, err error) {
	ctx, requestID, end := startKernelSpan(ctx, "set_identity", "")
	defer func() { end(err) }()
	if _, err := requireSystem(ctx, auth.PermissionWorkspace); err != nil {
		return nil, err
	}
	// Both uploads validate before anything is written, so one bad file
	// rejects the whole save.
	light, err := checkLogo(req.GetLogoLight(), req.GetClearLogoLight(), "light")
	if err != nil {
		return nil, err
	}
	dark, err := checkLogo(req.GetLogoDark(), req.GetClearLogoDark(), "dark")
	if err != nil {
		return nil, err
	}
	var name string
	if req.GetDisplayName() != "" {
		name = strings.TrimSpace(req.GetDisplayName())
		if n := utf8.RuneCountInString(name); n < 1 || n > maxDisplayName {
			return nil, connect.NewError(connect.CodeInvalidArgument, "the display name holds 1 to 80 characters")
		}
	}
	if (light != nil || dark != nil || req.GetClearLogoLight() || req.GetClearLogoDark()) && s.files == nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition, "this server has no Workspace directory; start it with athena run")
	}
	_, source := requestFrom(ctx)
	meta := files.Meta{Actor: actorName(ctx), Source: source, RequestID: requestID}
	svc := ledger.Service{DB: s.db, Dialect: dialect}
	doc, err := svc.GetIdentityDoc(ctx)
	if err != nil {
		return nil, internalError("read Workspace identity", err)
	}
	// New files land before the settings save, stale files leave after it,
	// so a crash never points the settings at missing files.
	if light != nil {
		ref, err := s.putLogo(ctx, "light", *light, meta)
		if err != nil {
			return nil, err
		}
		doc.Light = &ref
	} else if req.GetClearLogoLight() {
		doc.Light = nil
	}
	if dark != nil {
		ref, err := s.putLogo(ctx, "dark", *dark, meta)
		if err != nil {
			return nil, err
		}
		doc.Dark = &ref
	} else if req.GetClearLogoDark() {
		doc.Dark = nil
	}
	if name != "" {
		doc.DisplayName = name
	}
	if err := svc.SaveIdentityDoc(ctx, doc, actorName(ctx)); err != nil {
		return nil, internalError("save Workspace identity", err)
	}
	if err := s.pruneLogos(ctx, "light", doc.Light, meta); err != nil {
		return nil, err
	}
	if err := s.pruneLogos(ctx, "dark", doc.Dark, meta); err != nil {
		return nil, err
	}
	return &v1.SetIdentityResponse{Identity: identityMessage(s.name, doc)}, nil
}

// checkLogo validates one side's upload, or returns nil when it is untouched.
func checkLogo(upload *v1.LogoUpload, clear bool, side string) (*brand.Logo, error) {
	if upload == nil {
		return nil, nil
	}
	if clear {
		return nil, connect.NewError(connect.CodeInvalidArgument, "the "+side+" logo cannot be replaced and removed in one save")
	}
	logo, err := brand.ValidateLogo(upload.GetFilename(), upload.GetContent())
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err.Error())
	}
	return &logo, nil
}

// putLogo stores one validated logo and returns its reference.
func (s workspaceService) putLogo(ctx context.Context, side string, logo brand.Logo, meta files.Meta) (ledger.LogoRef, error) {
	rel := "brand/logo-" + side + "." + logo.Ext
	expect := files.Expect{}
	if info, _, err := s.files.Get(ctx, rel, 0); err == nil {
		expect.Hash = info.Hash
	} else if !errors.Is(err, files.ErrNotFound) {
		return ledger.LogoRef{}, fileError("save the logo", err)
	}
	info, err := s.files.Put(ctx, rel, logo.Bytes, expect, meta)
	if err != nil {
		return ledger.LogoRef{}, fileError("save the logo", err)
	}
	return ledger.LogoRef{Ext: logo.Ext, Hash: info.Hash}, nil
}

// pruneLogos deletes the stored files of one side that the document no
// longer names: the other extension after a type change, or every file
// after a removal.
func (s workspaceService) pruneLogos(ctx context.Context, side string, keep *ledger.LogoRef, meta files.Meta) error {
	for _, ext := range []string{"png", "svg"} {
		if keep != nil && keep.Ext == ext {
			continue
		}
		rel := "brand/logo-" + side + "." + ext
		info, _, err := s.files.Get(ctx, rel, 0)
		if errors.Is(err, files.ErrNotFound) {
			continue
		}
		if err != nil {
			return fileError("remove the old logo", err)
		}
		if err := s.files.Delete(ctx, rel, files.Expect{Hash: info.Hash}, meta); err != nil {
			return fileError("remove the old logo", err)
		}
	}
	return nil
}
