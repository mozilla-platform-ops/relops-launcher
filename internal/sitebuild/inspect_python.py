"""Analyze Python source supplied on stdin; never import or execute it."""

import ast
import json
import sys


def dotted(node):
    if isinstance(node, ast.Name):
        return node.id
    if isinstance(node, ast.Attribute):
        return dotted(node.value) + "." + node.attr
    return ""


def literal(node):
    return (
        node.value
        if isinstance(node, ast.Constant) and isinstance(node.value, str)
        else None
    )


def path_hint(node):
    if isinstance(node, ast.Constant):
        return literal(node)
    if isinstance(node, ast.BinOp) and isinstance(node.op, (ast.Div, ast.Add)):
        left, right = path_hint(node.left), path_hint(node.right)
        if left is not None and right is not None:
            return left.rstrip("/") + "/" + right.lstrip("/")
    if isinstance(node, ast.Call):
        name = dotted(node.func)
        if name in ("Path.home", "pathlib.Path.home"):
            return "~"
        if name in ("Path", "pathlib.Path", "os.path.expanduser") and node.args:
            return path_hint(node.args[0])
        if name == "os.path.join" and node.args:
            parts = [path_hint(arg) for arg in node.args]
            if all(part is not None for part in parts):
                return "/".join(parts)
    return None


def analyze(item):
    findings = []

    def emit(node, rule, detail):
        findings.append({"line": node.lineno, "rule": rule, "detail": detail})

    try:
        tree = ast.parse(item["source"], filename=item["path"])
    except (SyntaxError, ValueError) as exc:
        return [
            {
                "line": getattr(exc, "lineno", None) or 1,
                "rule": "python-parse",
                "detail": str(exc),
            }
        ]
    for node in ast.walk(tree):
        if not isinstance(node, ast.Call):
            continue
        name = dotted(node.func)
        path = node.args[0] if node.args else None
        mode = None
        if name in ("open", "io.open") and path:
            mode = literal(node.args[1]) if len(node.args) > 1 else "r"
            for keyword in node.keywords:
                if keyword.arg == "mode":
                    mode = literal(keyword.value)
        elif isinstance(node.func, ast.Attribute) and node.func.attr in (
            "write_text",
            "write_bytes",
            "open",
        ):
            path = node.func.value
            mode = (
                "w"
                if node.func.attr != "open"
                else (literal(node.args[0]) if node.args else "r")
            )
            for keyword in node.keywords:
                if keyword.arg == "mode":
                    mode = literal(keyword.value)
        if path is None or mode is None:
            continue
        hint = path_hint(path)
        if hint and hint.startswith("~/") and hint not in (item["data_files"] or []):
            emit(
                node,
                "undocumented-data-file",
                "possible data file " + hint + " is not declared in data_files",
            )
        if any(char in mode for char in "wax+"):
            if any(
                isinstance(n, ast.Name) and n.id == "__file__" for n in ast.walk(path)
            ):
                emit(
                    node,
                    "checkout-write",
                    "write path refers to __file__; output may be removed with the checkout",
                )
            elif item["temporary_cwd"] and hint and not hint.startswith(("/", "~")):
                emit(
                    node,
                    "relative-write",
                    "relative write may be removed with the temporary working directory: "
                    + hint,
                )
    return findings


items = json.load(sys.stdin)
json.dump([analyze(item) for item in items], sys.stdout)
