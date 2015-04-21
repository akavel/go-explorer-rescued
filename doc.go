// Copyright 2015 Gary Burd. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"flag"
	"fmt"
	"go/ast"
	"go/build"
	"go/doc"
	"go/printer"
	"go/scanner"
	"go/token"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"
)

const textWidth = 80
const textIndent = "    "

func init() {
	var fs flag.FlagSet
	hasConceal := fs.Bool("has_conceal", false, "")
	commands["doc"] = &command{
		fs: &fs,
		do: func() { os.Exit(doDoc(os.Stdout, fs.Args(), *hasConceal)) },
	}
}

func doDoc(out io.Writer, args []string, hasConceal bool) int {
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
	if pkg.bpkg.ImportPath == "builtin" {
		mode |= doc.AllDecls
	}

	dpkg := doc.New(pkg.apkg, pkg.bpkg.ImportPath, mode)

	if pkg.bpkg.ImportPath == "builtin" {
		for _, t := range dpkg.Types {
			dpkg.Funcs = append(dpkg.Funcs, t.Funcs...)
			t.Funcs = nil
		}
		sort.Sort(byFuncName(dpkg.Funcs))
	}

	p := docPrinter{
		fset:       pkg.fset,
		dpkg:       dpkg,
		bpkg:       pkg.bpkg,
		hasConceal: hasConceal,
		lineNum:    1,
		index:      make(map[string]int),
	}
	p.execute(out)
	return 0
}

type docPrinter struct {
	fset       *token.FileSet
	dpkg       *doc.Package
	bpkg       *build.Package
	hasConceal bool

	// Output buffers
	buf     bytes.Buffer
	metaBuf bytes.Buffer

	index map[string]int

	// Fields used by outputPosition
	lineNum    int
	lineOffset int
	scanOffset int
}

func (p *docPrinter) execute(out io.Writer) {
	fmt.Fprintf(&p.buf, "package %s\n_\n", p.dpkg.Name)
	fmt.Fprintf(&p.buf, "    import \"%s\"\n\n", p.dpkg.ImportPath)
	p.doc(p.dpkg.Doc)

	p.buf.WriteString("FILES\n")
	p.files(p.bpkg.GoFiles, p.bpkg.CgoFiles)
	p.files(p.bpkg.TestGoFiles, p.bpkg.XTestGoFiles)
	p.buf.WriteString("_\n")

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

	p.imports()

	p.metaBuf.WriteString("D\n")
	p.metaBuf.WriteTo(out)
	p.buf.WriteTo(out)
}

const (
	noLink = iota
	sourceLink
	packageLink
	docLink
	beginDocLink
	endDocLink
)

type link struct {
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
		p.buf.WriteString(err.Error())
		return
	}
	buf := bytes.TrimRight(w.Bytes(), " \t\n")

	var s scanner.Scanner
	fset := token.NewFileSet()
	file := fset.AddFile("", fset.Base(), len(buf))
	base := file.Base()
	s.Init(file, buf, nil, scanner.ScanComments)
	lastOffset := 0
	var startPos int64
loop:
	for {
		pos, tok, lit := s.Scan()
		switch tok {
		case token.EOF:
			break loop
		case token.IDENT:
			if len(v.links) == 0 {
				// Oops!
				break loop
			}
			offset := int(pos) - base
			p.buf.Write(buf[lastOffset:offset])
			lastOffset = offset + len(lit)
			a := v.links[0]
			v.links = v.links[1:]
			switch a.kind {
			case beginDocLink:
				startPos = p.outputPosition()
				p.buf.WriteByte('|')
				p.buf.WriteString(lit)
			case docLink:
				startPos = p.outputPosition()
				p.buf.WriteByte('|')
				fallthrough
			case endDocLink:
				p.buf.WriteString(lit)
				p.buf.WriteByte('|')
				file := ""
				if a.data != "" {
					file = "godoc://" + a.data
				}
				p.addTag(startPos, file, p.stringIndex(lit))
			case packageLink:
				startPos = p.outputPosition()
				p.buf.WriteByte('|')
				p.buf.WriteString(lit)
				p.buf.WriteByte('|')
				p.addTag(startPos, "godoc://"+a.data, p.stringIndex("0"))
			case sourceLink:
				startPos = p.outputPosition()
				p.buf.WriteByte('|')
				p.buf.WriteString(lit)
				p.buf.WriteByte('|')
				position := p.fset.Position(a.pos)
				p.addTag(startPos, filepath.Join(p.bpkg.Dir, position.Filename), -position.Line)
			default:
				p.buf.WriteString(lit)
			}
		}
	}
	p.buf.Write(buf[lastOffset:])
	p.buf.WriteString("\n_\n")
}

func (p *docPrinter) doc(s string) {
	s = strings.TrimRight(s, " \t\n")
	if s != "" {
		doc.ToText(&p.buf, s, textIndent, textIndent+"   ", textWidth)
		b := p.buf.Bytes()
		if b[len(b)-1] != '\n' {
			p.buf.WriteByte('\n')
		}
		p.buf.WriteByte('\n')
	}
}

func (p *docPrinter) files(sets ...[]string) {
	var fnames []string
	for _, set := range sets {
		fnames = append(fnames, set...)
	}
	if len(fnames) == 0 {
		return
	}

	sort.Strings(fnames)

	col := 0
	p.buf.WriteByte('\n')
	p.buf.WriteString(textIndent)
	for _, fname := range fnames {
		n := utf8.RuneCountInString(fname)
		if col != 0 {
			if col+n+3 > textWidth {
				col = 0
				p.buf.WriteByte('\n')
				p.buf.WriteString(textIndent)
			} else {
				col += 1
				p.buf.WriteByte(' ')
			}
		}
		startPos := p.outputPosition()
		p.buf.WriteString(fname)
		p.addTag(startPos, filepath.Join(p.bpkg.Dir, fname), p.stringIndex(""))
		col += n + 2
	}
	p.buf.WriteString("\n")
}

func (p *docPrinter) head(title string, n int) {
	if n > 0 {
		fmt.Fprintf(&p.buf, "%s\n\n", title)
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

func (p *docPrinter) imports() {
	if len(p.bpkg.Imports) == 0 {
		return
	}
	p.buf.WriteString("IMPORTS\n\n")
	for _, imp := range p.bpkg.Imports {
		p.buf.WriteString(textIndent)
		startPos := p.outputPosition()
		p.buf.WriteString(imp)
		p.addTag(startPos, "godoc://"+imp, p.stringIndex("0"))
		p.buf.WriteByte('\n')
	}
	p.buf.WriteString("_\n")
}

func (p *docPrinter) addTag(startPos int64, file string, address int) {
	fmt.Fprintf(&p.metaBuf, "T %d %d %d %d\n", startPos, p.outputPosition(), p.stringIndex(file), address)
}

func (p *docPrinter) stringIndex(s string) int {
	if i, ok := p.index[s]; ok {
		return i
	}
	i := len(p.index)
	p.index[s] = i
	fmt.Fprintf(&p.metaBuf, "S %s\n", s)
	return i
}

func (p *docPrinter) outputPosition() int64 {
	b := p.buf.Bytes()
	for i, c := range b[p.scanOffset:] {
		if c == '\n' {
			p.lineNum += 1
			p.lineOffset = p.scanOffset + i
		}
	}
	p.scanOffset = len(b)
	return int64(p.lineNum)*10000 + int64(len(b)-p.lineOffset)
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

// declVisitor modifies a declaration AST for printing and collects links.
type declVisitor struct {
	links    []*link
	comments []*ast.CommentGroup
}

func (v *declVisitor) addLink(kind int, data string, pos token.Pos) {
	v.links = append(v.links, &link{kind: kind, data: data, pos: pos})
}

func (v *declVisitor) ignoreName() {
	v.links = append(v.links, &link{kind: noLink})
}

func (v *declVisitor) Visit(n ast.Node) ast.Visitor {
	switch n := n.(type) {
	case *ast.TypeSpec:
		v.addLink(sourceLink, "", n.Pos())
		name := n.Name.Name
		switch n := n.Type.(type) {
		case *ast.InterfaceType:
			for _, f := range n.Methods.List {
				for _, n := range f.Names {
					v.addLink(sourceLink, name, n.Pos())
				}
				ast.Walk(v, f.Type)
			}
		case *ast.StructType:
			for _, f := range n.Fields.List {
				for _, n := range f.Names {
					v.addLink(sourceLink, name, n.Pos())
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
		v.addLink(sourceLink, "", n.Pos())
		ast.Walk(v, n.Type)
	case *ast.Field:
		for _ = range n.Names {
			v.ignoreName()
		}
		ast.Walk(v, n.Type)
	case *ast.ValueSpec:
		for _, n := range n.Names {
			v.addLink(sourceLink, "", n.Pos())
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
			v.addLink(docLink, "builtin", 0)
		case n.Obj != nil && ast.IsExported(n.Name):
			v.addLink(docLink, "", 0)
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
							v.addLink(beginDocLink, path, 0)
							v.addLink(endDocLink, path, 0)
						} else {
							v.addLink(packageLink, path, 0)
							v.addLink(docLink, path, 0)
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
