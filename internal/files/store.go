package files

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"sort"
	"strings"
	"time"

	"athena-erp/internal/audit"
	"athena-erp/internal/store"

	"github.com/google/uuid"
)

// AuditType is the record_type_name of file changes in the Audit Log. The
// colon cannot appear in a Record Type key, so it never collides with one.
const AuditType = "athena:file"

// externalActor is the actor of a version made from content that changed
// outside Athena, whose author is unknown.
const externalActor = "outside Athena"

// Meta says who makes a change, for the file version and the Audit Log.
type Meta struct {
	Actor     string
	Source    string
	RequestID string
}

// Expect is the precondition of a change. A non-empty Hash must equal the
// hash of the file on disk. An empty Hash means the file must not exist.
type Expect struct {
	Hash string
}

// Info describes one file on disk.
type Info struct {
	Path       string
	Size       int64
	Hash       string
	Revision   int64
	ModifiedAt time.Time
	ModifiedBy string
	// Recorded is false when the content on disk is not the last recorded
	// revision, for example after an edit outside Athena.
	Recorded bool
}

// Version is one recorded revision of a file.
type Version struct {
	Revision   int64
	Action     string
	Path       string
	Hash       string
	Size       int64
	Actor      string
	Source     string
	RequestID  string
	CreatedAt  time.Time
	HasContent bool
}

// Store manages the files under one Workspace directory. The disk holds the
// content; the database holds the version history and the Audit Log entries.
type Store struct {
	DB      *sql.DB
	Dialect store.Dialect
	root    *os.Root

	// fault lets a test fail a write between its steps. It is nil in
	// production.
	fault func(stage string) error
}

// Open confines a Store to the Workspace directory dir.
func Open(dir string, db *sql.DB, dialect store.Dialect) (*Store, error) {
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, fmt.Errorf("open the Workspace directory: %w", err)
	}
	return &Store{DB: db, Dialect: dialect, root: root}, nil
}

// Close releases the directory handle.
func (s *Store) Close() error { return s.root.Close() }

// Hash is the SHA-256 of content as lowercase hex.
func Hash(content []byte) string {
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:])
}

// PathID is a short stable identifier of a path for spans and logs, which
// must not carry file names.
func PathID(p string) string { return Hash([]byte(p))[:12] }

// diskFile is a file read from disk.
type diskFile struct {
	content []byte
	hash    string
	modTime time.Time
}

// stat returns the file at rel, or fs.ErrNotExist. It refuses a symbolic link
// at any level and anything that is not a regular file. rel must already be
// a clean path.
func (s *Store) stat(rel string) (fs.FileInfo, error) {
	parts := strings.Split(rel, "/")
	for i := range parts {
		prefix := strings.Join(parts[:i+1], "/")
		info, err := s.root.Lstat(prefix)
		if errors.Is(err, fs.ErrNotExist) {
			return nil, fs.ErrNotExist
		}
		if err != nil {
			return nil, fmt.Errorf("check %s: %w", prefix, err)
		}
		switch {
		case info.Mode()&fs.ModeSymlink != 0:
			return nil, invalidPath("%q is or passes through a symbolic link", rel)
		case i < len(parts)-1 && !info.IsDir():
			return nil, invalidPath("%q passes through a file", rel)
		case i == len(parts)-1 && !info.Mode().IsRegular():
			return nil, invalidPath("%q is not a regular file", rel)
		case i == len(parts)-1:
			return info, nil
		}
	}
	return nil, fs.ErrNotExist
}

// read loads a file for rel, or fs.ErrNotExist. A file over the size limit
// is refused, as is one that is not text unless it is a PNG logo under
// brand/.
func (s *Store) read(rel string) (diskFile, error) {
	limit := int64(MaxFileBytes)
	if IsImagePath(rel) {
		limit = MaxImageBytes
	}
	info, err := s.stat(rel)
	if err != nil {
		return diskFile{}, err
	}
	if info.Size() > limit {
		return diskFile{}, fmt.Errorf("%w: the limit is %d bytes", ErrTooLarge, limit)
	}
	f, err := s.root.Open(rel)
	if err != nil {
		return diskFile{}, err
	}
	defer f.Close()
	content, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil {
		return diskFile{}, err
	}
	if !AllowBinary(rel) && !IsImagePath(rel) {
		if err := checkText(content); err != nil {
			return diskFile{}, err
		}
	}
	return diskFile{content: content, hash: Hash(content), modTime: info.ModTime()}, nil
}

// writeAtomic puts content at rel through a temporary file in the same folder
// and a rename, so a reader sees the old file or the new one, never a mix.
func (s *Store) writeAtomic(rel string, content []byte) (err error) {
	dir := path.Dir(rel)
	if err := s.root.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	id, err := uuid.NewV7()
	if err != nil {
		return err
	}
	// A leading dot keeps the temporary file out of listings and out of
	// every path a caller may name.
	tmp := path.Join(dir, ".athena-tmp-"+id.String())
	f, err := s.root.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = s.root.Remove(tmp)
		}
	}()
	if _, err = f.Write(content); err != nil {
		f.Close()
		return err
	}
	if err = f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	if err = s.failAt("before_rename"); err != nil {
		return err
	}
	return s.root.Rename(tmp, rel)
}

func (s *Store) failAt(stage string) error {
	if s.fault == nil {
		return nil
	}
	return s.fault(stage)
}

// fileRow is the athena_files row of one file.
type fileRow struct {
	id       string
	revision int64
	hash     string
	size     int64
	by       string
	at       time.Time
}

// lockRow returns the row of the file at rel and holds its lock until tx ends,
// creating a row at revision 0 when the file has never been recorded. A
// concurrent change to the same path surfaces as ErrStale.
func (s *Store) lockRow(ctx context.Context, tx *sql.Tx, rel string) (fileRow, error) {
	var r fileRow
	err := tx.QueryRowContext(ctx, "SELECT id, revision, hash, size, modified_by, modified_at FROM athena_files WHERE path = ? "+s.Dialect.ForUpdate(), rel).
		Scan(&r.id, &r.revision, &r.hash, &r.size, &r.by, &r.at)
	if err == nil {
		return r, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return fileRow{}, s.lockError(err)
	}
	id, err := uuid.NewV7()
	if err != nil {
		return fileRow{}, err
	}
	now := time.Now().UTC()
	if _, err := tx.ExecContext(ctx, "INSERT INTO athena_files (id, path, revision, hash, size, modified_by, created_at, modified_at) VALUES (?,?,0,'',0,'',?,?)", id.String(), rel, now, now); err != nil {
		return fileRow{}, s.lockError(err)
	}
	return fileRow{id: id.String(), at: now}, nil
}

// lockError maps the errors two writers racing on one path produce to ErrStale.
func (s *Store) lockError(err error) error {
	switch s.Dialect.Classify(err) {
	case store.ErrDuplicateKey, store.ErrDeadlock, store.ErrLockTimeout, store.ErrSerializationFailure:
		return fmt.Errorf("%w: another change to this file is in progress; read it again and retry", ErrStale)
	}
	return err
}

// writeVersion stores one revision. content is nil for a delete.
func writeVersion(ctx context.Context, tx *sql.Tx, id string, revision int64, rel, action, hash string, size int64, content []byte, actor string, meta Meta) error {
	var body any
	if content != nil {
		body = content
	}
	var requestID any
	if meta.RequestID != "" {
		requestID = meta.RequestID
	}
	_, err := tx.ExecContext(ctx, "INSERT INTO athena_file_versions (file_id, revision, path, action, hash, size, content, actor, source, request_id, created_at) VALUES (?,?,?,?,?,?,?,?,?,?,?)",
		id, revision, rel, action, hash, size, body, actor, meta.Source, requestID, time.Now().UTC())
	return err
}

// keepExternal records content found on disk that differs from the last
// recorded revision, so an edit made outside Athena is never lost to the next
// change. It advances r.
func keepExternal(ctx context.Context, tx *sql.Tx, r *fileRow, rel string, disk diskFile, meta Meta) error {
	if r.hash == disk.hash {
		return nil
	}
	r.revision++
	if err := writeVersion(ctx, tx, r.id, r.revision, rel, "external", disk.hash, int64(len(disk.content)), disk.content, externalActor, meta); err != nil {
		return err
	}
	r.hash, r.size, r.by = disk.hash, int64(len(disk.content)), externalActor
	return nil
}

// check applies an Expect to the file on disk (nil when absent).
func check(rel string, disk *diskFile, expect Expect) error {
	switch {
	case expect.Hash == "" && disk != nil:
		return fmt.Errorf("%w: %s already exists", ErrExists, rel)
	case expect.Hash == "":
		return nil
	case disk == nil:
		return fmt.Errorf("%w: %s", ErrNotFound, rel)
	case disk.hash != expect.Hash:
		return fmt.Errorf("%w: %s no longer matches the version you read; read it again", ErrStale, rel)
	}
	return nil
}

func (s *Store) load(rel string) (*diskFile, error) {
	disk, err := s.read(rel)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &disk, nil
}

// audit appends the file change to the Audit Log. The entry holds the path,
// the hashes and the size, never the content.
func (s *Store) audit(ctx context.Context, tx *sql.Tx, id string, revision int64, action string, meta Meta, changes ...audit.Change) error {
	_, err := audit.Append(ctx, tx, s.Dialect, audit.Entry{
		RecordType: AuditType, RecordID: id, Revision: revision, Action: action,
		Changes: changes, Actor: meta.Actor, Source: meta.Source, RequestID: meta.RequestID,
	})
	return err
}

func change(field, oldValue, newValue string) audit.Change {
	c := audit.Change{Field: field}
	if oldValue != "" {
		c.Old = &oldValue
	}
	if newValue != "" {
		c.New = &newValue
	}
	return c
}

// Put creates or replaces the file at p and returns its Info after the
// change. Content identical to the file on disk changes nothing.
func (s *Store) Put(ctx context.Context, p string, content []byte, expect Expect, meta Meta) (Info, error) {
	rel, err := CleanPath(p)
	if err != nil {
		return Info{}, err
	}
	if err := Validate(rel, content); err != nil {
		return Info{}, err
	}
	return s.putContent(ctx, rel, content, expect, meta)
}

// ImageInfo describes a stored image: its file with the pixel dimensions of
// what was stored.
type ImageInfo struct {
	Info
	Width  int
	Height int
}

// PutImage validates uploaded bytes as an image, strips their metadata and
// stores the result like Put: versioned, audited and concurrency-checked.
func (s *Store) PutImage(ctx context.Context, p string, content []byte, expect Expect, meta Meta) (ImageInfo, error) {
	rel, err := CleanPath(p)
	if err != nil {
		return ImageInfo{}, err
	}
	if _, err := ValidateImage(rel, content); err != nil {
		return ImageInfo{}, err
	}
	cleaned, err := CleanImage(content)
	if err != nil {
		return ImageInfo{}, err
	}
	info, err := s.putContent(ctx, rel, cleaned.Content, expect, meta)
	if err != nil {
		return ImageInfo{}, err
	}
	return ImageInfo{Info: info, Width: cleaned.Width, Height: cleaned.Height}, nil
}

// putContent records validated content under an already-cleaned path.
func (s *Store) putContent(ctx context.Context, rel string, content []byte, expect Expect, meta Meta) (Info, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return Info{}, err
	}
	defer func() { _ = tx.Rollback() }()
	row, err := s.lockRow(ctx, tx, rel)
	if err != nil {
		return Info{}, err
	}
	disk, err := s.load(rel)
	if err != nil {
		return Info{}, err
	}
	if err := check(rel, disk, expect); err != nil {
		return Info{}, err
	}
	hash, size := Hash(content), int64(len(content))
	action, auditAction, oldHash := "created", "file_created", ""
	if disk == nil {
		if err := s.checkRoom(); err != nil {
			return Info{}, err
		}
	} else {
		if disk.hash == hash && row.hash == hash {
			return s.info(rel, row, *disk), nil
		}
		if err := keepExternal(ctx, tx, &row, rel, *disk, meta); err != nil {
			return Info{}, err
		}
		action, auditAction, oldHash = "updated", "file_updated", disk.hash
	}
	row.revision++
	if err := writeVersion(ctx, tx, row.id, row.revision, rel, action, hash, size, content, meta.Actor, meta); err != nil {
		return Info{}, err
	}
	now := time.Now().UTC()
	if _, err := tx.ExecContext(ctx, "UPDATE athena_files SET revision = ?, hash = ?, size = ?, modified_by = ?, modified_at = ? WHERE id = ?", row.revision, hash, size, meta.Actor, now, row.id); err != nil {
		return Info{}, err
	}
	if err := s.audit(ctx, tx, row.id, row.revision, auditAction, meta, change("path", "", rel), change("hash", oldHash, hash), change("size", "", fmt.Sprint(size))); err != nil {
		return Info{}, err
	}
	if err := s.writeAtomic(rel, content); err != nil {
		return Info{}, fmt.Errorf("write %s: %w", rel, err)
	}
	if err := tx.Commit(); err != nil {
		s.restore(rel, disk)
		return Info{}, err
	}
	return Info{Path: rel, Size: size, Hash: hash, Revision: row.revision, ModifiedAt: now, ModifiedBy: meta.Actor, Recorded: true}, nil
}

// restore puts the previous content back after a change was written to disk
// but could not be recorded. It is best effort: if it fails, the next change
// keeps whatever is on disk as an external version.
func (s *Store) restore(rel string, previous *diskFile) {
	if previous == nil {
		_ = s.root.Remove(rel)
		return
	}
	_ = s.writeAtomic(rel, previous.content)
}

// Move renames the file at from to to. The file keeps its history.
func (s *Store) Move(ctx context.Context, from, to string, expect Expect, meta Meta) (Info, error) {
	src, err := CleanPath(from)
	if err != nil {
		return Info{}, err
	}
	dst, err := CleanPath(to)
	if err != nil {
		return Info{}, err
	}
	if src == dst {
		return Info{}, invalidPath("the new path is the same as the old one")
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return Info{}, err
	}
	defer func() { _ = tx.Rollback() }()
	row, err := s.lockRow(ctx, tx, src)
	if err != nil {
		return Info{}, err
	}
	disk, err := s.load(src)
	if err != nil {
		return Info{}, err
	}
	if disk == nil {
		return Info{}, fmt.Errorf("%w: %s", ErrNotFound, src)
	}
	if err := check(src, disk, expect); err != nil {
		return Info{}, err
	}
	// The content must be valid where it lands: a file moved into
	// record_types/ has to be a Record Type file, and only images land in
	// images/.
	if IsImagePath(dst) {
		if _, err := ValidateImage(dst, disk.content); err != nil {
			return Info{}, err
		}
	} else if err := Validate(dst, disk.content); err != nil {
		return Info{}, err
	}
	if target, err := s.load(dst); err != nil {
		return Info{}, err
	} else if target != nil {
		return Info{}, fmt.Errorf("%w: %s", ErrExists, dst)
	}
	if err := s.releasePath(ctx, tx, dst, meta); err != nil {
		return Info{}, err
	}
	if err := keepExternal(ctx, tx, &row, src, *disk, meta); err != nil {
		return Info{}, err
	}
	row.revision++
	now := time.Now().UTC()
	size := int64(len(disk.content))
	if err := writeVersion(ctx, tx, row.id, row.revision, dst, "moved", disk.hash, size, disk.content, meta.Actor, meta); err != nil {
		return Info{}, err
	}
	if _, err := tx.ExecContext(ctx, "UPDATE athena_files SET path = ?, revision = ?, hash = ?, size = ?, modified_by = ?, modified_at = ? WHERE id = ?", dst, row.revision, disk.hash, size, meta.Actor, now, row.id); err != nil {
		return Info{}, s.lockError(err)
	}
	if err := s.audit(ctx, tx, row.id, row.revision, "file_moved", meta, change("path", src, dst), change("hash", "", disk.hash)); err != nil {
		return Info{}, err
	}
	if err := s.root.MkdirAll(path.Dir(dst), 0o755); err != nil {
		return Info{}, err
	}
	if err := s.root.Rename(src, dst); err != nil {
		return Info{}, fmt.Errorf("move %s: %w", src, err)
	}
	if err := tx.Commit(); err != nil {
		_ = s.root.Rename(dst, src)
		return Info{}, err
	}
	return Info{Path: dst, Size: size, Hash: disk.hash, Revision: row.revision, ModifiedAt: now, ModifiedBy: meta.Actor, Recorded: true}, nil
}

// releasePath frees rel in athena_files when a row still holds the path of a
// file that is no longer on disk (it was removed outside Athena). The row
// keeps its history and loses the path.
func (s *Store) releasePath(ctx context.Context, tx *sql.Tx, rel string, meta Meta) error {
	var r fileRow
	err := tx.QueryRowContext(ctx, "SELECT id, revision, hash, size FROM athena_files WHERE path = ? "+s.Dialect.ForUpdate(), rel).Scan(&r.id, &r.revision, &r.hash, &r.size)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return s.lockError(err)
	}
	r.revision++
	if err := writeVersion(ctx, tx, r.id, r.revision, rel, "deleted", r.hash, r.size, nil, externalActor, meta); err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, "UPDATE athena_files SET path = NULL, revision = ? WHERE id = ?", r.revision, r.id)
	return err
}

// Delete removes the file at p from disk. Its versions stay.
func (s *Store) Delete(ctx context.Context, p string, expect Expect, meta Meta) error {
	rel, err := CleanPath(p)
	if err != nil {
		return err
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	row, err := s.lockRow(ctx, tx, rel)
	if err != nil {
		return err
	}
	disk, err := s.load(rel)
	if err != nil {
		return err
	}
	if disk == nil {
		return fmt.Errorf("%w: %s", ErrNotFound, rel)
	}
	if err := check(rel, disk, expect); err != nil {
		return err
	}
	if err := keepExternal(ctx, tx, &row, rel, *disk, meta); err != nil {
		return err
	}
	row.revision++
	if err := writeVersion(ctx, tx, row.id, row.revision, rel, "deleted", disk.hash, int64(len(disk.content)), nil, meta.Actor, meta); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "UPDATE athena_files SET path = NULL, revision = ?, modified_by = ?, modified_at = ? WHERE id = ?", row.revision, meta.Actor, time.Now().UTC(), row.id); err != nil {
		return err
	}
	if err := s.audit(ctx, tx, row.id, row.revision, "file_deleted", meta, change("path", rel, ""), change("hash", disk.hash, "")); err != nil {
		return err
	}
	if err := s.root.Remove(rel); err != nil {
		return fmt.Errorf("delete %s: %w", rel, err)
	}
	if err := tx.Commit(); err != nil {
		s.restore(rel, disk)
		return err
	}
	return nil
}

// Get reads the file at p. With revision 0 it is the file on disk; with
// another number it is that recorded revision, which survives a delete.
func (s *Store) Get(ctx context.Context, p string, revision int64) (Info, []byte, error) {
	rel, err := CleanPath(p)
	if err != nil {
		return Info{}, nil, err
	}
	if revision > 0 {
		return s.getVersion(ctx, rel, revision)
	}
	disk, err := s.read(rel)
	if errors.Is(err, fs.ErrNotExist) {
		return Info{}, nil, fmt.Errorf("%w: %s", ErrNotFound, rel)
	}
	if err != nil {
		return Info{}, nil, err
	}
	rows, err := s.rows(ctx)
	if err != nil {
		return Info{}, nil, err
	}
	return s.info(rel, rows[rel], disk), disk.content, nil
}

func (s *Store) getVersion(ctx context.Context, rel string, revision int64) (Info, []byte, error) {
	id, err := s.lineage(ctx, rel)
	if err != nil {
		return Info{}, nil, err
	}
	var v Version
	var content []byte
	var created time.Time
	err = s.DB.QueryRowContext(ctx, "SELECT path, hash, size, actor, content, created_at FROM athena_file_versions WHERE file_id = ? AND revision = ?", id, revision).
		Scan(&v.Path, &v.Hash, &v.Size, &v.Actor, &content, &created)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && content == nil) {
		return Info{}, nil, fmt.Errorf("%w: %s has no stored content at revision %d", ErrNotFound, rel, revision)
	}
	if err != nil {
		return Info{}, nil, err
	}
	return Info{Path: v.Path, Size: v.Size, Hash: v.Hash, Revision: revision, ModifiedAt: created, ModifiedBy: v.Actor, Recorded: true}, content, nil
}

// lineage finds the file whose history last used rel, so history follows a
// file across renames and survives a delete.
func (s *Store) lineage(ctx context.Context, rel string) (string, error) {
	var id string
	err := s.DB.QueryRowContext(ctx, "SELECT file_id FROM athena_file_versions WHERE path = ? ORDER BY seq DESC LIMIT 1", rel).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return "", fmt.Errorf("%w: no history for %s", ErrNotFound, rel)
	}
	return id, err
}

// Versions lists the recorded revisions of the file last at p, newest first.
func (s *Store) Versions(ctx context.Context, p string) ([]Version, error) {
	rel, err := CleanPath(p)
	if err != nil {
		return nil, err
	}
	id, err := s.lineage(ctx, rel)
	if err != nil {
		return nil, err
	}
	rows, err := s.DB.QueryContext(ctx, "SELECT revision, action, path, hash, size, actor, source, request_id, created_at, content IS NOT NULL FROM athena_file_versions WHERE file_id = ? ORDER BY revision DESC", id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Version
	for rows.Next() {
		var v Version
		var requestID sql.NullString
		if err := rows.Scan(&v.Revision, &v.Action, &v.Path, &v.Hash, &v.Size, &v.Actor, &v.Source, &requestID, &v.CreatedAt, &v.HasContent); err != nil {
			return nil, err
		}
		v.RequestID = requestID.String
		out = append(out, v)
	}
	return out, rows.Err()
}

// rows loads the recorded state of every live file, by path.
func (s *Store) rows(ctx context.Context) (map[string]fileRow, error) {
	q, err := s.DB.QueryContext(ctx, "SELECT path, id, revision, hash, size, modified_by, modified_at FROM athena_files WHERE path IS NOT NULL")
	if err != nil {
		return nil, err
	}
	defer q.Close()
	out := map[string]fileRow{}
	for q.Next() {
		var p string
		var r fileRow
		if err := q.Scan(&p, &r.id, &r.revision, &r.hash, &r.size, &r.by, &r.at); err != nil {
			return nil, err
		}
		out[p] = r
	}
	return out, q.Err()
}

func (s *Store) info(rel string, row fileRow, disk diskFile) Info {
	info := Info{Path: rel, Size: int64(len(disk.content)), Hash: disk.hash, ModifiedAt: disk.modTime, Revision: row.revision}
	if row.revision > 0 && row.hash == disk.hash {
		info.Recorded, info.ModifiedBy = true, row.by
	}
	return info
}

// List returns every managed file, sorted by path. Anything Athena would
// refuse to open is left out: names that fail CleanPath, symbolic links,
// files over the size limit and files that are not text (except a PNG logo
// under brand/).
func (s *Store) List(ctx context.Context) ([]Info, error) {
	rows, err := s.rows(ctx)
	if err != nil {
		return nil, err
	}
	var out []Info
	for _, folder := range Folders {
		if err := s.walk(folder, func(rel string) error {
			if _, err := CleanPath(rel); err != nil {
				return nil
			}
			disk, err := s.read(rel)
			if err != nil {
				return nil
			}
			if len(out) >= MaxFiles {
				return fmt.Errorf("%w: the limit is %d", ErrLimit, MaxFiles)
			}
			out = append(out, s.info(rel, rows[rel], disk))
			return nil
		}); err != nil {
			return nil, err
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out, nil
}

// walk calls visit for every regular file under dir. It never follows a
// symbolic link.
func (s *Store) walk(dir string, visit func(rel string) error) error {
	info, err := s.root.Lstat(dir)
	if err != nil || !info.IsDir() {
		return nil
	}
	f, err := s.root.Open(dir)
	if err != nil {
		return nil
	}
	entries, err := f.ReadDir(-1)
	f.Close()
	if err != nil {
		return nil
	}
	for _, entry := range entries {
		rel := dir + "/" + entry.Name()
		switch {
		case entry.Type()&fs.ModeSymlink != 0:
		case entry.IsDir():
			if err := s.walk(rel, visit); err != nil {
				return err
			}
		case entry.Type().IsRegular():
			if err := visit(rel); err != nil {
				return err
			}
		}
	}
	return nil
}

// checkRoom refuses a new file when the Workspace already holds MaxFiles.
func (s *Store) checkRoom() error {
	count := 0
	for _, folder := range Folders {
		_ = s.walk(folder, func(string) error { count++; return nil })
	}
	if count >= MaxFiles {
		return fmt.Errorf("%w: the limit is %d", ErrLimit, MaxFiles)
	}
	return nil
}
