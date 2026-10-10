package gitx

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// WorktreeChange describes a path reported by Git, without reading its contents.
// Tracking and file kind are independent: either tracked or untracked paths can
// be symlinks. OriginalPath is populated for a rename or copy.
type WorktreeChange struct {
	Path           string
	OriginalPath   string
	Status         string
	Tracked        bool
	Kind           string
	SubmoduleState string
}

func (c WorktreeChange) String() string {
	tracking := "untracked"
	if c.Tracked {
		tracking = "tracked"
	}
	kind := c.Kind
	if c.SubmoduleState != "" {
		kind += " " + c.SubmoduleState
	}
	out := fmt.Sprintf("%q [%s, %s, status=%s]", c.Path, tracking, kind, c.Status)
	if c.OriginalPath != "" {
		out += fmt.Sprintf(" (from %q)", c.OriginalPath)
	}
	return out
}

// WorktreeChanges uses the same Git ignore/pathspec/submodule rules as Clean.
// Porcelain v2 supplies file modes and submodule state without following links.
func (r *Repo) WorktreeChanges(excludes []string) ([]WorktreeChange, error) {
	args := append([]string{"status", "--porcelain=v2", "-z", "--untracked-files=all", "--ignore-submodules=none", "--"}, Pathspec(excludes)...)
	out, err := r.run(args...)
	if err != nil {
		return nil, err
	}
	changes, err := parseWorktreeChanges(out)
	if err != nil {
		return nil, err
	}
	for i := range changes {
		c := &changes[i]
		if c.Tracked {
			continue
		}
		info, err := os.Lstat(filepath.Join(r.Root, filepath.FromSlash(c.Path)))
		switch {
		case os.IsNotExist(err):
			c.Kind = "missing"
		case err != nil:
			return nil, fmt.Errorf("cannot inspect untracked path %q", c.Path)
		case info.Mode()&os.ModeSymlink != 0:
			c.Kind = "symlink"
		case info.IsDir():
			c.Kind = "directory"
		default:
			c.Kind = "file"
		}
	}
	return changes, nil
}

func parseWorktreeChanges(out []byte) ([]WorktreeChange, error) {
	changes := []WorktreeChange{}
	if len(out) == 0 {
		return changes, nil
	}
	if out[len(out)-1] != 0 {
		return nil, fmt.Errorf("Git status returned an unterminated record")
	}
	fields := strings.Split(string(out[:len(out)-1]), "\x00")
	for i := 0; i < len(fields); i++ {
		entry := fields[i]
		if len(entry) < 3 || entry[1] != ' ' {
			return nil, fmt.Errorf("Git status returned a malformed record")
		}
		c := WorktreeChange{Tracked: true}
		var modes []string
		switch entry[0] {
		case '?':
			c.Path, c.Status, c.Tracked = entry[2:], "??", false
		case '1', '2', 'u':
			count := map[byte]int{'1': 9, '2': 10, 'u': 11}[entry[0]]
			parts := strings.SplitN(entry, " ", count)
			if len(parts) != count || len(parts[1]) != 2 || len(parts[2]) != 4 {
				return nil, fmt.Errorf("Git status returned malformed tracked metadata")
			}
			c.Path, c.Status = parts[count-1], parts[1]
			modes = parts[3:6]
			if entry[0] == 'u' {
				modes = parts[3:7]
			}
			if parts[2][0] == 'S' {
				c.SubmoduleState = parts[2]
			}
			if entry[0] == '2' {
				i++
				if i >= len(fields) {
					return nil, fmt.Errorf("Git status omitted a rename source")
				}
				c.OriginalPath = fields[i]
			}
		default:
			return nil, fmt.Errorf("Git status returned an unsupported record type")
		}
		if !worktreePath(c.Path) || (c.OriginalPath != "" && !worktreePath(c.OriginalPath)) {
			return nil, fmt.Errorf("Git status returned a path outside the working tree")
		}
		for j := len(modes) - 1; j >= 0; j-- {
			if modes[j] == "000000" {
				continue
			}
			switch modes[j] {
			case "120000":
				c.Kind = "symlink"
			case "160000":
				c.Kind = "submodule"
			default:
				c.Kind = "file"
			}
			break
		}
		changes = append(changes, c)
	}
	sort.Slice(changes, func(i, j int) bool { return changes[i].Path < changes[j].Path })
	return changes, nil
}

func worktreePath(name string) bool {
	clean := filepath.Clean(filepath.FromSlash(name))
	return name != "" && !filepath.IsAbs(clean) && clean != "." && clean != ".." && !strings.HasPrefix(clean, ".."+string(filepath.Separator))
}
