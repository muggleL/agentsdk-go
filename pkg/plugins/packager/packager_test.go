package packager

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cexll/agentsdk-go/pkg/plugins"
)

func TestPackagerExportImport(t *testing.T) {
	root := t.TempDir()
	pluginDir := filepath.Join(root, "demo")
	writePlugin(t, pluginDir)

	p, err := NewPackager(root, nil)
	if err != nil {
		t.Fatalf("packager: %v", err)
	}
	var buf bytes.Buffer
	manifest, err := p.Export("demo", &buf)
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	if manifest.Name != "demo" {
		t.Fatalf("unexpected manifest name %s", manifest.Name)
	}

	installRoot := filepath.Join(t.TempDir(), "plugins")
	installer, err := NewPackager(installRoot, nil)
	if err != nil {
		t.Fatalf("installer: %v", err)
	}
	imported, err := installer.Import(bytes.NewReader(buf.Bytes()), "demo")
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if imported.Name != manifest.Name || imported.Digest != manifest.Digest {
		t.Fatalf("imported manifest mismatch")
	}
	if _, err := os.Stat(filepath.Join(installRoot, "demo", "README.md")); err != nil {
		t.Fatalf("expected file copied: %v", err)
	}
}

func TestPackagerImportGuards(t *testing.T) {
	root := t.TempDir()
	p, err := NewPackager(root, nil)
	if err != nil {
		t.Fatalf("packager: %v", err)
	}

	// path traversal
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	if err := tw.WriteHeader(&tar.Header{Name: "../evil", Mode: 0600, Size: int64(len("x"))}); err != nil {
		t.Fatalf("write header: %v", err)
	}
	if _, err := tw.Write([]byte("x")); err != nil {
		t.Fatalf("write body: %v", err)
	}
	tw.Close()
	gz.Close()
	if _, err := p.Import(bytes.NewReader(buf.Bytes()), "evil"); !errors.Is(err, ErrUnsafeArchive) {
		t.Fatalf("expected unsafe archive error, got %v", err)
	}

	// missing manifest
	buf.Reset()
	gz = gzip.NewWriter(&buf)
	tw = tar.NewWriter(gz)
	if err := tw.WriteHeader(&tar.Header{Name: "file.txt", Mode: 0600, Size: int64(len("x"))}); err != nil {
		t.Fatalf("write header: %v", err)
	}
	if _, err := tw.Write([]byte("x")); err != nil {
		t.Fatalf("write body: %v", err)
	}
	tw.Close()
	gz.Close()
	if _, err := p.Import(bytes.NewReader(buf.Bytes()), "missing"); err == nil {
		t.Fatalf("expected manifest error")
	}
}

func TestPackagerValidationHelpers(t *testing.T) {
	if _, err := NewPackager("", nil); err == nil {
		t.Fatalf("expected error for empty root")
	}
	p, err := NewPackager(t.TempDir(), nil)
	if err != nil {
		t.Fatalf("packager: %v", err)
	}
	if p.Root() == "" {
		t.Fatalf("expected non-empty root")
	}
	var nilPackager *Packager
	if _, err := nilPackager.Export("demo", io.Discard); err == nil {
		t.Fatalf("nil packager export should error")
	}
	if nilPackager.Root() != "" {
		t.Fatalf("expected empty root for nil packager")
	}
}

func TestPackagerManifestMismatch(t *testing.T) {
	root := t.TempDir()
	pluginDir := filepath.Join(root, "demo")
	requireNoError(t, os.MkdirAll(filepath.Join(pluginDir, ".claude-plugin"), 0o755))
	bad := plugins.Manifest{Name: "demo", Version: "1.0.0", Digest: "deadbeef"}
	data, err := json.Marshal(bad)
	if err != nil {
		t.Fatalf("marshal manifest: %v", err)
	}
	requireNoError(t, os.WriteFile(filepath.Join(pluginDir, ".claude-plugin", "plugin.json"), data, 0o600))

	p, err := NewPackager(root, nil)
	if err != nil {
		t.Fatalf("packager: %v", err)
	}
	if _, err := p.Export("demo", io.Discard); err == nil {
		t.Fatalf("expected digest mismatch error")
	}
}

func TestPackagerImportDestinationExists(t *testing.T) {
	root := t.TempDir()
	pluginDir := filepath.Join(root, "demo")
	writePlugin(t, pluginDir)
	p, err := NewPackager(root, nil)
	if err != nil {
		t.Fatalf("packager: %v", err)
	}
	var buf bytes.Buffer
	if _, err := p.Export("demo", &buf); err != nil {
		t.Fatalf("export: %v", err)
	}
	dest := filepath.Join(root, "installed")
	requireNoError(t, os.MkdirAll(filepath.Join(dest, "placeholder"), 0o755))
	requireNoError(t, os.WriteFile(filepath.Join(dest, "placeholder", "file.txt"), []byte("x"), 0o600))
	installer, err := NewPackager(dest, nil)
	if err != nil {
		t.Fatalf("installer: %v", err)
	}
	if _, err := installer.Import(bytes.NewReader(buf.Bytes()), "placeholder"); !errors.Is(err, ErrDestinationExists) {
		t.Fatalf("expected destination exists error, got %v", err)
	}
}

func TestPackageDirErrorCases(t *testing.T) {
	root := t.TempDir()
	p, err := NewPackager(root, nil)
	requireNoError(t, err)

	t.Run("source missing", func(t *testing.T) {
		_, err := p.PackageDir(filepath.Join(root, "ghost"), io.Discard)
		if err == nil || !errors.Is(err, plugins.ErrManifestNotFound) {
			t.Fatalf("expected manifest not found, got %v", err)
		}
	})

	t.Run("outside root", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "demo")
		requireNoError(t, os.MkdirAll(dir, 0o755))
		if _, err := p.PackageDir(dir, io.Discard); err == nil || !strings.Contains(err.Error(), "outside root") {
			t.Fatalf("expected outside root error, got %v", err)
		}
	})

	t.Run("missing manifest", func(t *testing.T) {
		dir := filepath.Join(root, "missing-manifest")
		requireNoError(t, os.MkdirAll(dir, 0o755))
		if _, err := p.PackageDir(dir, io.Discard); err == nil || !errors.Is(err, plugins.ErrManifestNotFound) {
			t.Fatalf("expected manifest not found, got %v", err)
		}
	})

	t.Run("invalid manifest json", func(t *testing.T) {
		dir := filepath.Join(root, "bad-json")
		requireNoError(t, os.MkdirAll(filepath.Join(dir, ".claude-plugin"), 0o755))
		requireNoError(t, os.WriteFile(filepath.Join(dir, ".claude-plugin", "plugin.json"), []byte("{"), 0o600))
		if _, err := p.PackageDir(dir, io.Discard); err == nil || !strings.Contains(err.Error(), "decode manifest") {
			t.Fatalf("expected decode manifest error, got %v", err)
		}
	})

	t.Run("invalid manifest fields", func(t *testing.T) {
		dir := filepath.Join(root, "invalid-fields")
		writeManifest(t, dir, plugins.Manifest{Name: "Bad Name", Version: "1.0.0"})
		if _, err := p.PackageDir(dir, io.Discard); err == nil || !strings.Contains(err.Error(), "invalid plugin name") {
			t.Fatalf("expected plugin name error, got %v", err)
		}
	})

	t.Run("writer failure", func(t *testing.T) {
		dir := filepath.Join(root, "writer-failure")
		writePlugin(t, dir)
		fw := &failingWriter{err: errors.New("sink exploded")}
		if _, err := p.PackageDir(dir, fw); err == nil || !strings.Contains(err.Error(), "sink exploded") {
			t.Fatalf("expected write error, got %v", err)
		}
	})

	t.Run("file without read permission", func(t *testing.T) {
		dir := filepath.Join(root, "unreadable")
		writePlugin(t, dir)
		secret := filepath.Join(dir, "secret.txt")
		requireNoError(t, os.WriteFile(secret, []byte("topsecret"), 0o600))
		requireNoError(t, os.Chmod(secret, 0))
		defer func() {
			// best-effort cleanup; ignore inability to restore permissions
			_ = os.Chmod(secret, 0o600) //nolint:errcheck
		}()
		if _, err := p.PackageDir(dir, io.Discard); err == nil {
			t.Fatalf("expected unreadable file error")
		}
	})

	t.Run("nil receiver", func(t *testing.T) {
		var nilPackager *Packager
		if _, err := nilPackager.PackageDir(root, io.Discard); err == nil || !strings.Contains(err.Error(), "instance is nil") {
			t.Fatalf("expected instance is nil error, got %v", err)
		}
	})
}

func TestImportErrorCases(t *testing.T) {
	root := t.TempDir()
	p, err := NewPackager(root, nil)
	requireNoError(t, err)

	t.Run("corrupted gzip", func(t *testing.T) {
		if _, err := p.Import(bytes.NewReader([]byte("not a gzip")), "broken"); err == nil {
			t.Fatalf("expected gzip error")
		}
	})

	t.Run("invalid tar stream", func(t *testing.T) {
		var buf bytes.Buffer
		gz := gzip.NewWriter(&buf)
		if _, err := gz.Write([]byte("not tar")); err != nil {
			t.Fatalf("write gzip: %v", err)
		}
		requireNoError(t, gz.Close())
		if _, err := p.Import(bytes.NewReader(buf.Bytes()), "bad-tar"); err == nil {
			t.Fatalf("expected tar error")
		}
	})

	t.Run("absolute path entry", func(t *testing.T) {
		archive := buildArchive(t, archiveEntry{header: tar.Header{Name: "/abs", Mode: 0o600, Size: int64(len("x"))}, body: []byte("x")})
		if _, err := p.Import(bytes.NewReader(archive), "abs"); err == nil || !errors.Is(err, ErrUnsafeArchive) {
			t.Fatalf("expected unsafe archive error, got %v", err)
		}
	})

	t.Run("invalid file mode", func(t *testing.T) {
		archive := buildArchive(t, archiveEntry{header: tar.Header{Name: "file.txt", Mode: 010000, Size: int64(len("x"))}, body: []byte("x")})
		if _, err := p.Import(bytes.NewReader(archive), "badmode"); err == nil || !strings.Contains(err.Error(), "invalid file mode") {
			t.Fatalf("expected file mode error, got %v", err)
		}
	})

	t.Run("destination creation denied", func(t *testing.T) {
		lockedRoot := t.TempDir()
		requireNoError(t, os.Chmod(lockedRoot, 0o555))
		lockedPackager, err := NewPackager(lockedRoot, nil)
		requireNoError(t, err)
		t.Cleanup(func() {
			// best-effort cleanup; permissions may already be gone
			_ = os.Chmod(lockedRoot, 0o755) //nolint:errcheck
		})
		archive := buildArchive(t, archiveEntry{header: tar.Header{Name: ".claude-plugin/plugin.json", Mode: 0o600, Size: int64(len("{}"))}, body: []byte("{}")})
		if _, err := lockedPackager.Import(bytes.NewReader(archive), "locked"); err == nil {
			t.Fatalf("expected permission error")
		}
	})

	t.Run("destination is file", func(t *testing.T) {
		fileRoot := t.TempDir()
		conflict := filepath.Join(fileRoot, "demo")
		requireNoError(t, os.WriteFile(conflict, []byte("x"), 0o600))
		lockingPackager, err := NewPackager(fileRoot, nil)
		requireNoError(t, err)
		archive := buildArchive(t, archiveEntry{header: tar.Header{Name: ".claude-plugin/plugin.json", Mode: 0o600, Size: int64(len("{}"))}, body: []byte("{}")})
		if _, err := lockingPackager.Import(bytes.NewReader(archive), "demo"); !errors.Is(err, ErrDestinationExists) {
			t.Fatalf("expected destination exists for file, got %v", err)
		}
	})

	t.Run("digest mismatch", func(t *testing.T) {
		manifest := plugins.Manifest{Name: "demo", Version: "1.0.0", Digest: strings.Repeat("0", 64)}
		data, err := json.Marshal(manifest)
		requireNoError(t, err)
		archive := buildArchive(t, archiveEntry{header: tar.Header{Name: ".claude-plugin/plugin.json", Mode: 0o600, Size: int64(len(data))}, body: data})
		if _, err := p.Import(bytes.NewReader(archive), "digest"); err == nil || !strings.Contains(err.Error(), "manifest digest mismatch") {
			t.Fatalf("expected digest mismatch, got %v", err)
		}
	})

	t.Run("signature required", func(t *testing.T) {
		store := plugins.NewTrustStore() // allowUnsigned defaults to false
		requirer, err := NewPackager(t.TempDir(), store)
		requireNoError(t, err)
		manifest := plugins.Manifest{Name: "demo", Version: "1.0.0"}
		data, err := json.Marshal(manifest)
		requireNoError(t, err)
		archive := buildArchive(t, archiveEntry{header: tar.Header{Name: ".claude-plugin/plugin.json", Mode: 0o600, Size: int64(len(data))}, body: data})
		if _, err := requirer.Import(bytes.NewReader(archive), "unsigned"); err == nil || !strings.Contains(err.Error(), "unsigned plugins are rejected") {
			t.Fatalf("expected unsigned rejection, got %v", err)
		}
	})
}

func TestRestoreEntryBranches(t *testing.T) {
	dest := t.TempDir()
	p, err := NewPackager(dest, nil)
	requireNoError(t, err)

	t.Run("creates directory", func(t *testing.T) {
		header := &tar.Header{Name: "nested/dir", Mode: 0o700, Typeflag: tar.TypeDir}
		requireNoError(t, p.restoreEntry(dest, header, bytes.NewReader(nil)))
		info, err := os.Stat(filepath.Join(dest, "nested", "dir"))
		requireNoError(t, err)
		if !info.IsDir() {
			t.Fatalf("expected directory")
		}
		if info.Mode().Perm() != 0o700 {
			t.Fatalf("unexpected mode %v", info.Mode().Perm())
		}
	})

	t.Run("copy failure", func(t *testing.T) {
		header := &tar.Header{Name: "broken.txt", Mode: 0o644, Typeflag: tar.TypeReg}
		errCopy := errors.New("copy failed")
		if err := p.restoreEntry(dest, header, &errReader{err: errCopy}); err == nil || !strings.Contains(err.Error(), "copy failed") {
			t.Fatalf("expected copy failure, got %v", err)
		}
	})

	t.Run("unsupported type ignored", func(t *testing.T) {
		header := &tar.Header{Name: "link", Mode: 0o644, Typeflag: tar.TypeSymlink}
		if err := p.restoreEntry(dest, header, bytes.NewReader(nil)); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("dot entry skipped", func(t *testing.T) {
		header := &tar.Header{Name: ".", Mode: 0o755, Typeflag: tar.TypeDir}
		if err := p.restoreEntry(dest, header, bytes.NewReader(nil)); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("target escapes dest", func(t *testing.T) {
		dirtyDest := filepath.Join("..", "unsafe-root") // intentionally relative so prefix check fails
		header := &tar.Header{Name: "escape", Mode: 0o644, Typeflag: tar.TypeReg}
		if err := p.restoreEntry(dirtyDest, header, bytes.NewReader([]byte("x"))); err == nil || !errors.Is(err, ErrUnsafeArchive) {
			t.Fatalf("expected unsafe archive, got %v", err)
		}
	})
}

func TestEnsureEmptyDirBranches(t *testing.T) {
	t.Run("empty directory allowed", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "empty")
		requireNoError(t, os.MkdirAll(dir, 0o755))
		if err := ensureEmptyDir(dir); err != nil {
			t.Fatalf("expected nil for empty dir, got %v", err)
		}
	})

	t.Run("stat blocked", func(t *testing.T) {
		parent := t.TempDir()
		locked := filepath.Join(parent, "locked")
		requireNoError(t, os.MkdirAll(locked, 0o700))
		requireNoError(t, os.Chmod(locked, 0))
		t.Cleanup(func() {
			// best-effort cleanup for test temp dir
			_ = os.Chmod(locked, 0o700) //nolint:errcheck
		})
		target := filepath.Join(locked, "child")
		if err := ensureEmptyDir(target); err == nil {
			t.Fatalf("expected stat error")
		}
	})

	t.Run("readdir error", func(t *testing.T) {
		locked := filepath.Join(t.TempDir(), "locked")
		requireNoError(t, os.MkdirAll(locked, 0o300))
		t.Cleanup(func() {
			_ = os.Chmod(locked, 0o700) //nolint:errcheck // cleanup best effort
		})
		if err := ensureEmptyDir(locked); err == nil {
			t.Fatalf("expected readdir error")
		}
	})
}

func writePlugin(t *testing.T, pluginDir string) {
	t.Helper()
	requireNoError(t, os.MkdirAll(filepath.Join(pluginDir, ".claude-plugin"), 0o755))
	requireNoError(t, os.WriteFile(filepath.Join(pluginDir, "README.md"), []byte("demo"), 0o600))
	mf := plugins.Manifest{Name: "demo", Version: "1.0.0"}
	data, err := json.Marshal(mf)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	requireNoError(t, os.WriteFile(filepath.Join(pluginDir, ".claude-plugin", "plugin.json"), data, 0o600))
}

func requireNoError(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func writeManifest(t *testing.T, pluginDir string, manifest plugins.Manifest) {
	t.Helper()
	requireNoError(t, os.MkdirAll(filepath.Join(pluginDir, ".claude-plugin"), 0o755))
	data, err := json.Marshal(manifest)
	requireNoError(t, err)
	requireNoError(t, os.WriteFile(filepath.Join(pluginDir, ".claude-plugin", "plugin.json"), data, 0o600))
}

type failingWriter struct {
	err error
}

func (f *failingWriter) Write(_ []byte) (int, error) {
	return 0, f.err
}

type errReader struct {
	err error
}

func (e *errReader) Read(_ []byte) (int, error) {
	return 0, e.err
}

type archiveEntry struct {
	header tar.Header
	body   []byte
}

func buildArchive(t *testing.T, entries ...archiveEntry) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for _, e := range entries {
		h := e.header
		if h.Size == 0 {
			h.Size = int64(len(e.body))
		}
		requireNoError(t, tw.WriteHeader(&h))
		if len(e.body) > 0 {
			if _, err := tw.Write(e.body); err != nil {
				t.Fatalf("write body: %v", err)
			}
		}
	}
	requireNoError(t, tw.Close())
	requireNoError(t, gz.Close())
	return buf.Bytes()
}
