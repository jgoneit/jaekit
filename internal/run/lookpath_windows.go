package run

import (
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// LookPath is exec.LookPath for this environment: the current directory
// first unless NoDefaultCurrentDirectoryInExePath is set, then each %PATH%
// entry, trying the %PATHEXT% extensions, with relative names found against
// its working directory. An executable found relative to the working
// directory is exec.ErrDot unless the environment sets GODEBUG=execerrdot=0.
func (e Environment) LookPath(file string) (string, error) {
	if e.Env == nil && e.Dir == "" {
		return exec.LookPath(file)
	}
	if invalidLookPath(file) {
		return "", &exec.Error{Name: file, Err: exec.ErrNotFound}
	}
	return e.lookPathExts(file, e.pathExt())
}

func (e Environment) pathExt() []string {
	x := e.Getenv("PATHEXT")
	if x == "" {
		return []string{".com", ".exe", ".bat", ".cmd"}
	}
	var exts []string
	for _, ext := range strings.Split(strings.ToLower(x), ";") {
		if ext == "" {
			continue
		}
		if ext[0] != '.' {
			ext = "." + ext
		}
		exts = append(exts, ext)
	}
	return exts
}

func (e Environment) chkStat(file string) error {
	info, err := os.Stat(e.at(file))
	if err != nil {
		return err
	}
	if info.IsDir() {
		return fs.ErrPermission
	}
	return nil
}

func hasExt(file string) bool {
	i := strings.LastIndex(file, ".")
	return i >= 0 && strings.LastIndexAny(file, `:\/`) < i
}

func (e Environment) findExecutable(file string, exts []string) (string, error) {
	if len(exts) == 0 {
		return file, e.chkStat(file)
	}
	if hasExt(file) && e.chkStat(file) == nil {
		return file, nil
	}
	for _, ext := range exts {
		if f := file + ext; e.chkStat(f) == nil {
			return f, nil
		}
	}
	if hasExt(file) {
		return "", fs.ErrNotExist
	}
	return "", exec.ErrNotFound
}

func (e Environment) lookPathExts(file string, exts []string) (string, error) {
	if strings.ContainsAny(file, `:\/`) {
		f, err := e.findExecutable(file, exts)
		if err == nil {
			return f, nil
		}
		return "", &exec.Error{Name: file, Err: err}
	}
	errDotAllowed := e.errDotAllowed()
	var dotf string
	var dotErr error
	if _, found := e.LookupEnv("NoDefaultCurrentDirectoryInExePath"); !found {
		if f, err := e.findExecutable(filepath.Join(".", file), exts); err == nil {
			if errDotAllowed {
				return f, nil
			}
			dotf, dotErr = f, &exec.Error{Name: file, Err: exec.ErrDot}
		}
	}
	for _, dir := range filepath.SplitList(e.Getenv("path")) {
		if dir == "" {
			continue // as PowerShell does
		}
		f, err := e.findExecutable(filepath.Join(dir, file), exts)
		if err != nil {
			continue
		}
		if dotErr != nil {
			// The same executable through %PATH% is preferred to the implicit
			// relative one (go.dev/issue/53536); another one is not.
			dotInfo, dotInfoErr := os.Lstat(e.at(dotf))
			info, infoErr := os.Lstat(e.at(f))
			if dotInfoErr != nil || infoErr != nil || !os.SameFile(dotInfo, info) {
				return dotf, dotErr
			}
		}
		if !filepath.IsAbs(f) && !errDotAllowed {
			if dotErr == nil {
				dotf, dotErr = f, &exec.Error{Name: file, Err: exec.ErrDot}
			}
			continue
		}
		return f, nil
	}
	if dotErr != nil {
		return dotf, dotErr
	}
	return "", &exec.Error{Name: file, Err: exec.ErrNotFound}
}

// lookExtensions is what exec.Cmd.Start does on Windows for a name with a
// path separator: it adds a %PATHEXT% extension, relative to dir when the
// name is relative, and does not search %PATH%.
func (e Environment) lookExtensions(path, dir string) (string, error) {
	if invalidLookPath(path) {
		return "", &exec.Error{Name: path, Err: exec.ErrNotFound}
	}
	if filepath.Base(path) == path {
		path = "." + string(filepath.Separator) + path
	}
	exts := e.pathExt()
	if ext := filepath.Ext(path); ext != "" {
		for _, known := range exts {
			if strings.EqualFold(ext, known) {
				return path, nil // already resolved
			}
		}
	}
	if dir == "" || filepath.VolumeName(path) != "" || (len(path) > 1 && os.IsPathSeparator(path[0])) {
		return e.lookPathExts(path, exts)
	}
	joined := filepath.Join(dir, path)
	lp, err := e.lookPathExts(joined, exts)
	if err != nil {
		return "", err
	}
	return path + strings.TrimPrefix(lp, joined), nil
}
