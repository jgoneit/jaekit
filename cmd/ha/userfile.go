package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"syscall"
	"unicode/utf8"
)

// maxUserFileBytes bounds the user files ha reads as small JSON objects: the
// pre-approved run limit and a skill's Core requirements.
const maxUserFileBytes = 64 << 10

var utf8BOM = []byte{0xef, 0xbb, 0xbf}

// errUserFileAbsent is returned when no entry exists at a user file's path.
var errUserFileAbsent = errors.New("absent")

// readUserFile reads a small JSON file the user or a skill supplies. A path
// with no entry is errUserFileAbsent; an entry that exists but is not a
// readable regular file within maxUserFileBytes is an error, so a broken
// symbolic link or a folder is never mistaken for an absent file. On every
// platform an ancestor that exists but is not a folder is an error too.
func readUserFile(path string) ([]byte, error) {
	if _, err := os.Lstat(path); err != nil {
		if ancestor := notFolderAncestor(path); ancestor != nil {
			return nil, ancestor
		}
		if !errors.Is(err, fs.ErrNotExist) {
			return nil, err
		}
		return nil, errUserFileAbsent
	}
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, errors.New("not a regular file")
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, maxUserFileBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxUserFileBytes {
		return nil, fmt.Errorf("larger than %d bytes", maxUserFileBytes)
	}
	return data, nil
}

// notFolderAncestor reports the nearest existing ancestor of path when it is
// not a folder, including a symbolic link whose target is gone. Unix reports
// a path below a file as ENOTDIR, Windows as a missing path; both mean the
// same thing here.
func notFolderAncestor(path string) error {
	for dir := filepath.Dir(path); ; dir = filepath.Dir(dir) {
		if _, err := os.Lstat(dir); err != nil {
			if !missingEntry(err) {
				return err
			}
			if filepath.Dir(dir) == dir {
				return nil
			}
			continue
		}
		info, err := os.Stat(dir)
		if missingEntry(err) {
			return fmt.Errorf("%s is a symbolic link whose target does not exist", dir)
		}
		if err != nil {
			return err
		}
		if !info.IsDir() {
			return fmt.Errorf("%s is not a folder", dir)
		}
		return nil
	}
}

// missingEntry is an error that says no entry exists at a path, including a
// path below a file.
func missingEntry(err error) bool {
	return errors.Is(err, fs.ErrNotExist) || errors.Is(err, syscall.ENOTDIR)
}

// strictJSON decodes one UTF-8 JSON value into dst. A single leading UTF-8
// byte order mark is allowed. Duplicate keys at any depth, unknown fields and
// trailing data are refused, and the keys of a struct target's object must
// spell its field names exactly, so no meaning comes from a decoder's
// last-key, ignore-unknown or letter-case policy.
func strictJSON(data []byte, dst any) error {
	data = bytes.TrimPrefix(data, utf8BOM)
	if !utf8.Valid(data) {
		return errors.New("not valid UTF-8")
	}
	d := json.NewDecoder(bytes.NewReader(data))
	var top []string // the keys of a top-level object
	var value func(int) error
	value = func(depth int) error {
		if depth > 32 {
			return errors.New("JSON nesting exceeds 32")
		}
		token, err := d.Token()
		if err != nil {
			return err
		}
		delim, ok := token.(json.Delim)
		if !ok {
			return nil
		}
		switch delim {
		case '{':
			seen := map[string]bool{}
			for d.More() {
				key, err := d.Token()
				if err != nil {
					return err
				}
				name, ok := key.(string)
				if !ok || seen[name] {
					return fmt.Errorf("duplicate JSON key %q", name)
				}
				seen[name] = true
				if depth == 0 {
					top = append(top, name)
				}
				if err := value(depth + 1); err != nil {
					return err
				}
			}
		case '[':
			for d.More() {
				if err := value(depth + 1); err != nil {
					return err
				}
			}
		default:
			return errors.New("invalid JSON delimiter")
		}
		_, err = d.Token()
		return err
	}
	if err := value(0); err != nil {
		return fmt.Errorf("not valid JSON: %v", err)
	}
	if _, err := d.Token(); err != io.EOF {
		return errors.New("data after the JSON value")
	}
	if names := jsonFieldNames(dst); names != nil {
		for _, key := range top {
			if !names[key] {
				return fmt.Errorf("unknown JSON key %q (keys are matched exactly)", key)
			}
		}
	}
	d = json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if err := d.Decode(dst); err != nil {
		return err
	}
	return nil
}

// jsonFieldNames is the set of JSON names of dst's fields when dst points to
// a struct, and nil otherwise. encoding/json matches object keys to these
// names without regard to letter case; strictJSON matches them exactly.
func jsonFieldNames(dst any) map[string]bool {
	t := reflect.TypeOf(dst)
	for t != nil && t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t == nil || t.Kind() != reflect.Struct {
		return nil
	}
	names := map[string]bool{}
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		name, _, _ := strings.Cut(f.Tag.Get("json"), ",")
		if name == "-" || !f.IsExported() {
			continue
		}
		if name == "" {
			name = f.Name
		}
		names[name] = true
	}
	return names
}
