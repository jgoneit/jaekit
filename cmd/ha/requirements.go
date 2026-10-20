package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"sort"
)

const (
	coreRequirementsFile   = "core-requirements.json"
	coreRequirementsSchema = "core-requirements/v1"
)

// coreRequirements is a skill's declaration of the Core support it needs
// (core-capabilities.md §8). Absent lists require nothing.
type coreRequirements struct {
	Rules    []string
	Formats  map[string][]string
	Features []string
}

// readCoreRequirements reads core-requirements.json beside the skill file.
// It returns nil without an error when the skill has no declaration.
func readCoreRequirements(skillMD string) (*coreRequirements, string, error) {
	path := filepath.Join(filepath.Dir(skillMD), coreRequirementsFile)
	data, err := readUserFile(path)
	if errors.Is(err, errUserFileAbsent) {
		return nil, path, nil
	}
	if err != nil {
		return nil, path, err
	}
	var raw struct {
		Schema   json.RawMessage `json:"schema"`
		Rules    json.RawMessage `json:"rules"`
		Formats  json.RawMessage `json:"formats"`
		Features json.RawMessage `json:"features"`
	}
	if err := strictJSON(data, &raw); err != nil {
		return nil, path, err
	}
	var schema string
	if raw.Schema == nil || json.Unmarshal(raw.Schema, &schema) != nil || schema != coreRequirementsSchema {
		return nil, path, fmt.Errorf("schema must be %q", coreRequirementsSchema)
	}
	req := &coreRequirements{}
	if req.Rules, err = requirementList("rules", raw.Rules); err != nil {
		return nil, path, err
	}
	if req.Features, err = requirementList("features", raw.Features); err != nil {
		return nil, path, err
	}
	if raw.Formats != nil {
		var formats map[string]json.RawMessage
		if string(raw.Formats) == "null" || json.Unmarshal(raw.Formats, &formats) != nil {
			return nil, path, errors.New("formats must be an object of string lists")
		}
		req.Formats = map[string][]string{}
		for key, value := range formats {
			if key == "" {
				return nil, path, errors.New("formats has an empty key")
			}
			if req.Formats[key], err = requirementList("formats."+key, value); err != nil {
				return nil, path, err
			}
		}
	}
	return req, path, nil
}

// requirementList reads an optional list of distinct, non-empty strings.
func requirementList(name string, raw json.RawMessage) ([]string, error) {
	if raw == nil {
		return nil, nil
	}
	var values []*string
	if string(raw) == "null" || json.Unmarshal(raw, &values) != nil {
		return nil, fmt.Errorf("%s must be a list of strings", name)
	}
	var out []string
	for _, v := range values {
		if v == nil || *v == "" {
			return nil, fmt.Errorf("%s has an empty or null entry", name)
		}
		if slices.Contains(out, *v) {
			return nil, fmt.Errorf("%s lists %q twice", name, *v)
		}
		out = append(out, *v)
	}
	return out, nil
}

// missing lists each declared item this executable does not support, as
// `rules <value>`, `formats.<key> <value>` or `features <value>`.
func (r *coreRequirements) missing(have capabilityReport) []string {
	var out []string
	for _, rule := range r.Rules {
		if !slices.Contains(have.SupportedRules, rule) {
			out = append(out, "rules "+rule)
		}
	}
	keys := make([]string, 0, len(r.Formats))
	for key := range r.Formats {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		supported, known := have.Formats[key]
		for _, value := range r.Formats[key] {
			if !known || !slices.Contains(supported, value) {
				out = append(out, "formats."+key+" "+value)
			}
		}
	}
	for _, feature := range r.Features {
		if !slices.Contains(have.Features, feature) {
			out = append(out, "features "+feature)
		}
	}
	return out
}

// checkSkillRequirements refuses the start when the skill's declaration is
// unusable or names support this executable lacks; required is --check-skill,
// which also refuses a skill without a declaration. It returns -1 to go on.
func (c *cli) checkSkillRequirements(skillMD string, required bool) int {
	req, path, err := readCoreRequirements(skillMD)
	if err != nil {
		return c.fail(exitRefused, "skill requirements %s: %v; nothing recorded", path, err)
	}
	if req == nil {
		if required {
			return c.fail(exitRefused, "skill requirements %s: absent; --check-skill needs the skill's %s declaration; nothing recorded", path, coreRequirementsFile)
		}
		return -1
	}
	missing := req.missing(supportedCapabilities())
	if len(missing) == 0 {
		return -1
	}
	for _, item := range missing {
		fmt.Fprintf(c.err, "ha: missing Core support: %s (ha %s)\n", item, version)
	}
	return c.fail(exitRefused, "the skill %s needs Core support this ha does not provide; nothing recorded", skillMD)
}
