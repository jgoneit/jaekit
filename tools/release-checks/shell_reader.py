"""Bounded shell syntax reader. This module never executes observed text."""
import re
from pathlib import PurePosixPath


class ReadError(Exception):
    pass


class Redirect:
    def __init__(self, word):
        self.word = word


def literal(word):
    if not isinstance(word, list) or any(p[0] != "text" for p in word):
        return None
    return "".join(p[1] for p in word)


def value(word, scope):
    result = []
    for part in word:
        if part[0] == "text":
            result.append(part[1])
        elif part[0] == "var" and scope.get(part[1]) is not None:
            result.append(scope[part[1]])
        else:
            return None
    return "".join(result)


def assignment(word):
    if not isinstance(word, list) or not word or word[0][0] != "text" or word[0][2]:
        return None
    match = re.match(r"([A-Za-z_][A-Za-z0-9_]*)=", word[0][1])
    if match:
        return match[1], [("text", word[0][1][match.end():], False)] + word[1:]
    return None


class Lexer:
    def __init__(self, text, level=0):
        if level > 32:
            raise ReadError("observed shell text is nested too deeply")
        self.text, self.i, self.level = text, 0, level

    def at(self, n=0):
        return self.text[self.i + n:self.i + n + 1]

    def expansion(self, word, quoted=False):
        if self.text.startswith("$(", self.i) and not self.text.startswith("$((", self.i):
            self.i += 2
            word.append(("sub", self.tokens(")")))
        elif self.at() == "`":
            self.i += 1
            body = []
            while self.at() and self.at() != "`":
                if self.at() == "\\" and self.at(1) in ("`", "\\", "$"):
                    self.i += 1
                body.append(self.at())
                self.i += 1
            if not self.at():
                raise ReadError("unterminated shell backtick substitution")
            self.i += 1
            word.append(("sub", Lexer("".join(body), self.level + 1).tokens()))
        else:
            match = re.match(r"\$(?:\{([A-Za-z_][A-Za-z0-9_]*)\}|([A-Za-z_][A-Za-z0-9_]*))", self.text[self.i:])
            if match:
                word.append(("var", match[1] or match[2], quoted))
                self.i += match.end()
            elif self.at(1) in ("{", "("):
                opening = self.at(1)
                closing = "}" if opening == "{" else ")"
                self.i += 2
                depth = 1
                while self.at() and depth:
                    depth += (self.at() == opening) - (self.at() == closing)
                    self.i += 1
                if depth:
                    raise ReadError("unterminated shell expansion")
                word.append(("dynamic",))
            else:
                word.append(("text", "$", False))
                self.i += 1

    def word(self):
        out = []
        def add(text, quoted=False):
            if out and out[-1][0] == "text" and out[-1][2] == quoted:
                out[-1] = ("text", out[-1][1] + text, quoted)
            else:
                out.append(("text", text, quoted))
        while self.at() and self.at() not in " \t\n;&|(){}<>":
            char = self.at()
            if char == "\\":
                self.i += 1
                if not self.at():
                    raise ReadError("unterminated shell escape")
                if self.at() != "\n":
                    add(self.at(), True)
                self.i += bool(self.at())
            elif char == "'":
                self.i += 1
                end = self.text.find("'", self.i)
                if end < 0:
                    raise ReadError("unterminated shell quote")
                add(self.text[self.i:end], True)
                self.i = end + 1
            elif char == '"':
                self.i += 1
                add("", True)
                while self.at() and self.at() != '"':
                    if self.at() in ("$", "`"):
                        self.expansion(out, quoted=True)
                    elif self.at() == "\\" and self.at(1) in ('$', '`', '"', '\\', '\n'):
                        self.i += 1
                        if self.at() != "\n":
                            add(self.at(), True)
                        self.i += 1
                    else:
                        add(self.at(), True)
                        self.i += 1
                if not self.at():
                    raise ReadError("unterminated shell quote")
                self.i += 1
            elif char in ("$", "`"):
                self.expansion(out)
            else:
                add(char)
                self.i += 1
        return out

    def tokens(self, closing=None):
        result, pending = [], []
        while self.at():
            char = self.at()
            if char == closing:
                self.i += 1
                return result
            if char in " \t" or (char == "\\" and self.at(1) == "\n"):
                self.i += 2 if char == "\\" else 1
            elif char == "#":
                end = self.text.find("\n", self.i)
                self.i = len(self.text) if end < 0 else end
            elif char == "\n":
                self.i += 1
                for delimiter, quoted, strip in pending:
                    body = []
                    while self.at():
                        end = self.text.find("\n", self.i)
                        line = self.text[self.i:] if end < 0 else self.text[self.i:end]
                        self.i = len(self.text) if end < 0 else end + 1
                        if (line.lstrip("\t") if strip else line) == delimiter:
                            break
                        body.append(line)
                    if not quoted:
                        nested = Lexer("\n".join(body), self.level + 1)
                        parts = []
                        while nested.at():
                            if nested.at() == "\\" and nested.at(1) in ("$", "`", "\\", "\n"):
                                nested.i += 2
                            elif nested.at() in ("$", "`"):
                                nested.expansion(parts)
                            else:
                                nested.i += 1
                        result.append(Redirect(parts))
                pending = []
                result.append(";")
            elif char in "<>" and self.at(1) == "(":
                self.i += 2
                result.append(Redirect([("sub", self.tokens(")"))]))
            elif char in "<>" or (char == "&" and self.at(1) == ">"):
                match = re.match(r"&>>|&>|<<<|<<-|<<|>>|<>|<&|>&|>\||<|>", self.text[self.i:])
                operator = match[0]
                self.i += len(operator)
                while self.at() in (" ", "\t"):
                    self.i += 1
                if result and isinstance(result[-1], list) and (literal(result[-1]) or "").isdigit():
                    result.pop()
                target = self.word()
                if operator in ("<<", "<<-"):
                    delimiter = "".join(p[1] if p[0] == "text" else "$" + p[1] if p[0] == "var" else "?" for p in target)
                    quoted = any(p[0] == "text" and p[2] for p in target)
                    pending.append((delimiter, quoted, operator == "<<-"))
                else:
                    result.append(Redirect(target))
            elif char in ";&|":
                operator = char * 2 if self.at(1) == char else char
                self.i += len(operator)
                result.append(operator)
            elif char in "({":
                self.i += 1
                result.append(("group", char, self.tokens(")" if char == "(" else "}")))
            elif char in ")}":
                raise ReadError("unbalanced shell group")
            else:
                result.append(self.word())
        if closing:
            raise ReadError("unterminated shell group")
        if pending:
            raise ReadError("unterminated here-document")
        return result


def commands(tokens):
    nodes, words, guard, pipeline = [], [], False, False
    for token in tokens + [";"]:
        if isinstance(token, str):
            if words:
                nodes.append({"words": words, "conditional": guard,
                              "isolated": token in ("&", "|", "||pipe"),
                              "pipeline_final": pipeline and token != "|", "background": token == "&"})
                words = []
            guard = token in ("&&", "||") or (guard and token == "|")
            pipeline = token == "|"
        else:
            words.append(token)
    return nodes


def function_definition(words):
    if len(words) == 3 and literal(words[0]) and isinstance(words[1], tuple) and words[1][:2] == ("group", "(") \
            and not words[1][2] and isinstance(words[2], tuple) and words[2][:2] == ("group", "{"):
        return literal(words[0]), words[2][2]
    if len(words) in (3, 4) and literal(words[0]) == "function" and literal(words[1]) \
            and isinstance(words[-1], tuple) and words[-1][:2] == ("group", "{"):
        return literal(words[1]), words[-1][2]
    return None


class Analysis:
    def __init__(self):
        self.calls, self.unknown = [], False
        self.reasons = []

    def unresolved(self, reason):
        self.unknown = True
        if reason not in self.reasons:
            self.reasons.append(reason)


def analyse(text):
    result = Analysis()
    walk(Lexer(text).tokens(), {}, {}, result)
    return result


def walk(tokens, scope, functions, result, uncertain=False, depth=0):
    if depth > 32:
        raise ReadError("recursive shell syntax")
    control, stopped = 0, False
    for node in commands(tokens):
        words = node["words"]
        first = literal(words[0])
        if first in ("if", "while", "until", "for", "select", "case"):
            control += 1
        ambiguous = uncertain or node["conditional"] or control > 0 or stopped or node["background"]
        definition_words = words[:]
        while definition_words and literal(definition_words[0]) in ("then", "else", "elif", "do"):
            definition_words = definition_words[1:]
        definition = function_definition(definition_words)
        if definition:
            if not node["isolated"]:
                # A possibly executed definition may shadow a real Core command.
                functions[definition[0]] = None if ambiguous or node["pipeline_final"] else definition[1]
            continue
        local = dict(scope) if node["isolated"] or ambiguous else scope
        for word in words:
            if isinstance(word, tuple) and word[0] == "group":
                child = word[1] == "(" or node["isolated"]
                ended = walk(word[2], dict(local) if child else local, dict(functions) if child else functions, result, ambiguous, depth + 1)
                if ended and not child:
                    stopped = True
            else:
                for part in word.word if isinstance(word, Redirect) else word:
                    if part[0] == "sub":
                        walk(part[1], dict(local), dict(functions), result, ambiguous, depth + 1)
                    elif part[0] == "dynamic":
                        result.unresolved("unsupported shell expansion")
        plain = [w for w in words if isinstance(w, list)]
        while plain and literal(plain[0]) in ("if", "then", "else", "elif", "fi", "do", "done", "while", "until", "!", "time", "esac"):
            plain = plain[1:]
        if plain:
            ended = run(plain, local, functions, result, ambiguous, depth)
            if ended and not node["isolated"]:
                stopped = True
        if first in ("fi", "done", "esac"):
            control = max(0, control - 1)
        if first in ("exit", "return"):
            stopped = True
        if ambiguous and not node["isolated"]:
            for name in set(scope) | set(local):
                if local.get(name) != scope.get(name):
                    scope[name] = None
        if node["pipeline_final"]:
            for word in plain:
                pair = assignment(word)
                if pair:
                    scope[pair[0]] = None

    return stopped


def run(words, scope, functions, result, uncertain, depth):
    prefix = {}
    while words and assignment(words[0]):
        name, parts = assignment(words[0])
        prefix[name] = value(parts, {})
        words = words[1:]
    if not words:
        scope.update(prefix)
        return
    wrapped, replacement = False, False
    while words:
        name = value(words[0], scope)
        base = PurePosixPath(name).name if name else None
        if name in functions:
            if wrapped:
                result.unresolved("wrapper and shell function lookup require execution evidence")
                return
            break
        if base == "env":
            wrapped = True
            words = words[1:]
            while words:
                option, pair = value(words[0], scope), assignment(words[0])
                if pair:
                    prefix[pair[0]] = value(pair[1], {})
                    words = words[1:]
                elif option and (option.startswith("-S") or option.startswith("--split-string")):
                    result.unresolved("unsupported env split-string execution")
                    return
                elif option in ("-u", "--unset", "-C", "--chdir"):
                    if len(words) < 2 or value(words[1], scope) is None:
                        result.unresolved("unreadable env option argument")
                        return
                    words = words[2:]
                elif option in ("-i", "--ignore-environment", "--") or (option and option.startswith(("--unset=", "--chdir="))):
                    words = words[1:]
                    if option == "--":
                        break
                elif option and option.startswith("-"):
                    result.unresolved("unsupported env option")
                    return
                else:
                    break
        elif base in ("command", "exec"):
            replacement |= base == "exec"
            wrapped = True
            words = words[1:]
            while words and (value(words[0], scope) or "").startswith("-"):
                option = value(words[0], scope)
                if base == "command" and option not in ("-p", "--"):
                    if option not in ("-v", "-V"):
                        result.unresolved("unsupported command option")
                    return
                if base == "exec" and option not in ("-a", "-c", "-l", "--"):
                    result.unresolved("unsupported exec option")
                    return
                words = words[2:] if option == "-a" else words[1:]
                if option == "--":
                    break
        else:
            break
    if not words:
        return
    name = value(words[0], scope)
    base = PurePosixPath(name).name if name else None
    args = [value(w, scope) for w in words[1:]]
    if replacement:
        result.unresolved("exec replacement requires actual execution evidence")
    if name in functions:
        if functions[name] is None:
            result.unresolved("conditional function definition")
        else:
            walk(functions[name], scope, dict(functions), result, True, depth + 1)
    elif name in ("export", "readonly", "declare", "typeset", "local"):
        for word in words[1:]:
            pair = assignment(word)
            if pair and value(pair[1], {}) is not None:
                scope[pair[0]] = value(pair[1], {})
            else:
                result.unresolved("unsupported shell declaration or assignment option")
    elif base == "ha":
        if uncertain or not args or args[0] is None:
            result.unresolved("conditional or dynamic Core invocation")
        else:
            result.calls.append((args[0], args[1:]))
    elif base in ("sh", "bash", "zsh", "dash", "ksh"):
        if len(args) >= 2 and args[0] in ("-c", "-lc", "-cl") and args[1] is not None:
            walk(Lexer(args[1], depth + 1).tokens(), dict(scope, **prefix), {}, result, uncertain, depth + 1)
        else:
            result.unresolved("unsupported shell file, options or input execution")
    elif name is None:
        result.unresolved("dynamic executable")
    elif base in DATA_COMMANDS:
        if base == "rg" and not data_rg(args):
            result.unresolved("unsupported ripgrep preprocessor or dynamic option")
        elif base == "printf" and not data_printf(args):
            result.unresolved("unsupported printf format or assignment option")
        elif base in ("exit", "return") and (len(args) > 1 or (args and not re.fullmatch(r"[0-9]+", args[0] or ""))):
            result.unresolved("unsupported exit or return expression")
    elif base == "git" and args and args[0] in ("status", "rev-parse", "ls-files"):
        return
    else:
        result.unresolved("unsupported evaluator or executable: " + str(base))

    return replacement or name in ("exit", "return")


# Trusted conventional data/lookup commands, not a claim about arbitrary
# replacements, shell startup hooks or every child process on the machine.
DATA_COMMANDS = {":", "true", "false", "printf", "echo", "cat", "ls", "grep", "rg",
                 "head", "tail", "wc", "pwd", "which", "type", "cd", "exit", "return"}


def data_printf(args):
    if args and args[0] == "--":
        args = args[1:]
    if not args or args[0] is None or args[0].startswith("-"):
        return False
    # Keep output conversions, never variable-writing %n or opaque formats.
    remaining = re.sub(r"%%|%[-+ #0]*[0-9]*(?:\.[0-9]+)?[sdiouxXfeEgGcb]", "", args[0])
    return "%" not in remaining


def data_rg(args):
    index = 0
    while index < len(args):
        arg = args[index]
        if arg == "--":
            return True
        if arg is None or arg == "--pre" or arg.startswith("--pre="):
            return False
        if arg in ("-e", "--regexp", "-f", "--file"):
            if index + 1 >= len(args):
                return False
            index += 1
        index += 1
    return True


SAFE_BUILTINS = {":", "true", "false", "printf", "echo", "return", "exit",
                 "if", "then", "elif", "else", "fi", "while", "until", "do", "done", "!"}


def covered(text):
    """Whether every possible program in the supported script uses the launcher or a safe builtin.

    This deliberately rejects arbitrary external programs, dynamic command
    words, startup hooks, asynchronous jobs, eval/source and shell rewrites.
    It is a coverage check, not a prediction of the branches the shell takes.
    """
    def inspect(tokens, functions):
        control = 0
        for node in commands(tokens):
            if node["background"]:
                return False
            words = node["words"][:]
            first = literal(words[0])
            if first in ("if", "while", "until", "for", "select", "case"):
                control += 1
            while words and literal(words[0]) in ("then", "else", "elif", "do"):
                words = words[1:]
            definition = function_definition(words)
            if definition:
                name, body = definition
                if control or node["conditional"] or node["isolated"] or node["pipeline_final"] \
                        or not re.fullmatch(r"[A-Za-z_][A-Za-z0-9_]*", name) or name in SAFE_BUILTINS or name == "ha":
                    return False
                functions.add(name)
                # Definitions inside a function body do not exist until called.
                if not inspect(body, set(functions)):
                    return False
                continue
            plain, launcher_references = [], []
            for word in node["words"]:
                # Arbitrary destinations (including globbed paths) can rewrite
                # the wrapper or journal. Static analysis still reads these,
                # but the opt-in complete-coverage collector excludes them.
                if isinstance(word, Redirect):
                    return False
                if isinstance(word, tuple):
                    if not inspect(word[2], set(functions)):
                        return False
                else:
                    parts = word.word if isinstance(word, Redirect) else word
                    if any(p[0] == "dynamic" or (p[0] == "sub" and not inspect(p[1], set(functions))) for p in parts):
                        return False
                    if any(p[0] == "var" and p[1].startswith("JAEKIT_") and p[1] != "JAEKIT_CORE" for p in parts):
                        return False
                    if any(p[0] == "var" and p[1] == "JAEKIT_CORE" for p in parts):
                        launcher_references.append(word)
                    if isinstance(word, list):
                        plain.append(word)
            while plain and literal(plain[0]) in SAFE_BUILTINS & {"if", "then", "elif", "else", "fi", "while", "until", "do", "done", "!"}:
                plain.pop(0)
            for word in plain:
                pair = assignment(word)
                if pair and (pair[0].startswith("JAEKIT_") or pair[0] in ("PATH", "IFS", "ENV", "BASH_ENV", "SHELLOPTS", "BASHOPTS", "ZDOTDIR")):
                    return False
            while plain and assignment(plain[0]):
                plain.pop(0)
            if launcher_references and (len(launcher_references) != 1 or not plain
                    or launcher_references[0] is not plain[0]):
                # Do not expose the writable wrapper as an argument, assignment
                # or redirection target that can replace or alias it.
                return False
            if plain and literal(plain[0]) == "printf":
                arguments = plain[1:]
                if arguments and literal(arguments[0]) == "--":
                    arguments = arguments[1:]
                format_text = literal(arguments[0]) if arguments else None
                # printf -v and zsh's %n can assign variables without NAME=.
                # Dynamic formats and test/[[ arithmetic are outside coverage.
                if format_text is None or format_text.startswith("-") or re.search(r"%[-+ #0]*(?:[0-9]+|\*)?(?:\.(?:[0-9]+|\*))?[hljztL]*n", format_text):
                    return False
            if plain and literal(plain[0]) in ("exit", "return"):
                if len(plain) > 2 or (len(plain) == 2 and not re.fullmatch(r"[0-9]+", literal(plain[1]) or "")):
                    return False
            if plain and literal(plain[0]) not in SAFE_BUILTINS | functions:
                if [p for p in plain[0] if p != ("text", "", True)] != [("var", "JAEKIT_CORE", True)]:
                    return False
            if first in ("fi", "done", "esac"):
                control = max(0, control - 1)
        return True
    try:
        return inspect(Lexer(text).tokens(), set())
    except ReadError:
        return False
