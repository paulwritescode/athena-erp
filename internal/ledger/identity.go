package ledger

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
)

// brand.identity is a kernel-owned scalar in the G5 Settings chain holding
// the Workspace display name and logo references. The logo bytes live as
// Workspace files under brand/; the document only names them.
const identityKey = "brand.identity"
const identityPack = "athena"

// LogoRef names one stored logo: its extension and the SHA-256 of its bytes.
type LogoRef struct {
	Ext  string `json:"ext"`
	Hash string `json:"hash"`
}

// IdentityDoc is the stored brand.identity value. A nil side means no logo
// for that theme.
type IdentityDoc struct {
	DisplayName string   `json:"displayName"`
	Light       *LogoRef `json:"light,omitempty"`
	Dark        *LogoRef `json:"dark,omitempty"`
}

// GetIdentityDoc reads the published Workspace identity. A Workspace that
// never set one gets the zero document.
func (s *Service) GetIdentityDoc(ctx context.Context) (IdentityDoc, error) {
	var raw []byte
	err := s.DB.QueryRowContext(ctx, `SELECT value_json FROM athena_setting_values WHERE owner_pack=? AND setting_type=? AND scope_chain='' AND status='published' AND effective_from<=UTC_TIMESTAMP(3) ORDER BY effective_from DESC,revision DESC LIMIT 1`, identityPack, identityKey).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return IdentityDoc{}, nil
	}
	if err != nil {
		return IdentityDoc{}, err
	}
	var doc IdentityDoc
	if err := json.Unmarshal(raw, &doc); err != nil {
		return IdentityDoc{}, errors.New("invalid stored identity")
	}
	return doc, nil
}

// SaveIdentityDoc publishes one generation of the Workspace identity.
func (s *Service) SaveIdentityDoc(ctx context.Context, doc IdentityDoc, actor string) error {
	raw, err := json.Marshal(doc)
	if err != nil {
		return err
	}
	return s.withTransaction(ctx, func(tx *sql.Tx) error {
		// This upsert locks the definition so two saves in the same millisecond
		// receive distinct generations. The definition and publication commit together.
		if _, err := tx.ExecContext(ctx, `INSERT INTO athena_settings(owner_pack,setting_type,label,value_kind) VALUES (?,?,'Workspace identity','scalar') ON DUPLICATE KEY UPDATE revision=revision`, identityPack, identityKey); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, `INSERT INTO athena_setting_values(owner_pack,setting_type,scope_chain,effective_from,revision,value_json,status,published_by)
  SELECT ?,?,'',UTC_TIMESTAMP(3),COALESCE(MAX(revision),0)+1,?,'published',? FROM athena_setting_values
  WHERE owner_pack=? AND setting_type=? AND scope_chain='' AND effective_from=UTC_TIMESTAMP(3)`, identityPack, identityKey, raw, actor, identityPack, identityKey)
		return err
	})
}
