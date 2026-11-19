// Command parallelstate is the static part of the cmd-ha-test-speed AC-3
// check (tools/goal-checks/cmd-ha-test-speed/stability.py). It reports Go
// tests that call t.Parallel() and can reach a change of process-wide state:
// the environment, the working directory, or a package-level variable of the
// package analysed. It follows calls of the functions and methods declared in
// the same package; calls through function values and calls of interface
// methods are not followed. Variables of other packages are not in scope:
// every import is read as an empty package. Taking a variable's address with
// & counts as a write; the address a pointer method's call takes implicitly
// (v.Lock(), v.Write(p)) does not, and writes through a copy of the
// variable's value (a map, slice or pointer held elsewhere) are not seen.
// Writes in the shared-build helper are allowed by sharedBuildWriters. See
// usage below.
package main

import (
	"errors"
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

const usage = `usage: parallelstate DIR

Parses every .go file of the package in DIR (test and non-test files, under
every build constraint; names starting with "_" or "." are skipped, as Go
does) with go/parser, and reports each function body (declaration or
function literal) that calls t.Parallel() on its own *testing.T parameter
and can reach a call of:

  os.Setenv  os.Unsetenv  os.Clearenv  os.Chdir
  syscall.Setenv  syscall.Unsetenv  syscall.Clearenv
  t.Setenv  t.Chdir   (on a *testing.T, *testing.B, *testing.F or testing.TB parameter)

Reachable code is the whole body of the parallel function, function literals
in it included: the code after the t.Parallel() call ("after") and the code
before it ("before"; a test that changes this state and then calls
t.Parallel() is reported too). From there it follows, transitively, the
bodies of the functions and methods of the same package that the code names,
whether it calls them or passes them as a value:

  package-level functions  by identifier:  f(...)  f
  methods                  by selector:    x.m(...)  x.f.m(...)  x.m  T.m  (*T).m

A method's receiver type comes from go/types, run on each package alone with
every import read as an empty package: it is the declared type of the
variable, field, parameter or result (or of any other expression) the method
is selected on, and a method promoted from an embedded field is that field
type's method. Every declaration of that method (one per build constraint)
is followed. When the receiver's type cannot be known (it comes from another
package, or the code does not type-check), every method of the package with
that name is followed, and the path marks the step "by name". A method
selected on a testing parameter or on an imported package is not followed.
A call of, or a reference to, one of the calls above is a finding.

A write of a package-level variable of the package analysed, as go/types
resolves the name (not a local variable, parameter or field that has the
name), in the same reachable code is a finding too, "write of v":

  v = x  v += x  v++  v--  for v = range x    the variable itself
  v.f = x  v[i] = x  (*v).f = x  v.f[i]++      a field of it, an index into
                                               it, what it points to, mixed
  delete(v, k)  clear(v)  copy(v[i:], x)       builtins that change it
  &v  &v.f  &v[i]                              its address, wherever it goes

Writes in the shared-build helper of cmd/ha are allowed, as the allowlist
sharedBuildWriters lists them: sharedBinary and the functions only it
calls, in shared_build_test.go (AC-4 checks that helper separately). They
are allowed only on a path that enters the list through sharedBinary and
stays in it: a listed function reached another way, and a function outside
the list that a listed one calls, are checked as any other.

Each finding lists one shortest path from the parallel function to the call
or the write.

Not followed: calls of interface methods, calls through function values
(variables, fields, parameters, results), functions and methods of other
packages, and testing values copied into local variables or held in struct
fields. A local variable that shadows a package-level function name is read
as that function. Not looked for: variables of other packages (every import
is read as an empty package, so other.V = x is not seen), writes through a
copy of a variable's value (a map, slice or pointer held in a local
variable, field, parameter or result), the address that calling a pointer
method on a variable takes implicitly (v.Lock(), v.Write(p)), and writes by
reflection or package unsafe. The -race runs of stability.py do not report
what this static reading misses: the environment is guarded by a lock, the
working directory is not Go memory, and a package-level value written under
a mutex does not race.

Exit 0: nothing found. Exit 1: a finding; each is printed. Exit 2: DIR cannot
be read, holds no .go file, a file cannot be parsed, or go/types stops.
`

var (
	packageCalls = map[string]map[string]bool{
		"os":      {"Setenv": true, "Unsetenv": true, "Clearenv": true, "Chdir": true},
		"syscall": {"Setenv": true, "Unsetenv": true, "Clearenv": true},
	}
	testingCalls = map[string]bool{"Setenv": true, "Chdir": true}
	testingKinds = map[string]bool{"T": true, "B": true, "F": true, "TB": true}
	// changingBuiltins change the value of their first argument.
	changingBuiltins = map[string]bool{"delete": true, "clear": true, "copy": true}
)

// sharedBuildWriters is the allowlist of package-level writes: the file of
// cmd/ha, then sharedBinary and the functions only it calls. sharedBinary
// builds each binary once per test process for every test that asks, so it
// keeps the builds in the package-level sharedBuilds under its lock; AC-4
// checks that helper and its binaries separately. A write in one of these
// functions is allowed only on a path that enters them through sharedBinary
// (the first name) and stays in them.
const sharedBuildWriters = "shared_build_test.go: sharedBinary (*sharedBuild).make sharedBuildFolder sharedModuleRoot"

// inSharedBuild reports whether fn, named from code that is (from) or is not
// in the shared-build helper, is in it: fn is the first function of
// sharedBuildWriters, or another one of them named from inside the helper.
func inSharedBuild(fn *function, from bool) bool {
	file, names, _ := strings.Cut(sharedBuildWriters, ": ")
	if fn.file.name != file {
		return false
	}
	for i, name := range strings.Fields(names) {
		if name == fn.name {
			return i == 0 || from
		}
	}
	return false
}

type file struct {
	name    string
	syntax  *ast.File
	imports map[string]string // local name -> import path
	dots    map[string]bool   // dot-imported paths
}

// param is a function parameter (or named result) in scope. kind is the
// testing type ("T", "B", "F", "TB") or "" for any other type.
type param struct {
	owner ast.Node
	kind  string
}

type scope map[string]param

func (s scope) with(owner ast.Node, recv *ast.FieldList, typ *ast.FuncType, f *file) scope {
	next := make(scope, len(s)+4)
	for name, p := range s {
		next[name] = p
	}
	for _, fields := range []*ast.FieldList{recv, typ.Params, typ.Results} {
		if fields == nil {
			continue
		}
		for _, field := range fields.List {
			kind := testingKind(field.Type, f)
			for _, name := range field.Names {
				next[name.Name] = param{owner, kind}
			}
		}
	}
	return next
}

// testingKind returns "T", "B" or "F" for *testing.T, *testing.B or
// *testing.F, "TB" for testing.TB, and "" otherwise.
func testingKind(expr ast.Expr, f *file) string {
	star, pointer := expr.(*ast.StarExpr)
	if pointer {
		expr = star.X
	}
	var name string
	switch e := expr.(type) {
	case *ast.SelectorExpr:
		x, ok := e.X.(*ast.Ident)
		if !ok || f.imports[x.Name] != "testing" {
			return ""
		}
		name = e.Sel.Name
	case *ast.Ident:
		if !f.dots["testing"] {
			return ""
		}
		name = e.Name
	default:
		return ""
	}
	if !testingKinds[name] || (name == "TB") == pointer {
		return ""
	}
	return name
}

// inspect walks root and calls visit with the parameter scope in force at
// each node. A function literal is visited with its own parameters added.
func inspect(root ast.Node, sc scope, f *file, visit func(ast.Node, scope)) {
	ast.Inspect(root, func(n ast.Node) bool {
		if n == nil {
			return false
		}
		if lit, ok := n.(*ast.FuncLit); ok && n != root {
			inner := sc.with(lit, nil, lit.Type, f)
			visit(lit, inner)
			inspect(lit.Body, inner, f, visit)
			return false
		}
		visit(n, sc)
		return true
	})
}

// function is a package-level function or a method. name is "f" for a
// function and "T.m" or "(*T).m" for a method.
type function struct {
	name string
	file *file
	decl *ast.FuncDecl
}

type body struct {
	label string // "TestX" or "func literal in TestX"
	node  ast.Node
	block *ast.BlockStmt
	scope scope
	file  *file
}

// reference is a finding site: what is "os.Chdir", "t.Setenv" and so on.
type reference struct {
	what string
	pos  token.Pos
}

// link is a function or method of the package that a body names: every
// declaration it can be. byName is true for a method whose receiver's type
// is not known, so to holds every method of the package with that name.
type link struct {
	pos    token.Pos
	to     []*function
	byName bool
}

type analysis struct {
	fset    *token.FileSet
	pkg     *types.Package
	funcs   map[string][]*function  // package-level functions by name
	methods map[string][]*function  // methods by "T.m", T the receiver's base type
	byName  map[string][]*function  // methods by method name
	declAt  map[token.Pos]*function // methods by the position of their name
	info    *types.Info
}

func (a *analysis) at(pos token.Pos) string {
	p := a.fset.Position(pos)
	return filepath.Base(p.Filename) + ":" + strconv.Itoa(p.Line)
}

// scan lists the forbidden calls, the writes of package-level variables and
// the functions and methods of the package that a block names.
func (a *analysis) scan(block *ast.BlockStmt, sc scope, f *file) (forbidden, writes []reference, named []link) {
	skip := map[*ast.Ident]bool{}
	write := func(target ast.Expr) {
		if v := a.packageVar(target); v != nil {
			writes = append(writes, reference{"write of " + v.Name(), target.Pos()})
		}
	}
	inspect(block, sc, f, func(n ast.Node, sc scope) {
		switch e := n.(type) {
		case *ast.AssignStmt:
			if e.Tok != token.DEFINE {
				for _, lhs := range e.Lhs {
					write(lhs)
				}
			}
		case *ast.IncDecStmt:
			write(e.X)
		case *ast.RangeStmt:
			if e.Tok == token.ASSIGN {
				write(e.Key)
				write(e.Value)
			}
		case *ast.UnaryExpr:
			if e.Op == token.AND {
				write(e.X)
			}
		case *ast.CallExpr:
			if id, ok := ast.Unparen(e.Fun).(*ast.Ident); ok && len(e.Args) > 0 {
				if b, isBuiltin := a.info.Uses[id].(*types.Builtin); isBuiltin && changingBuiltins[b.Name()] {
					write(e.Args[0])
				}
			}
		case *ast.SelectorExpr:
			skip[e.Sel] = true
			if l, ok := a.method(e, sc); ok {
				named = append(named, l)
			}
			x, ok := e.X.(*ast.Ident)
			if !ok {
				return
			}
			if p, isParam := sc[x.Name]; isParam {
				if p.kind != "" && testingCalls[e.Sel.Name] {
					forbidden = append(forbidden, reference{x.Name + "." + e.Sel.Name, e.Pos()})
				}
				return
			}
			if path := f.imports[x.Name]; packageCalls[path][e.Sel.Name] {
				forbidden = append(forbidden, reference{path + "." + e.Sel.Name, e.Pos()})
			}
		case *ast.KeyValueExpr:
			if key, ok := e.Key.(*ast.Ident); ok {
				skip[key] = true // a struct field name
			}
		case *ast.Ident:
			if skip[e] {
				return
			}
			if _, isParam := sc[e.Name]; isParam {
				return
			}
			if fns := a.funcs[e.Name]; len(fns) > 0 {
				named = append(named, link{pos: e.Pos(), to: fns})
				return
			}
			for path, names := range packageCalls {
				if f.dots[path] && names[e.Name] {
					forbidden = append(forbidden, reference{path + "." + e.Name, e.Pos()})
				}
			}
		}
	})
	return forbidden, writes, named
}

// packageVar returns the package-level variable of the package analysed
// whose value a write of target changes: target itself, or the variable that
// target selects a field of, indexes, slices or dereferences, in any mix. It
// returns nil for anything else, a variable of another package included.
func (a *analysis) packageVar(target ast.Expr) *types.Var {
	for {
		switch e := target.(type) {
		case *ast.ParenExpr:
			target = e.X
		case *ast.SelectorExpr:
			target = e.X // a field; a package name below it is no variable
		case *ast.IndexExpr:
			target = e.X
		case *ast.SliceExpr:
			target = e.X
		case *ast.StarExpr:
			target = e.X
		case *ast.Ident:
			v, ok := a.info.Uses[e].(*types.Var)
			if !ok || v.IsField() || a.pkg == nil || v.Parent() != a.pkg.Scope() {
				return nil
			}
			return v
		default:
			return nil
		}
	}
}

// method resolves the selector e to the methods of the package it can name.
// ok is false when e selects no method of the package: a field, a method of
// an interface or of another package, or a member of an imported package.
func (a *analysis) method(e *ast.SelectorExpr, sc scope) (l link, ok bool) {
	all := a.byName[e.Sel.Name]
	if len(all) == 0 {
		return link{}, false
	}
	if sel, found := a.info.Selections[e]; found {
		fn, isFunc := sel.Obj().(*types.Func)
		if !isFunc || sel.Kind() == types.FieldVal {
			return link{}, false
		}
		decl := a.declAt[fn.Origin().Pos()]
		if decl == nil {
			return link{}, false // a method of an interface
		}
		return link{pos: e.Pos(), to: a.methods[methodKey(decl.decl)]}, true
	}
	if x, isIdent := e.X.(*ast.Ident); isIdent {
		if _, isPackage := a.info.Uses[x].(*types.PkgName); isPackage {
			return link{}, false
		}
		if sc[x.Name].kind != "" {
			return link{}, false // a method of package testing
		}
	}
	return link{pos: e.Pos(), to: all, byName: true}, true
}

// receiverBase returns the receiver's base type name of a method, without
// "*" and type parameters, and whether the receiver is a pointer.
func receiverBase(decl *ast.FuncDecl) (name string, pointer bool) {
	if decl.Recv == nil || len(decl.Recv.List) == 0 {
		return "?", false
	}
	expr := decl.Recv.List[0].Type
	for {
		switch e := expr.(type) {
		case *ast.ParenExpr:
			expr = e.X
			continue
		case *ast.StarExpr:
			pointer = true
			expr = e.X
			continue
		case *ast.IndexExpr:
			expr = e.X
			continue
		case *ast.IndexListExpr:
			expr = e.X
			continue
		case *ast.Ident:
			return e.Name, pointer
		}
		return "?", pointer
	}
}

func methodKey(decl *ast.FuncDecl) string {
	base, _ := receiverBase(decl)
	return base + "." + decl.Name.Name
}

func methodLabel(decl *ast.FuncDecl) string {
	base, pointer := receiverBase(decl)
	if pointer {
		return "(*" + base + ")." + decl.Name.Name
	}
	return base + "." + decl.Name.Name
}

// emptyImports reads every import as an empty package, so go/types types
// the package alone: what comes from another package has no type.
type emptyImports map[string]*types.Package

func (m emptyImports) Import(path string) (*types.Package, error) {
	if p := m[path]; p != nil {
		return p, nil
	}
	p := types.NewPackage(path, packageName(path))
	p.MarkComplete()
	m[path] = p
	return p, nil
}

// packageName guesses an imported package's name from its path, as
// goimports does: ".../go-toml/v2" is "toml", "gopkg.in/yaml.v3" is "yaml".
func packageName(path string) string {
	parts := strings.Split(path, "/")
	name := parts[len(parts)-1]
	if len(parts) > 1 && len(name) > 1 && name[0] == 'v' && strings.Trim(name[1:], "0123456789") == "" {
		name = parts[len(parts)-2]
	}
	if i := strings.Index(name, ".v"); i > 0 {
		name = name[:i]
	}
	name = strings.TrimPrefix(name, "go-")
	return strings.ReplaceAll(name, "-", "_")
}

// typeInfo runs go/types on one package. Its type errors are expected (the
// imports are empty) and only leave types unknown; a panic is an error.
func typeInfo(name string, fset *token.FileSet, files []*file) (pkg *types.Package, info *types.Info, err error) {
	defer func() {
		if r := recover(); r != nil {
			pkg, info, err = nil, nil, fmt.Errorf("go/types stopped on package %s: %v", name, r)
		}
	}()
	syntax := make([]*ast.File, len(files))
	for i, f := range files {
		syntax[i] = f.syntax
	}
	info = &types.Info{
		Selections: map[*ast.SelectorExpr]*types.Selection{},
		Uses:       map[*ast.Ident]types.Object{},
	}
	conf := types.Config{Importer: emptyImports{}, Error: func(error) {}, FakeImportC: true}
	pkg, _ = conf.Check(name, fset, syntax, info) // type errors only leave types unknown
	if pkg == nil {
		return nil, nil, fmt.Errorf("go/types returned no package for %s", name)
	}
	return pkg, info, nil
}

// parallelCall returns the position of the first t.Parallel() call on the
// body's own *testing.T parameter, or token.NoPos.
func parallelCall(b *body) token.Pos {
	first := token.NoPos
	inspect(b.block, b.scope, b.file, func(n ast.Node, sc scope) {
		call, ok := n.(*ast.CallExpr)
		if !ok || len(call.Args) != 0 {
			return
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != "Parallel" {
			return
		}
		x, ok := sel.X.(*ast.Ident)
		if !ok {
			return
		}
		if p := sc[x.Name]; p.owner == b.node && p.kind == "T" && (first == token.NoPos || call.Pos() < first) {
			first = call.Pos()
		}
	})
	return first
}

type finding struct {
	root  *body
	when  string
	call  reference
	path  []string
	order [3]int
}

// step is a function to scan and the path to it. shared is true when the
// path entered the shared-build helper through sharedBinary and stayed in
// it: the writes of package-level variables in fn are then allowed.
type step struct {
	fn     *function
	path   []string
	shared bool
}

type visit struct {
	fn     *function
	shared bool
}

func (a *analysis) findings(root *body, parallel token.Pos) []finding {
	var found []finding
	seen := map[string]bool{}
	add := func(when string, call reference, path []string) {
		key := when + "\x00" + a.at(call.pos) + "\x00" + call.what
		if seen[key] {
			return
		}
		seen[key] = true
		found = append(found, finding{root: root, when: when, call: call, path: append([]string(nil), path...)})
	}
	start := step{path: []string{root.label + " (" + a.at(root.node.Pos()) + ")"}}
	forbidden, writes, named := a.scan(root.block, root.scope, root.file)
	for _, call := range append(forbidden, writes...) {
		add(when(call.pos, parallel), call, start.path)
	}
	for _, label := range []string{"before", "after"} {
		visited := map[visit]bool{}
		var queue []step
		follow := func(l link, from step) {
			for _, fn := range l.to {
				shared := inSharedBuild(fn, from.shared)
				if visited[visit{fn, shared}] {
					continue
				}
				visited[visit{fn, shared}] = true
				name := fn.name
				if l.byName {
					name += " by name"
				}
				queue = append(queue, step{fn, append(append([]string(nil), from.path...), name+" ("+a.at(fn.decl.Pos())+")"), shared})
			}
		}
		for _, l := range named {
			if when(l.pos, parallel) == label {
				follow(l, start)
			}
		}
		for len(queue) > 0 {
			current := queue[0]
			queue = queue[1:]
			fn := current.fn
			if fn.decl.Body == nil {
				continue
			}
			sc := scope{}.with(fn.decl, fn.decl.Recv, fn.decl.Type, fn.file)
			calls, writes, next := a.scan(fn.decl.Body, sc, fn.file)
			if !current.shared {
				calls = append(calls, writes...)
			}
			for _, call := range calls {
				add(label, call, current.path)
			}
			for _, l := range next {
				follow(l, current)
			}
		}
	}
	return found
}

func when(pos, parallel token.Pos) string {
	if pos < parallel {
		return "before"
	}
	return "after"
}

func load(dir string) (*token.FileSet, []*file, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, nil, err
	}
	fset := token.NewFileSet()
	var files []*file
	var problems []string
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasPrefix(name, "_") || strings.HasPrefix(name, ".") {
			continue
		}
		syntax, err := parser.ParseFile(fset, filepath.Join(dir, name), nil, parser.SkipObjectResolution)
		if err != nil {
			problems = append(problems, err.Error())
			continue
		}
		f := &file{name: name, syntax: syntax, imports: map[string]string{}, dots: map[string]bool{}}
		for _, spec := range syntax.Imports {
			path, err := strconv.Unquote(spec.Path.Value)
			if err != nil {
				problems = append(problems, fmt.Sprintf("%s: import %s: %v", name, spec.Path.Value, err))
				continue
			}
			switch {
			case spec.Name == nil:
				f.imports[path[strings.LastIndex(path, "/")+1:]] = path
			case spec.Name.Name == ".":
				f.dots[path] = true
			case spec.Name.Name != "_":
				f.imports[spec.Name.Name] = path
			}
		}
		files = append(files, f)
	}
	if len(problems) > 0 {
		return nil, nil, fmt.Errorf("cannot parse the package:\n%s", strings.Join(problems, "\n"))
	}
	if len(files) == 0 {
		return nil, nil, fmt.Errorf("%s holds no .go file", dir)
	}
	return fset, files, nil
}

// run analyses one package name (main or main_test) of the directory.
func run(name string, fset *token.FileSet, files []*file) ([]finding, int, error) {
	pkg, info, err := typeInfo(name, fset, files)
	if err != nil {
		return nil, 0, err
	}
	a := &analysis{
		fset:    fset,
		pkg:     pkg,
		funcs:   map[string][]*function{},
		methods: map[string][]*function{},
		byName:  map[string][]*function{},
		declAt:  map[token.Pos]*function{},
		info:    info,
	}
	for _, f := range files {
		for _, decl := range f.syntax.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Name.Name == "_" {
				continue
			}
			if fn.Recv == nil {
				a.funcs[fn.Name.Name] = append(a.funcs[fn.Name.Name], &function{fn.Name.Name, f, fn})
				continue
			}
			m := &function{methodLabel(fn), f, fn}
			a.methods[methodKey(fn)] = append(a.methods[methodKey(fn)], m)
			a.byName[fn.Name.Name] = append(a.byName[fn.Name.Name], m)
			a.declAt[fn.Name.Pos()] = m
		}
	}
	var bodies []*body
	for _, f := range files {
		for _, decl := range f.syntax.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			sc := scope{}.with(fn, fn.Recv, fn.Type, f)
			bodies = append(bodies, &body{fn.Name.Name, fn, fn.Body, sc, f})
			inspect(fn.Body, sc, f, func(n ast.Node, sc scope) {
				if lit, ok := n.(*ast.FuncLit); ok {
					bodies = append(bodies, &body{"func literal in " + fn.Name.Name, lit, lit.Body, sc, f})
				}
			})
		}
	}
	var found []finding
	parallel := 0
	for _, b := range bodies {
		call := parallelCall(b)
		if call == token.NoPos {
			continue
		}
		parallel++
		for _, item := range a.findings(b, call) {
			item.path = append(item.path, item.call.what+" ("+a.at(item.call.pos)+")")
			item.order = [3]int{int(b.node.Pos()), map[string]int{"before": 0, "after": 1}[item.when], int(item.call.pos)}
			item.when += " t.Parallel() at " + a.at(call)
			found = append(found, item)
		}
	}
	return found, parallel, nil
}

func main() {
	os.Exit(check(os.Args[1:], os.Stdout, os.Stderr))
}

// check runs the command with args and returns its exit code.
func check(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("parallelstate", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.Usage = func() { fmt.Fprint(stderr, usage) }
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if flags.NArg() != 1 {
		flags.Usage()
		return 2
	}
	dir := flags.Arg(0)
	fset, files, err := load(dir)
	if err != nil {
		fmt.Fprintf(stderr, "parallelstate: not observed: %v\n", err)
		return 2
	}
	byPackage := map[string][]*file{}
	var names []string
	for _, f := range files {
		if _, ok := byPackage[f.syntax.Name.Name]; !ok {
			names = append(names, f.syntax.Name.Name)
		}
		byPackage[f.syntax.Name.Name] = append(byPackage[f.syntax.Name.Name], f)
	}
	sort.Strings(names)
	var found []finding
	parallel := 0
	for _, name := range names {
		items, count, err := run(name, fset, byPackage[name])
		if err != nil {
			fmt.Fprintf(stderr, "parallelstate: not observed: %v\n", err)
			return 2
		}
		found = append(found, items...)
		parallel += count
	}
	sort.SliceStable(found, func(i, j int) bool {
		for k := range found[i].order {
			if found[i].order[k] != found[j].order[k] {
				return found[i].order[k] < found[j].order[k]
			}
		}
		return false
	})
	for _, item := range found {
		fmt.Fprintf(stdout, "parallel test reaches %s %s:\n    %s\n", item.call.what, item.when, strings.Join(item.path, " -> "))
	}
	if len(found) > 0 {
		fmt.Fprintf(stdout, "parallelstate: violation: %d finding(s) in %d parallel function(s) of %s\n", len(found), parallel, dir)
		return 1
	}
	fmt.Fprintf(stdout, "parallelstate: pass: %d parallel function(s) in %d file(s) of %s; none reaches a process-state change\n",
		parallel, len(files), dir)
	return 0
}
