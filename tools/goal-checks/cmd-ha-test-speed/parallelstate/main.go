// Command parallelstate is the static part of the cmd-ha-test-speed AC-3
// check (tools/goal-checks/cmd-ha-test-speed/stability.py). It reports Go
// tests that call t.Parallel() and can reach a change of process-wide state:
// the environment or the working directory. See usage below.
package main

import (
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
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
bodies of package-level functions of the same package that the code names by
identifier, whether it calls them or passes them as a value. A call of, or a
reference to, one of the functions above is a finding. Each finding lists one
shortest path from the parallel function to the call.

Not followed: method calls, interface calls and calls through function values
(variables, fields, parameters, results), functions of other packages, and
testing parameters copied into local variables. A local variable that
shadows a package-level function name is read as that function. Writes to
package-level variables are not looked for. The -race and -shuffle runs of
stability.py cover what this static reading misses.

Exit 0: nothing found. Exit 1: a finding; each is printed. Exit 2: DIR cannot
be read, holds no .go file, or a file cannot be parsed.
`

var (
	packageCalls = map[string]map[string]bool{
		"os":      {"Setenv": true, "Unsetenv": true, "Clearenv": true, "Chdir": true},
		"syscall": {"Setenv": true, "Unsetenv": true, "Clearenv": true},
	}
	testingCalls = map[string]bool{"Setenv": true, "Chdir": true}
	testingKinds = map[string]bool{"T": true, "B": true, "F": true, "TB": true}
)

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

// reference is a finding site (what is "os.Chdir") or a package-level
// function named in a body (what is the function's name).
type reference struct {
	what string
	pos  token.Pos
}

type analysis struct {
	fset  *token.FileSet
	funcs map[string][]*function
}

func (a *analysis) at(pos token.Pos) string {
	p := a.fset.Position(pos)
	return filepath.Base(p.Filename) + ":" + strconv.Itoa(p.Line)
}

// scan lists the forbidden calls and the package-level functions a block names.
func (a *analysis) scan(block *ast.BlockStmt, sc scope, f *file) (forbidden, named []reference) {
	skip := map[*ast.Ident]bool{}
	inspect(block, sc, f, func(n ast.Node, sc scope) {
		switch e := n.(type) {
		case *ast.SelectorExpr:
			skip[e.Sel] = true
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
			if len(a.funcs[e.Name]) > 0 {
				named = append(named, reference{e.Name, e.Pos()})
				return
			}
			for path, names := range packageCalls {
				if f.dots[path] && names[e.Name] {
					forbidden = append(forbidden, reference{path + "." + e.Name, e.Pos()})
				}
			}
		}
	})
	return forbidden, named
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

type step struct {
	name string
	path []string
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
	start := []string{root.label + " (" + a.at(root.node.Pos()) + ")"}
	forbidden, named := a.scan(root.block, root.scope, root.file)
	for _, call := range forbidden {
		add(when(call.pos, parallel), call, start)
	}
	for _, label := range []string{"before", "after"} {
		visited := map[string]bool{}
		var queue []step
		for _, ref := range named {
			if when(ref.pos, parallel) == label && !visited[ref.what] {
				visited[ref.what] = true
				queue = append(queue, step{ref.what, start})
			}
		}
		for len(queue) > 0 {
			current := queue[0]
			queue = queue[1:]
			for _, fn := range a.funcs[current.name] {
				if fn.decl.Body == nil {
					continue
				}
				path := append(append([]string(nil), current.path...), fn.name+" ("+a.at(fn.decl.Pos())+")")
				sc := scope{}.with(fn.decl, fn.decl.Recv, fn.decl.Type, fn.file)
				calls, next := a.scan(fn.decl.Body, sc, fn.file)
				for _, call := range calls {
					add(label, call, path)
				}
				for _, ref := range next {
					if !visited[ref.what] {
						visited[ref.what] = true
						queue = append(queue, step{ref.what, path})
					}
				}
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
func run(fset *token.FileSet, files []*file) ([]finding, int) {
	a := &analysis{fset: fset, funcs: map[string][]*function{}}
	for _, f := range files {
		for _, decl := range f.syntax.Decls {
			if fn, ok := decl.(*ast.FuncDecl); ok && fn.Recv == nil && fn.Name.Name != "_" {
				a.funcs[fn.Name.Name] = append(a.funcs[fn.Name.Name], &function{fn.Name.Name, f, fn})
			}
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
	return found, parallel
}

func main() {
	flag.Usage = func() { fmt.Fprint(os.Stderr, usage) }
	flag.Parse()
	if flag.NArg() != 1 {
		flag.Usage()
		os.Exit(2)
	}
	dir := flag.Arg(0)
	fset, files, err := load(dir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "parallelstate: not observed: %v\n", err)
		os.Exit(2)
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
		items, count := run(fset, byPackage[name])
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
		fmt.Printf("parallel test reaches %s %s:\n    %s\n", item.call.what, item.when, strings.Join(item.path, " -> "))
	}
	if len(found) > 0 {
		fmt.Printf("parallelstate: violation: %d finding(s) in %d parallel function(s) of %s\n", len(found), parallel, dir)
		os.Exit(1)
	}
	fmt.Printf("parallelstate: pass: %d parallel function(s) in %d file(s) of %s; none reaches a process-state change\n",
		parallel, len(files), dir)
}
