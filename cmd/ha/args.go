package main

import (
	"fmt"
	"strings"
)

// args holds positional arguments and --flags. Flags may appear anywhere
// after the command; `--name value` and `--name=value` are both accepted.
type args struct {
	pos   []string
	vals  map[string]string
	bools map[string]bool
	lists map[string][]string
}

func parseArgs(in []string, valueFlags, boolFlags []string) (*args, error) {
	return parseFlags(in, valueFlags, nil, boolFlags)
}

// parseFlags also accepts list flags, which may repeat and keep their order.
func parseFlags(in []string, valueFlags, listFlags, boolFlags []string) (*args, error) {
	a := &args{vals: map[string]string{}, bools: map[string]bool{}, lists: map[string][]string{}}
	isValue := map[string]bool{}
	for _, f := range valueFlags {
		isValue[f] = true
	}
	isList := map[string]bool{}
	for _, f := range listFlags {
		isList[f] = true
		isValue[f] = true
	}
	isBool := map[string]bool{}
	for _, f := range boolFlags {
		isBool[f] = true
	}
	for i := 0; i < len(in); i++ {
		s := in[i]
		if s == "--" {
			a.pos = append(a.pos, in[i+1:]...)
			break
		}
		if !strings.HasPrefix(s, "--") {
			a.pos = append(a.pos, s)
			continue
		}
		name, value, hasValue := strings.Cut(strings.TrimPrefix(s, "--"), "=")
		switch {
		case isBool[name]:
			if hasValue {
				return nil, fmt.Errorf("--%s takes no value", name)
			}
			a.bools[name] = true
		case isValue[name]:
			if !hasValue {
				if i+1 >= len(in) {
					return nil, fmt.Errorf("--%s needs a value", name)
				}
				i++
				value = in[i]
			}
			if isList[name] {
				a.lists[name] = append(a.lists[name], value)
				continue
			}
			// Commands that take list flags refuse a repeated single value;
			// the older commands keep their last-value behavior.
			if _, repeated := a.vals[name]; repeated && listFlags != nil {
				return nil, fmt.Errorf("--%s is given more than once", name)
			}
			a.vals[name] = value
		default:
			return nil, fmt.Errorf("unknown flag --%s", name)
		}
	}
	return a, nil
}
