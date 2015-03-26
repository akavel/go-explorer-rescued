// Copyright 2015 Gary Burd. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"flag"
	"fmt"
	"go/ast"
	"go/doc"
	"go/printer"
	"go/scanner"
	"go/token"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"
)

func init() {
	var fs flag.FlagSet
	commands["doc"] = &command{
		fs: &fs,
		do: func() { os.Exit(doDoc(os.Stdout, fs.Args())) },
	}
}

func doDoc(out io.Writer, args []string) int {
	if len(args) != 1 {
		fmt.Fprint(out, "one command line argument expected")
		return 1
	}
	importPath := args[0]
	importPath = strings.TrimPrefix(importPath, "godoc://")
	pkg, err := loadPackage(importPath)
	if err != nil {
		fmt.Fprint(out, err)
		return 1
	}

	mode := doc.Mode(0)
	if pkg.BPkg.ImportPath == "builtin" {
		mode |= doc.AllDecls
	}

	dpkg := doc.New(pkg.APkg, pkg.BPkg.ImportPath, mode)

	if pkg.BPkg.ImportPath == "builtin" {
		for _, t := range dpkg.Types {
			dpkg.Funcs = append(dpkg.Funcs, t.Funcs...)
			t.Funcs = nil
		}
		sort.Sort(byFuncName(dpkg.Funcs))
	}

	p := docPrinter{
		w:       &bytes.Buffer{},
		dpkg:    dpkg,
		fset:    pkg.FSet,
		lineNum: 1,
	}
	p.execute(out)
	return 0
}

type docPrinter struct {
	w *bytes.Buffer

	fset *token.FileSet
	dpkg *doc.Package

	lineNum  int
	linePos  int
	lineScan int
}

func (p *docPrinter) execute(out io.Writer) {
	fmt.Fprintf(p.w, "package %s\n_\n", p.dpkg.Name)
	fmt.Fprintf(p.w, "    import \"%s\"\n\n", p.dpkg.ImportPath)
	p.doc(p.dpkg.Doc)

	p.head("CONSTANTS", len(p.dpkg.Consts))
	p.values(p.dpkg.Consts)

	p.head("VARIABLES", len(p.dpkg.Vars))
	p.values(p.dpkg.Vars)

	p.head("FUNCTIONS", len(p.dpkg.Funcs))
	p.funcs(p.dpkg.Funcs)

	p.head("TYPES", len(p.dpkg.Types))
	for _, d := range p.dpkg.Types {
		p.decl(d.Decl)
		p.doc(d.Doc)
		p.values(d.Consts)
		p.values(d.Vars)
		p.funcs(d.Funcs)
		p.funcs(d.Methods)
	}
	out.Write(p.w.Bytes())
}

const (
	noAnnotation = iota
	anchorAnnotation
	packageLinkAnnotation
	builtinAnnotation
	linkAnnotation
	linkBeginAnnotation
	linkEndAnnotation
)

type annotation struct {
	kind int
	data string
	pos  token.Pos
}

func (p *docPrinter) decl(decl ast.Decl) {
	v := &declVisitor{}
	ast.Walk(v, decl)
	var w bytes.Buffer
	err := (&printer.Config{Mode: printer.UseSpaces, Tabwidth: 4}).Fprint(
		&w,
		p.fset,
		&printer.CommentedNode{Node: decl, Comments: v.comments})
	if err != nil {
		p.w.WriteString(err.Error())
		return
	}
	buf := bytes.TrimRight(w.Bytes(), " \t\n")

	var s scanner.Scanner
	fset := token.NewFileSet()
	file := fset.AddFile("", fset.Base(), len(buf))
	base := file.Base()
	s.Init(file, buf, nil, scanner.ScanComments)
	lastOffset := 0
loop:
	for {
		pos, tok, lit := s.Scan()
		switch tok {
		case token.EOF:
			break loop
		case token.IDENT:
			if len(v.annotations) == 0 {
				// Oops!
				break loop
			}
			offset := int(pos) - base
			p.w.Write(buf[lastOffset:offset])
			lastOffset = offset + len(lit)
			a := v.annotations[0]
			v.annotations = v.annotations[1:]
			switch a.kind {
			case linkBeginAnnotation:
				p.w.WriteByte('|')
				p.w.WriteString(lit)
			case linkEndAnnotation:
				p.w.WriteString(lit)
				p.w.WriteByte('|')
			case linkAnnotation, packageLinkAnnotation, builtinAnnotation:
				p.w.WriteByte('|')
				p.w.WriteString(lit)
				p.w.WriteByte('|')
			case anchorAnnotation:
				p.w.WriteByte('!')
				p.w.WriteString(lit)
				p.w.WriteByte('!')
			default:
				p.w.WriteString(lit)
			}
		}
	}
	p.w.Write(buf[lastOffset:])
	p.w.WriteString("\n_\n")
}

func (p *docPrinter) doc(s string) {
	s = strings.TrimRight(s, " \t\n")
	if s != "" {
		doc.ToText(p.w, s, "    ", "      ", 80)
		b := p.w.Bytes()
		if b[len(b)-1] != '\n' {
			p.w.WriteByte('\n')
		}
		p.w.WriteByte('\n')
	}
}

func (p *docPrinter) head(title string, n int) {
	if n > 0 {
		fmt.Fprintf(p.w, "%s\n\n", title)
	}
}

func (p *docPrinter) values(values []*doc.Value) {
	for _, d := range values {
		p.decl(d.Decl)
		p.doc(d.Doc)
	}
}

func (p *docPrinter) funcs(values []*doc.Func) {
	for _, d := range values {
		p.decl(d.Decl)
		p.doc(d.Doc)
	}
}

func (p *docPrinter) lineAndColumn() (int, int) {
	b := p.w.Bytes()
	for i, c := range b[p.lineScan:] {
		if c == '\n' {
			p.lineNum += 1
			p.linePos = p.lineScan + i
		}
	}
	p.lineScan = len(b)
	return p.lineNum, len(b) - p.linePos
}

const (
	notPredeclared = iota
	predeclaredType
	predeclaredConstant
	predeclaredFunction
)

// predeclared represents the set of all predeclared identifiers.
var predeclared = map[string]int{
	"bool":       predeclaredType,
	"byte":       predeclaredType,
	"complex128": predeclaredType,
	"complex64":  predeclaredType,
	"error":      predeclaredType,
	"float32":    predeclaredType,
	"float64":    predeclaredType,
	"int16":      predeclaredType,
	"int32":      predeclaredType,
	"int64":      predeclaredType,
	"int8":       predeclaredType,
	"int":        predeclaredType,
	"rune":       predeclaredType,
	"string":     predeclaredType,
	"uint16":     predeclaredType,
	"uint32":     predeclaredType,
	"uint64":     predeclaredType,
	"uint8":      predeclaredType,
	"uint":       predeclaredType,
	"uintptr":    predeclaredType,

	"true":  predeclaredConstant,
	"false": predeclaredConstant,
	"iota":  predeclaredConstant,
	"nil":   predeclaredConstant,

	"append":  predeclaredFunction,
	"cap":     predeclaredFunction,
	"close":   predeclaredFunction,
	"complex": predeclaredFunction,
	"copy":    predeclaredFunction,
	"delete":  predeclaredFunction,
	"imag":    predeclaredFunction,
	"len":     predeclaredFunction,
	"make":    predeclaredFunction,
	"new":     predeclaredFunction,
	"panic":   predeclaredFunction,
	"print":   predeclaredFunction,
	"println": predeclaredFunction,
	"real":    predeclaredFunction,
	"recover": predeclaredFunction,
}

// declVisitor modifies a declaration AST for printing and collects annotations.
type declVisitor struct {
	annotations []*annotation
	comments    []*ast.CommentGroup
}

func (v *declVisitor) add(kind int, data string, pos token.Pos) {
	v.annotations = append(v.annotations, &annotation{kind: kind, data: data, pos: pos})
}

func (v *declVisitor) ignoreName() {
	v.annotations = append(v.annotations, &annotation{kind: noAnnotation})
}

func (v *declVisitor) Visit(n ast.Node) ast.Visitor {
	switch n := n.(type) {
	case *ast.TypeSpec:
		v.add(anchorAnnotation, "", n.Pos())
		name := n.Name.Name
		switch n := n.Type.(type) {
		case *ast.InterfaceType:
			for _, f := range n.Methods.List {
				for _, n := range f.Names {
					v.add(anchorAnnotation, name, n.Pos())
				}
				ast.Walk(v, f.Type)
			}
		case *ast.StructType:
			for _, f := range n.Fields.List {
				for _, n := range f.Names {
					v.add(anchorAnnotation, name, n.Pos())
				}
				ast.Walk(v, f.Type)
			}
		default:
			ast.Walk(v, n)
		}
	case *ast.FuncDecl:
		if n.Recv != nil {
			ast.Walk(v, n.Recv)
		}
		v.add(anchorAnnotation, "", n.Pos())
		ast.Walk(v, n.Type)
	case *ast.Field:
		for _ = range n.Names {
			v.ignoreName()
		}
		ast.Walk(v, n.Type)
	case *ast.ValueSpec:
		for _ = range n.Names {
			v.add(anchorAnnotation, "", 0)
		}
		if n.Type != nil {
			ast.Walk(v, n.Type)
		}
		for _, x := range n.Values {
			ast.Walk(v, x)
		}
	case *ast.Ident:
		switch {
		case n.Obj == nil && predeclared[n.Name] != notPredeclared:
			v.add(builtinAnnotation, "", 0)
		case n.Obj != nil && ast.IsExported(n.Name):
			v.add(linkAnnotation, "", 0)
		default:
			v.ignoreName()
		}
	case *ast.SelectorExpr:
		if x, _ := n.X.(*ast.Ident); x != nil {
			if obj := x.Obj; obj != nil && obj.Kind == ast.Pkg {
				if spec, _ := obj.Decl.(*ast.ImportSpec); spec != nil {
					if path, err := strconv.Unquote(spec.Path.Value); err == nil {
						if path == "C" {
							v.ignoreName()
							v.ignoreName()
						} else if n.Sel.Pos()-x.End() == 1 {
							v.add(linkBeginAnnotation, path, 0)
							v.add(linkEndAnnotation, path, 0)
						} else {
							v.add(packageLinkAnnotation, path, 0)
							v.add(linkAnnotation, path, 0)
						}
						return nil
					}
				}
			}
		}
		ast.Walk(v, n.X)
		v.ignoreName()
	case *ast.BasicLit:
		if n.Kind == token.STRING && len(n.Value) > 128 {
			v.comments = append(v.comments,
				&ast.CommentGroup{List: []*ast.Comment{{
					Slash: n.Pos(),
					Text:  fmt.Sprintf("/* %d byte string literal not displayed */", len(n.Value)),
				}}})
			n.Value = `""`
		} else {
			return v
		}
	case *ast.CompositeLit:
		if len(n.Elts) > 100 {
			if n.Type != nil {
				ast.Walk(v, n.Type)
			}
			v.comments = append(v.comments,
				&ast.CommentGroup{List: []*ast.Comment{{
					Slash: n.Lbrace,
					Text:  fmt.Sprintf("/* %d elements not displayed */", len(n.Elts)),
				}}})
			n.Elts = n.Elts[:0]
		} else {
			return v
		}
	default:
		return v
	}
	return nil
}
