"""Finite Go test declaration and unconditional file-selection reader.

This is lexical selection evidence, not a Go parser, compiler, or execution.
Conditional files are unsupported: no host, toolchain, configured tags or Go
environment is guessed, and importing this module never invokes a Go tool.
"""
from pathlib import Path
import re


# Go's filename matcher includes historical targets, not only current ports.
# Source: go/build.goodOSArchFile and internal/syslist in the Go distribution.
GOOS = frozenset("aix android darwin dragonfly freebsd hurd illumos ios js linux nacl netbsd openbsd plan9 solaris wasip1 windows zos".split())
GOARCH = frozenset("386 amd64 amd64p32 arm armbe arm64 arm64be loong64 mips mipsle mips64 mips64le mips64p32 mips64p32le ppc ppc64 ppc64le riscv riscv64 s390 s390x sparc sparc64 wasm".split())
KEYWORDS = frozenset("break default func interface select case defer go map struct chan else goto package switch const fallthrough if range type continue for import return var".split())
CONTEXT = {"mode": "static-unconditional-files", "goos_goarch": "independent",
           "build_constraints": "none; conditional files are unsupported",
           "package_pattern": "./...",
           "package_scope": "checkout descendants excluding ignored directories, vendored descendants, nested modules and directory symlinks",
           "assurance": "lexical declaration and file selection; not compilation or execution"}


class SelectionError(ValueError):
    pass


def tokens(source):
    """Keep declaration tokens, separating comments, literals and delimiters."""
    result, comments = [], []
    position = 0

    def newline():
        if result and (result[-1][0] in ("name", "literal") or result[-1][1] in (")", "]", "}", "++", "--", "break", "continue", "fallthrough", "return")):
            result.append(("punctuation", ";", position))

    while position < len(source):
        char = source[position]
        if char.isspace():
            if char == "\n":
                newline()
            position += 1
            continue
        if source.startswith("//", position):
            end = source.find("\n", position)
            end = len(source) if end < 0 else end
            comments.append((position, source[position:end]))
            position = end
            continue
        if source.startswith("/*", position):
            end = source.find("*/", position + 2)
            if end < 0:
                raise SelectionError("unterminated Go block comment")
            if "\n" in source[position:end + 2]:
                newline()
            position = end + 2
            continue
        if char in ('"', "'", "`"):
            start, delimiter = position, char
            position += 1
            while position < len(source):
                if source[position] == delimiter:
                    position += 1
                    break
                if delimiter != "`" and source[position] == "\\":
                    position += 2
                elif delimiter != "`" and source[position] == "\n":
                    raise SelectionError("newline in interpreted Go literal")
                else:
                    position += 1
            else:
                raise SelectionError("unterminated Go literal")
            if position > len(source):
                raise SelectionError("unterminated Go literal escape")
            result.append(("literal", source[start:position], start))
            continue
        if char == "_" or char.isalpha():
            start = position
            position += 1
            while position < len(source) and (source[position] == "_" or source[position].isalnum()):
                position += 1
            word = source[start:position]
            result.append(("keyword" if word in KEYWORDS else "name", word, start))
            continue
        if char.isdigit():
            start = position
            position += 1
            while position < len(source) and (source[position].isalnum() or source[position] in "_."):
                position += 1
            result.append(("literal", source[start:position], start))
            continue
        operator = source[position:position + 2]
        if operator in ("++", "--", ":=", "==", "!=", "<=", ">=", "&&", "||", "<-", "<<", ">>"):
            result.append(("punctuation", operator, position))
            position += 2
        else:
            result.append(("punctuation", char, position))
            position += 1
    newline()
    return result, comments


def declarations(items):
    """Split package-level declarations at Go's explicit/inserted semicolons."""
    chunks, chunk, stack = [], [], []
    matching = {")": "(", "]": "[", "}": "{"}
    for item in items:
        value = item[1]
        if item[0] == "punctuation":
            if value in ("(", "[", "{"):
                stack.append(value)
            elif value in matching:
                if not stack or stack.pop() != matching[value]:
                    raise SelectionError("unbalanced Go declaration delimiters")
            elif value == ";" and not stack:
                if chunk:
                    chunks.append(chunk)
                    chunk = []
                continue
        chunk.append(item)
    if stack:
        raise SelectionError("unbalanced Go declaration delimiters")
    if chunk:
        chunks.append(chunk)
    return chunks


def testing_import(declaration):
    entries = declaration[1:]
    if entries and entries[0][1] == "(":
        if entries[-1][1] != ")":
            raise SelectionError("unsupported Go import declaration")
        entries = entries[1:-1]
    aliases, entry = [], []
    for item in [*entries, ("punctuation", ";", 0)]:
        if item[1] == ";":
            if not entry:
                continue
            if len(entry) not in (1, 2) or entry[-1][0] != "literal":
                raise SelectionError("unsupported Go import declaration")
            if entry[-1][1] in ('"testing"', '`testing`'):
                alias = "testing" if len(entry) == 1 else entry[0][1]
                if alias == "_" or (len(entry) == 2 and alias != "." and entry[0][0] != "name"):
                    raise SelectionError("testing import does not bind a supported type name")
                aliases.append(alias)
            entry = []
        else:
            entry.append(item)
    return aliases


def unconditional_file(path, comments, package_start):
    name = Path(path).name
    if name.startswith(("_", ".")) or not name.endswith("_test.go"):
        raise SelectionError("Go test filename is excluded from package selection")
    # Go removes everything before the first underscore before interpreting
    # suffixes; linux_test.go has no GOOS suffix, sample_linux_test.go does.
    stem = name.split(".", 1)[0]
    suffixes = stem[stem.find("_"):].split("_") if "_" in stem else []
    if suffixes and suffixes[-1] == "test":
        suffixes.pop()
    if suffixes and suffixes[-1] in GOOS | GOARCH:
        raise SelectionError("unsupported conditional Go filename: GOOS/GOARCH selection is not established")
    for position, comment in comments:
        if position < package_start and re.match(r"//(?:go:build(?:\s|$)|\s*\+build(?:\s|$))", comment):
            raise SelectionError("unsupported Go build constraint: conditional file selection is not established")


def package_layout(path, root):
    """Match the supported checkout-root ./... walk without resolving aliases.

    Only descendants participate: a symlink or hidden-name ancestor outside the
    checkout does not change package selection. Go includes a package named
    vendor itself, but ./... never includes its descendants.
    """
    root, path = Path(root).absolute(), Path(path).absolute()
    try:
        relative = path.relative_to(root)
    except ValueError as error:
        raise SelectionError("Go source is outside the common package-selection root") from error
    if ".." in relative.parts:
        raise SelectionError("unsupported parent traversal in Go package selection")
    directories = relative.parts[:-1]
    directory = root
    for index, part in enumerate(directories):
        directory /= part
        if directory.is_symlink():
            raise SelectionError("Go package directory symlink is not traversed by ./...")
        if part.startswith((".", "_")) or part == "testdata":
            raise SelectionError("Go package directory is excluded from ./... selection")
        if part == "vendor" and index < len(directories) - 1:
            raise SelectionError("vendored Go package descendants are excluded from ./... selection")
        module = directory / "go.mod"
        if module.exists() and not module.is_dir():
            raise SelectionError("nested Go module is excluded from checkout-root ./... selection")


def validate_test(path, selector, *, root):
    """Require one supported real test declaration in an unconditional file.

    The caller supplies the lexical checkout root and remains responsible for
    tracked source and common-suite wiring.
    Raises SelectionError for an absent definition or unsupported file context.
    """
    if not isinstance(selector, str) or not re.fullmatch(r"Test(?:[A-Z0-9_][A-Za-z0-9_]*)?", selector):
        raise SelectionError("unsupported or non-test Go selector name")
    package_layout(path, root)
    items, comments = tokens(Path(path).read_text(encoding="utf-8"))
    chunks = declarations(items)
    if not chunks or [part[1] for part in chunks[0][:1]] != ["package"] or len(chunks[0]) != 2 or chunks[0][1][0] != "name":
        raise SelectionError("Go package declaration is missing or unsupported")
    unconditional_file(path, comments, chunks[0][0][2])
    aliases, candidates = [], []
    for declaration in chunks[1:]:
        kind = declaration[0][1]
        if kind == "import":
            aliases.extend(testing_import(declaration))
        elif kind == "func":
            if len(declaration) > 1 and declaration[1][1] == selector:
                candidates.append(declaration)
        elif kind not in ("const", "var", "type"):
            raise SelectionError("unsupported non-declaration text at Go package scope")
    if len(candidates) != 1 or len(aliases) != 1:
        raise SelectionError("Go test definition or testing import is missing or repeated")
    declaration = candidates[0]
    values = [item[1] for item in declaration]
    if values[2:3] != ["("]:
        raise SelectionError("unsupported Go test signature: type parameters are not tests")
    try:
        closing = values.index(")", 3)
    except ValueError as error:
        raise SelectionError("unsupported Go test parameter list") from error
    arguments = values[3:closing]
    if arguments[-1:] == [","]:
        arguments.pop()
    expected = ["*", "T"] if aliases[0] == "." else ["*", aliases[0], ".", "T"]
    if len(arguments) == len(expected) + 1 and declaration[3][0] == "name":
        arguments = arguments[1:]
    if arguments != expected:
        raise SelectionError("unsupported Go test signature: one *testing.T parameter is required")
    remainder = values[closing + 1:]
    if remainder[:2] == ["(", ")"]:
        remainder = remainder[2:]
    if not remainder or remainder[0] != "{" or remainder[-1] != "}":
        raise SelectionError("unsupported Go test signature: a body and no return values are required")
    depth = 0
    for index, value in enumerate(remainder):
        if value == "{":
            depth += 1
        elif value == "}":
            depth -= 1
        if depth == 0 and index != len(remainder) - 1:
            raise SelectionError("unexpected text after the Go test body")
    return dict(CONTEXT)
