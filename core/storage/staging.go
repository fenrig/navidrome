package storage

import (
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// StagedPath returns a local filesystem path for a file in the given library.
// Local file libraries return their direct path with no cleanup needed.
// Remote libraries are copied into a temporary file and a cleanup function is
// returned to remove the staged copy after use.
func StagedPath(libraryPath, relPath string) (path string, cleanup func() error, err error) {
	if localPath, ok := localLibraryPath(libraryPath, relPath); ok {
		return localPath, nil, nil
	}

	s, err := For(libraryPath)
	if err != nil {
		return "", nil, err
	}
	opener, ok := s.(FileOpener)
	if !ok {
		return "", nil, fmt.Errorf("storage %q does not support direct file open", libraryPath)
	}

	src, err := opener.Open(relPath)
	if err != nil {
		return "", nil, err
	}
	defer src.Close()

	tmp, err := os.CreateTemp("", "navidrome-stage-*")
	if err != nil {
		return "", nil, err
	}

	if _, err := io.Copy(tmp, src); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmp.Name())
		return "", nil, err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmp.Name())
		return "", nil, err
	}

	return tmp.Name(), func() error {
		return os.Remove(tmp.Name())
	}, nil
}

func localLibraryPath(libraryPath, relPath string) (string, bool) {
	if !strings.Contains(libraryPath, "://") {
		return filepath.Join(libraryPath, filepath.FromSlash(relPath)), true
	}
	u, err := url.Parse(libraryPath)
	if err != nil || u.Scheme == "" || u.Scheme == LocalSchemaID {
		base := libraryPath
		if err == nil && u.Scheme == LocalSchemaID {
			base = u.Path
			if u.Host != "" {
				base = filepath.Join(string(filepath.Separator)+u.Host, u.Path)
			}
		}
		return filepath.Join(base, filepath.FromSlash(relPath)), true
	}
	return "", false
}
