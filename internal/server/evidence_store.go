package server

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

const evidenceTempDirName = ".tmp"

type evidenceStore struct {
	root string

	recoverOnce sync.Once
	recoverErr  error
}

func newEvidenceStore() *evidenceStore {
	root := strings.TrimSpace(os.Getenv("WAYPOINT_EVIDENCE_DIR"))
	if root == "" {
		root = filepath.Join(os.TempDir(), "waypoint", "evidence")
	}
	return &evidenceStore{root: root}
}

func (s *evidenceStore) ensureReady(ctx context.Context, db *sql.DB) error {
	s.recoverOnce.Do(func() {
		s.recoverErr = s.recover(ctx, db)
	})
	return s.recoverErr
}

func (s *evidenceStore) recover(ctx context.Context, db *sql.DB) error {
	if err := os.MkdirAll(s.root, 0o750); err != nil {
		return fmt.Errorf("prepare evidence root: %w", err)
	}
	tmpDir := filepath.Join(s.root, evidenceTempDirName)
	if err := os.RemoveAll(tmpDir); err != nil {
		return fmt.Errorf("clear evidence temp dir: %w", err)
	}
	if err := os.MkdirAll(tmpDir, 0o700); err != nil {
		return fmt.Errorf("recreate evidence temp dir: %w", err)
	}
	if db == nil {
		return nil
	}

	referenced := map[string]struct{}{}
	rows, err := db.QueryContext(ctx, `SELECT storage_key FROM evidence WHERE storage_key IS NOT NULL AND storage_key <> ''`)
	if err != nil {
		return fmt.Errorf("load evidence references: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			return fmt.Errorf("scan evidence reference: %w", err)
		}
		referenced[key] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate evidence references: %w", err)
	}

	if err := filepath.WalkDir(s.root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == s.root || d.IsDir() {
			return nil
		}
		if strings.HasPrefix(filepath.Base(path), ".tmp-") {
			return os.Remove(path)
		}
		rel, err := filepath.Rel(s.root, path)
		if err != nil {
			return err
		}
		storageKey := filepath.ToSlash(rel)
		if _, ok := referenced[storageKey]; ok {
			return nil
		}
		return os.Remove(path)
	}); err != nil {
		return fmt.Errorf("reconcile evidence storage: %w", err)
	}
	return nil
}

func (s *evidenceStore) ingest(ctx context.Context, pointer, kind string, declared captureEvidenceDescriptor, part *multipart.Part) (captureEvidenceBytes, error) {
	if err := os.MkdirAll(filepath.Join(s.root, evidenceTempDirName), 0o700); err != nil {
		return captureEvidenceBytes{}, fmt.Errorf("prepare evidence temp dir: %w", err)
	}

	tmp, err := os.CreateTemp(filepath.Join(s.root, evidenceTempDirName), ".tmp-*")
	if err != nil {
		return captureEvidenceBytes{}, fmt.Errorf("create evidence temp file: %w", err)
	}
	tmpPath := tmp.Name()
	defer func() {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
	}()

	hasher := sha256.New()
	written, err := copyEvidenceStream(ctx, tmp, hasher, part, maxCaptureEvidenceBytes, pointer)
	if err != nil {
		return captureEvidenceBytes{}, err
	}
	if err := tmp.Sync(); err != nil {
		return captureEvidenceBytes{}, fmt.Errorf("sync evidence temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return captureEvidenceBytes{}, fmt.Errorf("close evidence temp file: %w", err)
	}

	actual := captureEvidenceBytes{digest: hex.EncodeToString(hasher.Sum(nil)), byteLength: written}
	if actual.digest != declared.SHA256 || actual.byteLength != declared.ByteLength {
		return captureEvidenceBytes{}, captureRequestProblem{problem: captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusUnprocessableEntity), Status: http.StatusUnprocessableEntity, Code: "evidence_integrity_mismatch", Retryable: false, Detail: "declared evidence did not match the streamed bytes.", FieldErrors: []fieldError{{Pointer: pointer + "/sha256", Code: "digest_mismatch", Message: "declared digest did not match the streamed bytes."}}}}
	}

	if err := s.promoteEvidenceBlob(tmpPath, actual.digest, kind); err != nil {
		return captureEvidenceBytes{}, err
	}
	return actual, nil
}

// writeBlob persists an in-memory evidence blob into the content-addressed
// store and returns its digest and byte length. It is used by the demo seeder
// to lay down the stdout/stderr blobs that its synthetic actions reference, so
// evidence downloads and report bundles resolve exactly as they would for a
// real capture. The digest is computed here, so callers cannot desynchronize
// the stored bytes from the evidence row they insert.
func (s *evidenceStore) writeBlob(kind string, data []byte) (sha string, byteLength int64, err error) {
	if err := os.MkdirAll(filepath.Join(s.root, evidenceTempDirName), 0o700); err != nil {
		return "", 0, fmt.Errorf("prepare evidence temp dir: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Join(s.root, evidenceTempDirName), ".tmp-*")
	if err != nil {
		return "", 0, fmt.Errorf("create evidence temp file: %w", err)
	}
	tmpPath := tmp.Name()
	defer func() {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
	}()

	hasher := sha256.New()
	if _, err := tmp.Write(data); err != nil {
		return "", 0, fmt.Errorf("write evidence temp file: %w", err)
	}
	if _, err := hasher.Write(data); err != nil {
		return "", 0, err
	}
	if err := tmp.Sync(); err != nil {
		return "", 0, fmt.Errorf("sync evidence temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return "", 0, fmt.Errorf("close evidence temp file: %w", err)
	}

	sha = hex.EncodeToString(hasher.Sum(nil))
	if err := s.promoteEvidenceBlob(tmpPath, sha, kind); err != nil {
		return "", 0, err
	}
	return sha, int64(len(data)), nil
}

func (s *evidenceStore) promoteEvidenceBlob(tempPath, sha, kind string) error {
	finalDir := filepath.Join(s.root, "captures", sha)
	if err := os.MkdirAll(finalDir, 0o750); err != nil {
		return fmt.Errorf("prepare evidence destination: %w", err)
	}
	finalPath := filepath.Join(finalDir, kind)
	if err := promoteBlob(tempPath, finalPath, sha); err != nil {
		return err
	}
	return syncEvidenceDir(finalDir)
}

func promoteBlob(tempPath, finalPath, sha string) error {
	if err := os.Link(tempPath, finalPath); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrExist) {
		if verifyErr := verifyEvidenceBlob(finalPath, sha); verifyErr == nil {
			return nil
		}
		return fmt.Errorf("promote evidence blob: %w", err)
	}
	if err := verifyEvidenceBlob(finalPath, sha); err != nil {
		return err
	}
	return nil
}

func verifyEvidenceBlob(path, sha string) error {
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open evidence blob: %w", err)
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return fmt.Errorf("hash evidence blob: %w", err)
	}
	if hex.EncodeToString(h.Sum(nil)) != sha {
		return fmt.Errorf("evidence blob hash mismatch")
	}
	return nil
}

func syncEvidenceDir(dir string) error {
	d, err := os.Open(dir)
	if err != nil {
		return fmt.Errorf("open evidence dir: %w", err)
	}
	defer d.Close()
	if err := d.Sync(); err != nil {
		return fmt.Errorf("sync evidence dir: %w", err)
	}
	return nil
}

func copyEvidenceStream(ctx context.Context, dst io.Writer, hasher io.Writer, src io.Reader, limit int64, pointer string) (int64, error) {
	buf := make([]byte, 32*1024)
	var written int64
	for {
		if ctx != nil {
			select {
			case <-ctx.Done():
				return written, ctx.Err()
			default:
			}
		}
		n, err := src.Read(buf)
		if n > 0 {
			if written+int64(n) > limit {
				return written, captureRequestProblem{problem: captureProblem{Type: "about:blank", Title: http.StatusText(http.StatusBadRequest), Status: http.StatusBadRequest, Code: "invalid_request", Retryable: false, FieldErrors: []fieldError{{Pointer: pointer, Code: "invalid_range", Message: fmt.Sprintf("%s evidence is too large; maximum allowed is %d bytes.", strings.TrimPrefix(pointer, "/"), limit)}}}}
			}
			if _, werr := dst.Write(buf[:n]); werr != nil {
				return written, werr
			}
			if _, werr := hasher.Write(buf[:n]); werr != nil {
				return written, werr
			}
			written += int64(n)
		}
		if errors.Is(err, io.EOF) {
			return written, nil
		}
		if err != nil {
			return written, err
		}
	}
}
