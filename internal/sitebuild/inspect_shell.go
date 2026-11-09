package sitebuild

import (
	"bytes"
	"fmt"
	"path"
	"sort"
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

// Symbolic roots distinguish checkout paths from paths outside the checkout.
// No inspected commands are executed and no environment variables are read.
const checkoutRoot = "/@checkout"

type shellValues []string // An empty string represents an unknown value.
type shellState struct {
	cwd, previous, output shellValues
	vars                  map[string]shellValues
}

func (s *shellState) clone() *shellState {
	c := *s
	c.vars = make(map[string]shellValues, len(s.vars))
	for k, v := range s.vars {
		c.vars[k] = v
	}
	return &c
}

func unionValues(a, b shellValues) shellValues {
	out := append(shellValues{}, a...)
	for _, v := range b {
		found := false
		for _, existing := range out {
			if v == existing {
				found = true
				break
			}
		}
		if !found {
			out = append(out, v)
		}
	}
	// Bound state growth on scripts with many branches.
	if len(out) > 16 {
		return shellValues{""}
	}
	return out
}

func mergeShell(a, b *shellState) *shellState {
	c := a.clone()
	c.cwd = unionValues(a.cwd, b.cwd)
	c.previous = unionValues(a.previous, b.previous)
	c.output = unionValues(a.output, b.output)
	for k, v := range c.vars {
		other, ok := b.vars[k]
		if !ok {
			other = shellValues{""}
		}
		c.vars[k] = unionValues(v, other)
	}
	for k, v := range b.vars {
		if _, ok := c.vars[k]; !ok {
			c.vars[k] = unionValues(shellValues{""}, v)
		}
	}
	return c
}

type shellAnalysis struct {
	findings []finding
	seen     map[string]bool
}

func inspectShell(input pythonInput) []finding {
	file, err := syntax.NewParser(syntax.Variant(syntax.LangBash)).Parse(strings.NewReader(input.Source), input.Path)
	if err != nil {
		line := 1
		if e, ok := err.(syntax.ParseError); ok {
			line = int(e.Pos.Line())
		}
		return []finding{{Line: line, Rule: "shell-parse", Detail: err.Error()}}
	}
	cwd := "/@caller"
	if input.TemporaryCwd {
		cwd = checkoutRoot
	}
	s := &shellState{cwd: shellValues{cwd}, previous: shellValues{""}, vars: map[string]shellValues{
		"0": {path.Join(checkoutRoot, input.Path)}, "BASH_SOURCE": {path.Join(checkoutRoot, input.Path)}, "HOME": {"/@home"},
	}}
	a := &shellAnalysis{seen: map[string]bool{}}
	a.stmts(file.Stmts, s)
	sort.SliceStable(a.findings, func(i, j int) bool { return a.findings[i].Line < a.findings[j].Line })
	return a.findings
}

func shellText(n syntax.Node) string {
	var b bytes.Buffer
	_ = syntax.NewPrinter().Print(&b, n)
	return b.String()
}

func (a *shellAnalysis) parts(parts []syntax.WordPart, s *shellState) shellValues {
	result := shellValues{""}
	// Unlike an unknown value, the initial empty prefix is a known literal.
	initial := true
	for _, part := range parts {
		var v shellValues
		switch p := part.(type) {
		case *syntax.Lit:
			v = shellValues{p.Value}
		case *syntax.SglQuoted:
			v = shellValues{p.Value}
		case *syntax.DblQuoted:
			v = a.parts(p.Parts, s)
		case *syntax.ParamExp:
			v = s.vars[p.Param.Value]
			if p.Param.Value == "PWD" {
				v = s.cwd
			}
			if p.Param.Value == "OLDPWD" {
				v = s.previous
			}
			if p.Excl || p.Length || p.Width || p.Slice != nil || p.Repl != nil || p.Names != 0 || (p.Index != nil && (p.Param.Value != "BASH_SOURCE" || shellText(p.Index) != "0")) {
				v = nil
			}
			if p.Exp != nil {
				if p.Exp.Op == syntax.RemSmallSuffix && p.Exp.Word.Lit() == "/*" {
					v = mapValues(v, path.Dir)
				} else {
					v = nil
				}
			}
		case *syntax.CmdSubst:
			child := s.clone()
			a.stmts(p.Stmts, child)
			v = child.output
		case *syntax.ProcSubst:
			a.stmts(p.Stmts, s.clone())
		}
		if len(v) == 0 {
			v = shellValues{""}
		}
		if initial {
			result = v
			initial = false
			continue
		}
		var combined shellValues
		for _, x := range result {
			for _, y := range v {
				value := ""
				if x != "" && y != "" {
					value = x + y
				}
				combined = unionValues(combined, shellValues{value})
			}
		}
		result = combined
	}
	return result
}

func mapValues(v shellValues, fn func(string) string) shellValues {
	out := shellValues{}
	for _, x := range v {
		if x != "" {
			x = fn(x)
		}
		out = unionValues(out, shellValues{x})
	}
	return out
}

func (a *shellAnalysis) word(w *syntax.Word, s *shellState) shellValues {
	if w == nil {
		return shellValues{""}
	}
	return a.parts(w.Parts, s)
}

func resolveShell(v, cwd shellValues) shellValues {
	var out shellValues
	for _, value := range v {
		if value == "" {
			out = unionValues(out, shellValues{""})
			continue
		}
		if strings.HasPrefix(value, "/") {
			out = unionValues(out, shellValues{path.Clean(value)})
			continue
		}
		if strings.HasPrefix(value, "~") {
			out = unionValues(out, shellValues{"/@home"})
			continue
		}
		for _, dir := range cwd {
			resolved := ""
			if dir != "" {
				resolved = path.Join(dir, value)
			}
			out = unionValues(out, shellValues{resolved})
		}
	}
	return out
}

func (a *shellAnalysis) write(w *syntax.Word, values shellValues, s *shellState) {
	for _, resolved := range resolveShell(values, s.cwd) {
		if resolved != checkoutRoot && !strings.HasPrefix(resolved, checkoutRoot+"/") {
			continue
		}
		rule := "relative-write"
		for _, v := range values {
			if strings.HasPrefix(v, checkoutRoot) {
				rule = "checkout-write"
			}
		}
		detail := "write may be removed with the temporary checkout: " + shellText(w)
		key := fmt.Sprintf("%d:%s:%s", w.Pos().Line(), rule, detail)
		if !a.seen[key] {
			a.findings = append(a.findings, finding{Line: int(w.Pos().Line()), Rule: rule, Detail: detail})
			a.seen[key] = true
		}
		return
	}
}

func (a *shellAnalysis) stmts(stmts []*syntax.Stmt, s *shellState) {
	for _, stmt := range stmts {
		a.stmt(stmt, s)
	}
}

func (a *shellAnalysis) stmt(stmt *syntax.Stmt, s *shellState) {
	if stmt == nil {
		return
	}
	if stmt.Background {
		s = s.clone()
	}
	// Redirections are expanded before running the command, including cd.
	for _, r := range stmt.Redirs {
		switch r.Op {
		case syntax.RdrOut, syntax.AppOut, syntax.ClbOut, syntax.RdrAll, syntax.AppAll, syntax.RdrInOut:
			a.write(r.Word, a.word(r.Word, s), s)
		default:
			a.word(r.Word, s)
		}
		if r.Hdoc != nil {
			a.word(r.Hdoc, s)
		}
	}
	s.output = shellValues{""}
	switch c := stmt.Cmd.(type) {
	case *syntax.CallExpr:
		a.call(c, s)
	case *syntax.DeclClause:
		for _, assign := range c.Args {
			a.assign(assign, s)
		}
	case *syntax.Block:
		a.stmts(c.Stmts, s)
	case *syntax.Subshell:
		child := s.clone()
		a.stmts(c.Stmts, child)
		s.output = child.output
	case *syntax.BinaryCmd:
		if c.Op == syntax.Pipe || c.Op == syntax.PipeAll {
			a.stmt(c.X, s.clone())
			a.stmt(c.Y, s.clone())
		} else {
			before := s.clone()
			a.stmt(c.X, s)
			afterX := s.clone()
			if c.Op == syntax.OrStmt {
				*s = *mergeShell(before, s)
			}
			a.stmt(c.Y, s)
			*s = *mergeShell(afterX, s)
		}
	case *syntax.IfClause:
		if !c.ThenPos.IsValid() {
			a.stmts(c.Then, s)
			break
		}
		a.stmts(c.Cond, s)
		other := s.clone()
		a.stmts(c.Then, s)
		if c.Else != nil {
			a.stmt(&syntax.Stmt{Cmd: c.Else}, other)
		}
		*s = *mergeShell(s, other)
	case *syntax.WhileClause:
		a.stmts(c.Cond, s)
		before := s.clone()
		a.stmts(c.Do, s)
		*s = *mergeShell(before, s)
	case *syntax.ForClause:
		before := s.clone()
		if iter, ok := c.Loop.(*syntax.WordIter); ok {
			var values shellValues
			for _, item := range iter.Items {
				values = unionValues(values, a.word(item, s))
			}
			if len(values) == 0 {
				values = shellValues{""}
			}
			s.vars[iter.Name.Value] = values
		}
		a.stmts(c.Do, s)
		*s = *mergeShell(before, s)
	case *syntax.CaseClause:
		a.word(c.Word, s)
		merged := s.clone()
		for _, item := range c.Items {
			branch := s.clone()
			a.stmts(item.Stmts, branch)
			merged = mergeShell(merged, branch)
		}
		*s = *merged
	case *syntax.FuncDecl:
		// Scan function bodies independently; this is not a call-graph analysis.
		a.stmt(c.Body, s.clone())
	case *syntax.TimeClause:
		a.stmt(c.Stmt, s)
	default:
		// Visit expansions in test/arithmetic and other unsupported command forms.
		if stmt.Cmd == nil {
			break
		}
		syntax.Walk(stmt.Cmd, func(n syntax.Node) bool {
			if w, ok := n.(*syntax.Word); ok {
				a.word(w, s)
				return false
			}
			return true
		})
	}
}

func (a *shellAnalysis) assign(assign *syntax.Assign, s *shellState) {
	value := a.word(assign.Value, s)
	if assign.Name == nil || assign.Naked {
		return
	}
	if assign.Append || assign.Index != nil || assign.Array != nil {
		value = shellValues{""}
	}
	s.vars[assign.Name.Value] = value
}

func (a *shellAnalysis) call(c *syntax.CallExpr, s *shellState) {
	if len(c.Args) == 0 {
		for _, assign := range c.Assigns {
			a.assign(assign, s)
		}
		return
	}
	// Command-prefix assignments don't persist and don't affect argument expansion.
	for _, assign := range c.Assigns {
		a.word(assign.Value, s)
	}
	args := make([]shellValues, len(c.Args))
	for i, word := range c.Args {
		args[i] = a.word(word, s)
	}
	literal := func(i int) string {
		if i < len(args) && len(args[i]) == 1 {
			return args[i][0]
		}
		return ""
	}
	command := path.Base(literal(0))
	switch command {
	case "cd":
		target := shellValues{"/@home"}
		for i := 1; i < len(args); i++ {
			option := literal(i)
			if option == "--" || option == "-P" || option == "-L" {
				continue
			}
			target = args[i]
			break
		}
		previous := s.cwd
		if len(target) == 1 && target[0] == "-" {
			s.cwd = s.previous
		} else {
			s.cwd = resolveShell(target, s.cwd)
		}
		s.previous = previous
	case "pwd":
		s.output = s.cwd
	case "dirname":
		if len(args) == 2 {
			s.output = mapValues(args[1], path.Dir)
		} else if len(args) == 3 && literal(1) == "--" {
			s.output = mapValues(args[2], path.Dir)
		}
	case "git":
		if len(args) == 3 && literal(1) == "rev-parse" && literal(2) == "--show-toplevel" {
			for _, dir := range s.cwd {
				if dir == checkoutRoot || strings.HasPrefix(dir, checkoutRoot+"/") {
					s.output = shellValues{checkoutRoot}
					break
				}
			}
		}
	case "unset":
		for i := 1; i < len(args); i++ {
			delete(s.vars, literal(i))
		}
	}
	var operands []int
	for i := 1; i < len(args); i++ {
		if !strings.HasPrefix(literal(i), "-") {
			operands = append(operands, i)
		}
	}
	var writes []int
	switch command {
	case "touch", "mkdir", "tee":
		writes = operands
	case "cp", "mv", "install":
		if len(operands) >= 2 {
			writes = operands[len(operands)-1:]
		}
	case "sed":
		inplace := false
		for i := 1; i < len(args); i++ {
			if strings.HasPrefix(literal(i), "-i") {
				inplace = true
			}
		}
		if inplace && len(operands) >= 2 {
			writes = operands[1:]
		}
	}
	for _, i := range writes {
		a.write(c.Args[i], args[i], s)
	}
}
