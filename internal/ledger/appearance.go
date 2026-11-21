package ledger

import (
	"context"
	"database/sql"
	"errors"

	v1 "athena-erp/gen/athena/v1"
	"athena-erp/internal/appearance"
	"google.golang.org/protobuf/encoding/protojson"
)

// brand.accent is a kernel-owned scalar in the G5 Settings chain. A dot
// distinguishes it from setting types that a Pack author may create.
const appearanceKey = "brand.accent"
const appearancePack = "athena"

func (s *Service) GetAppearance(ctx context.Context) (*v1.Appearance, appearance.Palette, error) {
	var raw []byte
	err := s.DB.QueryRowContext(ctx, `SELECT value_json FROM athena_setting_values WHERE owner_pack=? AND setting_type=? AND scope_chain='' AND status='published' AND effective_from<=UTC_TIMESTAMP(3) ORDER BY effective_from DESC,revision DESC LIMIT 1`, appearancePack, appearanceKey).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return &v1.Appearance{Preset: "ink"}, appearance.Palette{}, nil
	}
	if err != nil {
		return nil, appearance.Palette{}, err
	}
	a := &v1.Appearance{}
	if err := protojson.Unmarshal(raw, a); err != nil {
		return nil, appearance.Palette{}, errors.New("invalid stored appearance")
	}
	palette, err := appearance.Resolve(a)
	if err != nil {
		return nil, appearance.Palette{}, errors.New("invalid stored appearance")
	}
	return a, palette, nil
}

func (s *Service) SetAppearance(ctx context.Context, a *v1.Appearance, actor string) (appearance.Palette, error) {
	palette, err := appearance.Resolve(a)
	if err != nil {
		return appearance.Palette{}, err
	}
	raw, err := protojson.Marshal(a)
	if err != nil {
		return appearance.Palette{}, err
	}
	err = s.withTransaction(ctx, func(tx *sql.Tx) error {
		// This upsert locks the definition so two saves in the same millisecond
		// receive distinct generations. The definition and publication commit together.
		if _, err := tx.ExecContext(ctx, `INSERT INTO athena_settings(owner_pack,setting_type,label,value_kind) VALUES (?,?,'Workspace accent','scalar') ON DUPLICATE KEY UPDATE revision=revision`, appearancePack, appearanceKey); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, `INSERT INTO athena_setting_values(owner_pack,setting_type,scope_chain,effective_from,revision,value_json,status,published_by)
  SELECT ?,?,'',UTC_TIMESTAMP(3),COALESCE(MAX(revision),0)+1,?,'published',? FROM athena_setting_values
  WHERE owner_pack=? AND setting_type=? AND scope_chain='' AND effective_from=UTC_TIMESTAMP(3)`, appearancePack, appearanceKey, raw, actor, appearancePack, appearanceKey)
		return err
	})
	return palette, err
}
