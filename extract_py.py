"""thresh's Python extractor: reads one Python source on stdin with the stdlib `ast` and writes its facts as
JSON to stdout. Embedded in the binary (extract_py.go) and run by the target's own python3; the target
need not import or run.

    {"ok": bool,
     "funcs": [{"name": "Class.method", "line": n}, ...],          every function and method, qualified
     "uses": [{"name": "a.b.c", "line": n, "func": "Class.method"}, ...],
         every dotted name referenced, through the file's imports, with the function it sits in ("" for
         module level); an import statement is a use of each name it binds
     "registries": {"NAME": [{"key": "k", "line": n}, ...], ...},
         each module-level dict, set, list or tuple literal (or dict(...) call) bound to a name, and each
         module-level `NAME[k] = ...`: its keys or elements, string constants and dotted names
     "normalized": "..."}                                            the module's AST without docstrings

"ok" is false, the rest empty, where the source does not parse.
"""
import ast
import json
import sys


def dotted(node, aliases):
    """The dotted name an expression of names and attributes spells, its root through `aliases`; None for
    anything else (a call's result, a subscript)."""
    parts = []
    while isinstance(node, ast.Attribute):
        parts.append(node.attr)
        node = node.value
    if not isinstance(node, ast.Name):
        return None
    parts.append(aliases.get(node.id, node.id))
    return ".".join(reversed(parts))


def bound(node):
    """Each (local name, dotted name) an import statement binds, and each dotted name it imports."""
    if isinstance(node, ast.Import):
        for alias in node.names:
            if alias.asname:
                yield alias.asname, alias.name, alias.name
            else:
                root = alias.name.split(".")[0]
                yield root, root, alias.name
    elif isinstance(node, ast.ImportFrom) and node.module and not node.level:
        for alias in node.names:
            if alias.name != "*":
                name = f"{node.module}.{alias.name}"
                yield alias.asname or alias.name, name, name


def key_of(node):
    """A registry key or element: a string constant's value, or a dotted name; None for anything else."""
    if isinstance(node, ast.Constant) and isinstance(node.value, (str, int)):
        return str(node.value)
    return dotted(node, {})


def entries(value):
    """The keys or elements of a registry literal: a dict, set, list or tuple display, or dict(...) by
    keywords, through frozenset(...), set(...), tuple(...) or MappingProxyType(...) around one."""
    while (isinstance(value, ast.Call) and isinstance(value.func, (ast.Name, ast.Attribute))
           and dotted(value.func, {}) in ("frozenset", "set", "tuple", "list", "MappingProxyType",
                                          "types.MappingProxyType") and len(value.args) == 1):
        value = value.args[0]
    if isinstance(value, ast.Dict):
        return [(key, key.lineno) for key in value.keys if key is not None]
    if isinstance(value, (ast.Set, ast.List, ast.Tuple)):
        return [(element, element.lineno) for element in value.elts]
    if isinstance(value, ast.Call) and dotted(value.func, {}) == "dict" and not value.args:
        return [(ast.Constant(keyword.arg), keyword.value.lineno) for keyword in value.keywords if keyword.arg]
    return None


def undocumented(tree):
    """`tree` without its docstrings: the first statement of a module, class or function when it is a string."""
    for node in ast.walk(tree):
        if isinstance(node, (ast.Module, ast.ClassDef, ast.FunctionDef, ast.AsyncFunctionDef)):
            body = node.body
            if body and isinstance(body[0], ast.Expr) and isinstance(body[0].value, ast.Constant) \
                    and isinstance(body[0].value.value, str):
                node.body = body[1:] or [ast.Pass()]
    return tree


class Facts(ast.NodeVisitor):
    def __init__(self):
        self.funcs, self.uses, self.registries = [], [], {}
        self.aliases, self.scope = {}, []

    def within(self, node, name):
        self.scope.append(name)
        self.generic_visit(node)
        self.scope.pop()

    def visit_ClassDef(self, node):
        for decorator in node.decorator_list:
            self.visit(decorator)
        for base in node.bases:
            self.visit(base)
        self.scope.append(node.name)
        for statement in node.body:
            self.visit(statement)
        self.scope.pop()

    def visit_FunctionDef(self, node):
        self.funcs.append({"name": ".".join([*self.scope, node.name]), "line": node.lineno})
        for decorator in node.decorator_list:
            self.visit(decorator)
        self.scope.append(node.name)
        self.visit(node.args)
        if node.returns:
            self.visit(node.returns)
        for statement in node.body:
            self.visit(statement)
        self.scope.pop()

    visit_AsyncFunctionDef = visit_FunctionDef

    def use(self, name, line):
        self.uses.append({"name": name, "line": line, "func": ".".join(self.scope)})

    def visit_Import(self, node):
        for local, name, imported in bound(node):
            self.aliases[local] = name
            self.use(imported, node.lineno)

    visit_ImportFrom = visit_Import

    def visit_Attribute(self, node):
        name = dotted(node, self.aliases)
        if name is None:
            self.generic_visit(node)
        else:
            self.use(name, node.lineno)

    def visit_Name(self, node):
        self.use(self.aliases.get(node.id, node.id), node.lineno)

    def registry(self, name, found):
        if found is not None:
            listed = self.registries.setdefault(name, [])
            for key, line in found:
                if (text := key_of(key)) is not None:
                    listed.append({"key": text, "line": line})

    def visit_Assign(self, node):
        if not self.scope:
            for target in node.targets:
                if isinstance(target, ast.Name):
                    self.registry(target.id, entries(node.value))
                elif isinstance(target, ast.Subscript) and isinstance(target.value, ast.Name):
                    self.registry(target.value.id, [(target.slice, node.lineno)])
        self.generic_visit(node)

    def visit_AnnAssign(self, node):
        if not self.scope and isinstance(node.target, ast.Name) and node.value is not None:
            self.registry(node.target.id, entries(node.value))
        self.generic_visit(node)


def main():
    source = sys.stdin.read()
    try:
        tree = ast.parse(source)
    except (SyntaxError, ValueError):
        json.dump({"ok": False, "funcs": [], "uses": [], "registries": {}, "normalized": ""}, sys.stdout)
        return
    facts = Facts()
    facts.visit(tree)
    json.dump({"ok": True, "funcs": facts.funcs, "uses": facts.uses, "registries": facts.registries,
               "normalized": ast.dump(undocumented(tree), include_attributes=False)}, sys.stdout)


main()
