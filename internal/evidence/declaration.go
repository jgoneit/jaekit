// Package evidence binds a declared behavioral check to one process invocation.
// It consumes structured observations, never natural-language log output.
package evidence

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

const (
	DeclarationSchema = "check-declaration/v1"
	ReportSchema      = "check-result/v1"
	SummarySchema     = "check-evidence/v1"
	maxJSONBytes      = 1 << 20
)

type Target struct {
	ID        string `json:"id"`
	Violation string `json:"violation"`
}

type Declaration struct {
	Schema          string   `json:"schema"`
	Producer        string   `json:"producer"`
	ProducerVersion string   `json:"producer_version"`
	ProducerPaths   []string `json:"producer_paths"`
	Targets         []Target `json:"targets"`
}

var identifier = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.:/-]{0,127}$`)
var digestPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

func digest(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

// ValidPath accepts a canonical repository-relative file path.
func ValidPath(p string) bool {
	return p != "" && p != "." && p != ".." && !strings.HasPrefix(p, "../") &&
		!path.IsAbs(p) && path.Clean(p) == p && !strings.ContainsAny(p, "\\\x00\r\n\t") &&
		!strings.Contains("/"+p+"/", "/.git/")
}

// ReadInput reads a tracked regular input without traversing symlinks. No Git
// mutation or source rewrite is performed. Inputs must equal HEAD even when
// their paths are otherwise excluded from ordinary working-tree checks.
func ReadInput(root, p string) ([]byte, error) {
	if !ValidPath(p) {
		return nil, fmt.Errorf("invalid evidence path %q", p)
	}
	cur := root
	parts := strings.Split(p, "/")
	for i, part := range parts {
		cur = filepath.Join(cur, part)
		info, err := os.Lstat(cur)
		if err != nil {
			return nil, fmt.Errorf("evidence input %q is unavailable", p)
		}
		if info.Mode()&os.ModeSymlink != 0 || (i < len(parts)-1 && !info.IsDir()) || (i == len(parts)-1 && !info.Mode().IsRegular()) {
			return nil, fmt.Errorf("evidence input %q must be a regular file without symlink components", p)
		}
	}
	git := func(args ...string) ([]byte, error) {
		cmd := exec.Command("git", args...)
		cmd.Dir = root
		cmd.Env = append(os.Environ(), "GIT_OPTIONAL_LOCKS=0", "LC_ALL=C")
		return cmd.Output()
	}
	entry, err := git("ls-tree", "-z", "HEAD", "--", ":(literal)"+p)
	if err != nil || (!bytes.HasPrefix(entry, []byte("100644 ")) && !bytes.HasPrefix(entry, []byte("100755 "))) {
		return nil, fmt.Errorf("evidence input %q must be committed as a regular file at HEAD", p)
	}
	committed, err := git("show", "HEAD:"+p)
	if err != nil {
		return nil, fmt.Errorf("evidence input %q cannot be read at HEAD", p)
	}
	data, err := os.ReadFile(cur)
	if err != nil {
		return nil, err
	}
	if !bytes.Equal(data, committed) {
		return nil, fmt.Errorf("evidence input %q differs from HEAD; commit it before checking", p)
	}
	return data, nil
}

// Load validates a declaration and binds its raw bytes and every producer file.
func Load(root, p string) (*Declaration, string, error) {
	data, err := ReadInput(root, p)
	if err != nil {
		return nil, "", err
	}
	var d Declaration
	if err := strictJSON(data, &d); err != nil {
		return nil, "", errors.New("invalid evidence declaration JSON")
	}
	if err := d.Validate(); err != nil {
		return nil, "", err
	}
	inputs := map[string]string{p: digest(data)}
	for _, producer := range d.ProducerPaths {
		if producer == p {
			return nil, "", errors.New("the declaration cannot also be a producer path")
		}
		content, err := ReadInput(root, producer)
		if err != nil {
			return nil, "", err
		}
		inputs[producer] = digest(content)
	}
	paths := make([]string, 0, len(inputs))
	for p := range inputs {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	var binding strings.Builder
	for _, p := range paths {
		fmt.Fprintf(&binding, "%s\x00%s\n", p, inputs[p])
	}
	return &d, digest([]byte(binding.String())), nil
}

func (d *Declaration) Validate() error {
	if d == nil || d.Schema != DeclarationSchema || !identifier.MatchString(d.Producer) || !identifier.MatchString(d.ProducerVersion) {
		return errors.New("unsupported evidence declaration or producer")
	}
	if len(d.ProducerPaths) == 0 || len(d.ProducerPaths) > 128 || len(d.Targets) == 0 || len(d.Targets) > 1024 {
		return errors.New("evidence declaration needs producer paths and one to 1024 targets")
	}
	paths, targets := map[string]bool{}, map[string]bool{}
	for _, p := range d.ProducerPaths {
		if !ValidPath(p) || paths[p] {
			return errors.New("invalid or duplicate evidence producer path")
		}
		paths[p] = true
	}
	for _, t := range d.Targets {
		if !identifier.MatchString(t.ID) || !identifier.MatchString(t.Violation) || targets[t.ID] {
			return errors.New("invalid or duplicate evidence target")
		}
		targets[t.ID] = true
	}
	return nil
}

// strictJSON rejects duplicate keys as well as unknown fields and trailing data.
// Ambiguous reports must not acquire meaning from a decoder's last-key policy.
func strictJSON(data []byte, dst any) error {
	if len(data) > maxJSONBytes {
		return errors.New("JSON exceeds limit")
	}
	d := json.NewDecoder(bytes.NewReader(data))
	var value func(int) error
	value = func(depth int) error {
		if depth > 32 {
			return errors.New("JSON nesting exceeds limit")
		}
		token, err := d.Token()
		if err != nil {
			return err
		}
		if delim, ok := token.(json.Delim); ok {
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
						return errors.New("duplicate or invalid JSON key")
					}
					seen[name] = true
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
		}
		return err
	}
	if err := value(0); err != nil {
		return err
	}
	if _, err := d.Token(); err != io.EOF {
		return errors.New("trailing JSON data")
	}
	d = json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	return d.Decode(dst)
}
