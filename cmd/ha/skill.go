package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/jgoneit/jaekit/internal/goaldocs"
	"github.com/jgoneit/jaekit/internal/record"
)

// readSkill reads name and version from a SKILL.md front matter and digests
// every file in the skill directory. The values remain claims: the agent
// chooses which file to point at.
func readSkill(skillMD string) (record.Skill, error) {
	data, err := os.ReadFile(skillMD)
	if err != nil {
		return record.Skill{}, err
	}
	name, version := frontMatter(string(data))
	if name == "" {
		return record.Skill{}, fmt.Errorf("%s has no `name` in its front matter", skillMD)
	}
	digest, err := dirDigest(filepath.Dir(skillMD))
	if err != nil {
		return record.Skill{}, err
	}
	return record.Skill{Name: name, Version: version, Digest: digest}, nil
}

// frontMatter reads `name` and `metadata.version` (or a top-level `version`)
// from YAML front matter. Only the flat key: value form is supported.
func frontMatter(src string) (name, version string) {
	lines := strings.Split(strings.ReplaceAll(src, "\r\n", "\n"), "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return "", ""
	}
	inMetadata := false
	for _, l := range lines[1:] {
		if strings.TrimSpace(l) == "---" {
			break
		}
		indented := strings.HasPrefix(l, " ") || strings.HasPrefix(l, "\t")
		key, value, ok := strings.Cut(strings.TrimSpace(l), ":")
		if !ok {
			continue
		}
		value = strings.Trim(strings.TrimSpace(value), `"'`)
		switch {
		case !indented && key == "name":
			name = value
			inMetadata = false
		case !indented && key == "metadata":
			inMetadata = true
		case !indented && key == "version":
			version = value
			inMetadata = false
		case indented && inMetadata && key == "version":
			version = value
		case !indented:
			inMetadata = false
		}
	}
	return name, version
}

func dirDigest(dir string) (string, error) {
	content := map[string]string{}
	var paths []string
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if p != dir && strings.HasPrefix(d.Name(), ".") {
				return filepath.SkipDir
			}
			return nil
		}
		rel, err := filepath.Rel(dir, p)
		if err != nil {
			return err
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		content[rel] = string(b)
		paths = append(paths, rel)
		return nil
	})
	if err != nil {
		return "", err
	}
	sort.Strings(paths)
	return goaldocs.Digest(paths, content), nil
}

func sha(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
