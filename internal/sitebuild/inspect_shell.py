"""Heuristically inspect shell source supplied on stdin without executing it."""

import json
import re
import shlex
import sys


VARIABLE = re.compile(
    r"\$(?:\{([A-Za-z_][A-Za-z_0-9]*)[^}]*\}|([A-Za-z_][A-Za-z_0-9]*))"
)
ASSIGNMENT = re.compile(
    r"^\s*(?:export\s+|readonly\s+|local\s+)?([A-Za-z_][A-Za-z_0-9]*)="
)
REDIRECT = re.compile(r"(?<![<>])(?:\d*>>?)(?![>&])\s*([^\s;|&]+)")


def analyze(item):
    findings = []
    checkout_vars = set()
    for number, line in enumerate(item["source"].splitlines(), 1):
        try:
            lexer = shlex.shlex(line, posix=True, punctuation_chars=";|&")
            lexer.whitespace_split = True
            tokens = list(lexer)
        except ValueError:
            # Multiline shell constructs cannot be understood by this line scanner.
            continue
        if not tokens:
            continue
        # Strip comments outside quotes while retaining the original quoting for redirects.
        # shlex identifies the comment boundary, but a hash inside a quoted path is valid.
        assignment = ASSIGNMENT.match(line)
        if assignment:
            value = line[assignment.end() :]
            if ("BASH_SOURCE" in value or re.search(r"\$\{?0\}?", value)) and (
                "dirname" in value or "%/" in value
            ):
                checkout_vars.add(assignment.group(1))
            elif "git rev-parse" in value and "--show-toplevel" in value:
                checkout_vars.add(assignment.group(1))
            elif any((a or b) in checkout_vars for a, b in VARIABLE.findall(value)):
                checkout_vars.add(assignment.group(1))

        # Ignore redirects in comments and quoted strings by scanning lexical state.
        clean = []
        quote = None
        escaped = False
        for char in line:
            if escaped:
                clean.append(char)
                escaped = False
            elif char == "\\" and quote != "'":
                clean.append(char)
                escaped = True
            elif char in "'\"":
                if quote == char:
                    quote = None
                elif quote is None:
                    quote = char
                clean.append(char)
            elif char == "#" and quote is None:
                break
            elif char in "><" and quote is not None:
                clean.append(" ")
            else:
                clean.append(char)
        candidates = [
            match.group(1).strip("'\"") for match in REDIRECT.finditer("".join(clean))
        ]
        # Recognize common commands with output paths. Options and complex shell
        # expressions are deliberately not evaluated.
        for segment in re.split(r"(?:&&|\|\||[;|&])", " ".join(tokens)):
            words = segment.split()
            if not words:
                continue
            command = words[0].rsplit("/", 1)[-1]
            args = [word for word in words[1:] if not word.startswith("-")]
            if command in ("touch", "mkdir", "tee"):
                candidates.extend(args)
            elif command in ("cp", "mv", "install") and len(args) >= 2:
                candidates.append(args[-1])
            elif (
                command == "sed"
                and any(word == "-i" or word.startswith("-i") for word in words[1:])
                and len(args) >= 2
            ):
                candidates.extend(args[1:])
        seen = set()
        for candidate in candidates:
            if not candidate or candidate in seen:
                continue
            seen.add(candidate)
            variables = {a or b for a, b in VARIABLE.findall(candidate)}
            if (
                variables & checkout_vars
                or "BASH_SOURCE" in candidate
                or re.search(r"\$\{?0\}?", candidate)
            ):
                rule = "checkout-write"
                detail = "write path may refer to the temporary checkout: " + candidate
            elif (
                item["temporary_cwd"]
                and not candidate.startswith(("/", "~", "$"))
                and "$" not in candidate
                and not candidate.startswith(("&", ">"))
            ):
                rule = "relative-write"
                detail = (
                    "relative write may be removed with the temporary working directory: "
                    + candidate
                )
            else:
                continue
            findings.append({"line": number, "rule": rule, "detail": detail})
    return findings


items = json.load(sys.stdin)
json.dump([analyze(item) for item in items], sys.stdout)
