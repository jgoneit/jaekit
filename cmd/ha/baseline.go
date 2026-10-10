package main

import (
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/jgoneit/jaekit/internal/gitx"
	"github.com/jgoneit/jaekit/internal/record"
)

type baselineFile struct {
	path string
	data []byte
	mode os.FileMode
}

// baselineFiles reads the whole source list before any overlay is applied.
// Git blobs are read directly: a source symlink must never become a text file.
func baselineFiles(repo *gitx.Repo, base, head string, paths []string) ([]baselineFile, error) {
	files := make([]baselineFile, 0, len(paths))
	for _, p := range paths {
		if !filepath.IsLocal(p) || p == "." || path.Clean(p) != p || strings.Contains("/"+p+"/", "/.git/") {
			return nil, fmt.Errorf("check path %q must be a repository-relative file path", p)
		}
		mode, err := repo.Mode(head, ":(literal)"+p)
		if err != nil {
			return nil, fmt.Errorf("check path %q: inspect HEAD file mode: %w", p, err)
		}
		if mode != "100644" && mode != "100755" {
			return nil, fmt.Errorf("check path %q must be committed as a regular file at HEAD (mode %q)", p, mode)
		}
		// core.symlinks=false materializes a committed link as a regular
		// text file. The base tree, not that representation, defines safety.
		parts := strings.Split(p, "/")
		for i := range parts {
			component := strings.Join(parts[:i+1], "/")
			baseMode, err := repo.Mode(base, ":(literal)"+component)
			if err != nil {
				return nil, fmt.Errorf("check path %q: inspect base component %q: %w", p, component, err)
			}
			if baseMode == "" {
				break
			}
			if baseMode == "120000" {
				return nil, fmt.Errorf("check path %q: base component %q is a symlink", p, component)
			}
			if i < len(parts)-1 && baseMode != "040000" {
				return nil, fmt.Errorf("check path %q: base parent %q is not a directory", p, component)
			}
			if i == len(parts)-1 && baseMode != "100644" && baseMode != "100755" {
				return nil, fmt.Errorf("check path %q: base destination is not a regular file", p)
			}
		}
		data, ok, err := repo.Cat(head, p)
		if err != nil {
			return nil, fmt.Errorf("check path %q: read HEAD file: %w", p, err)
		}
		if !ok {
			return nil, fmt.Errorf("check path %q does not exist at HEAD; commit it before the baseline check", p)
		}
		perm := os.FileMode(0o644)
		if mode == "100755" {
			perm = 0o755
		}
		files = append(files, baselineFile{p, data, perm})
	}
	return files, nil
}

// baselineDestination rejects all symlinks, including internal and dangling
// links. Root additionally confines operations if a path changes between calls.
func baselineDestination(root *os.Root, p string) error {
	parts := strings.Split(p, "/")
	for i := range parts {
		component := strings.Join(parts[:i+1], "/")
		info, err := root.Lstat(filepath.FromSlash(component))
		if errors.Is(err, os.ErrNotExist) {
			return nil // This component and its descendants can be created safely.
		}
		if err != nil {
			return fmt.Errorf("check path %q: inspect baseline component %q: %w", p, component, err)
		}
		switch {
		case info.Mode()&os.ModeSymlink != 0:
			return fmt.Errorf("check path %q: baseline component %q is a symlink", p, component)
		case i < len(parts)-1 && !info.IsDir():
			return fmt.Errorf("check path %q: baseline parent %q is not a directory", p, component)
		case i == len(parts)-1 && !info.Mode().IsRegular():
			return fmt.Errorf("check path %q: baseline destination is not a regular file", p)
		}
	}
	return nil
}

func applyBaselineFile(root *os.Root, file baselineFile) error {
	if err := baselineDestination(root, file.path); err != nil {
		return err
	}
	dir := path.Dir(file.path)
	if err := root.MkdirAll(filepath.FromSlash(dir), 0o755); err != nil {
		return fmt.Errorf("check path %q: create baseline parent: %w", file.path, err)
	}
	// Never truncate the destination or chmod it by path. Prepare a new regular
	// file, set its permissions through the descriptor, then replace the entry.
	temp := filepath.FromSlash(path.Join(dir, ".ha-overlay-"+rand.Text()))
	f, err := root.OpenFile(temp, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("check path %q: prepare baseline file: %w", file.path, err)
	}
	defer root.Remove(temp)
	n, err := f.Write(file.data)
	if err == nil && n != len(file.data) {
		err = io.ErrShortWrite
	}
	if err == nil {
		err = f.Chmod(file.mode)
	}
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return fmt.Errorf("check path %q: write baseline file: %w", file.path, err)
	}
	if err := baselineDestination(root, file.path); err != nil {
		return err
	}
	if err := root.Rename(temp, filepath.FromSlash(file.path)); err != nil {
		return fmt.Errorf("check path %q: replace baseline file: %w", file.path, err)
	}
	return nil
}

func applyBaseline(root *os.Root, files []baselineFile) ([]record.Overlay, error) {
	for _, file := range files {
		if err := baselineDestination(root, file.path); err != nil {
			return nil, err
		}
	}
	var overlay []record.Overlay
	for _, file := range files {
		if err := applyBaselineFile(root, file); err != nil {
			return nil, err
		}
		overlay = append(overlay, record.Overlay{Path: file.path, Digest: sha(file.data)})
	}
	// A later preparation step must not leave an earlier path observably unsafe
	// when the command is about to start.
	for _, file := range files {
		if err := baselineDestination(root, file.path); err != nil {
			return nil, err
		}
	}
	return overlay, nil
}
