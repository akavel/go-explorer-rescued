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
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"
)

const textIndent = "    "
const textWidth = 80 - len(textIndent)

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
		fset:    pkg.fset,
		dpkg:    dpkg,
		bpkg:    pkg.bpkg,
		lineNum: 1,
		index:   make(map[string]int),
	}
	p.execute(out)
	return 0
}

type docPrinter struct {
	fset *token.FileSet
	dpkg *doc.Package
	bpkg *build.Package

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
	fmt.Fprintf(&p.buf, "package %s\n\n", p.dpkg.Name)
	fmt.Fprintf(&p.buf, "    import \"%s\"\n\n", p.dpkg.ImportPath)
	p.doc(p.dpkg.Doc)

	p.buf.WriteString("FILES\n")
	p.files(p.bpkg.GoFiles, p.bpkg.CgoFiles)
	p.files(p.bpkg.TestGoFiles, p.bpkg.XTestGoFiles)
	p.buf.WriteString("\n")

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
	p.dirs()

	p.metaBuf.WriteString("D\n")
	p.metaBuf.WriteTo(out)
	p.buf.WriteTo(out)
}

const (
	noAnnotation = iota
	anchorAnnotation
	packageLinkAnnoation
	linkAnnotation
	startLinkAnnotation
	endLinkAnnotation
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
			if len(v.annotations) == 0 {
				// Oops!
				break loop
			}
			offset := int(pos) - base
			p.buf.Write(buf[lastOffset:offset])
			lastOffset = offset + len(lit)
			a := v.annotations[0]
			v.annotations = v.annotations[1:]
			switch a.kind {
			case startLinkAnnotation:
				startPos = p.adjustedOutputPosition()
				p.buf.WriteString(lit)
			case linkAnnotation:
				startPos = p.adjustedOutputPosition()
				fallthrough
			case endLinkAnnotation:
				p.buf.WriteString(lit)
				file := ""
				if a.data != "" {
					file = "godoc://" + a.data
				}
				p.addLink(startPos, file, p.stringAddress(lit))
			case packageLinkAnnoation:
				startPos = p.outputPosition()
				p.buf.WriteString(lit)
				p.addLink(startPos, "godoc://"+a.data, p.stringAddress(""))
			case anchorAnnotation:
				startPos = p.outputPosition()
				p.buf.WriteString(lit)
				p.addAnchor(startPos, lit, a.data)
				position := p.fset.Position(a.pos)
				p.addLink(startPos,
					filepath.Join(p.bpkg.Dir, position.Filename),
					-p.lineColumnAddress(position.Line, position.Column))
			default:
				p.buf.WriteString(lit)
			}
		}
	}
	p.buf.Write(buf[lastOffset:])
	p.buf.WriteString("\n\n")
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
		p.addLink(startPos, filepath.Join(p.bpkg.Dir, fname), p.stringAddress(""))
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
		p.addLink(startPos, "godoc://"+imp, p.stringAddress(""))
		p.buf.WriteByte('\n')
	}
	p.buf.WriteString("\n")
}

func (p *docPrinter) dirs() {
	fis, err := ioutil.ReadDir(p.bpkg.Dir)
	if err != nil {
		return
	}

	head := false
	for _, fi := range fis {
		if !fi.IsDir() || strings.HasPrefix(fi.Name(), ".") {
			continue
		}
		if !head {
			head = true
			p.buf.WriteString("SUBDIRECTORIES\n\n")
		}
		p.buf.WriteString(textIndent)
		startPos := p.outputPosition()
		p.buf.WriteString(fi.Name())
		p.addLink(startPos, "godoc://"+p.bpkg.ImportPath+"/"+fi.Name(), p.stringAddress(""))
		p.buf.WriteByte('\n')
	}
	if head {
		p.buf.WriteString("\n")
	}
}

func (p *docPrinter) addLink(startPos int64, file string, address int64) {
	fmt.Fprintf(&p.metaBuf, "L %d %d %d %d\n", startPos, p.outputPosition(), p.stringAddress(file), address)
}

func (p *docPrinter) addAnchor(startPos int64, name, typeName string) {
	if typeName != "" {
		name = typeName + "." + name
	}
	fmt.Fprintf(&p.metaBuf, "A %d %s\n", startPos, name)
}

func (p *docPrinter) stringAddress(s string) int64 {
	if i, ok := p.index[s]; ok {
		return int64(i)
	}
	i := len(p.index)
	p.index[s] = i
	fmt.Fprintf(&p.metaBuf, "S %s\n", s)
	return int64(i)
}

func (p *docPrinter) lineColumnAddress(line, col int) int64 {
	return int64(line)*10000 + int64(col)
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
	return p.lineColumnAddress(p.lineNum, len(b)-p.lineOffset)
}

func (p *docPrinter) adjustedOutputPosition() int64 {
	b := p.buf.Bytes()
	b = bytes.TrimSuffix(b, []byte{'*'})
	b = bytes.TrimSuffix(b, []byte{'[', ']'})
	b = bytes.TrimSuffix(b, []byte{'*'})
	return p.outputPosition() - int64(p.buf.Len()-len(b))
}

func (p *docPrinter) afterStar() bool {
	b := p.buf.Bytes()
	if len(b) == 0 {
		return false
	}
	return b[len(b)-1] == '*'
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

func (v *declVisitor) addAnnoation(kind int, data string, pos token.Pos) {
	v.annotations = append(v.annotations, &annotation{kind: kind, data: data, pos: pos})
}

func (v *declVisitor) ignoreName() {
	v.annotations = append(v.annotations, &annotation{kind: noAnnotation})
}

func (v *declVisitor) Visit(n ast.Node) ast.Visitor {
	switch n := n.(type) {
	case *ast.TypeSpec:
		v.addAnnoation(anchorAnnotation, "", n.Pos())
		name := n.Name.Name
		switch n := n.Type.(type) {
		case *ast.InterfaceType:
			for _, f := range n.Methods.List {
				for _, n := range f.Names {
					v.addAnnoation(anchorAnnotation, name, n.Pos())
				}
				ast.Walk(v, f.Type)
			}
		case *ast.StructType:
			for _, f := range n.Fields.List {
				for _, n := range f.Names {
					v.addAnnoation(anchorAnnotation, name, n.Pos())
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
		v.addAnnoation(anchorAnnotation, "", n.Pos())
		ast.Walk(v, n.Type)
	case *ast.Field:
		for _ = range n.Names {
			v.ignoreName()
		}
		ast.Walk(v, n.Type)
	case *ast.ValueSpec:
		for _, n := range n.Names {
			v.addAnnoation(anchorAnnotation, "", n.Pos())
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
			v.addAnnoation(linkAnnotation, "builtin", 0)
		case n.Obj != nil && ast.IsExported(n.Name):
			v.addAnnoation(linkAnnotation, "", 0)
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
							v.addAnnoation(startLinkAnnotation, path, 0)
							v.addAnnoation(endLinkAnnotation, path, 0)
						} else {
							v.addAnnoation(packageLinkAnnoation, path, 0)
							v.addAnnoation(linkAnnotation, path, 0)
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
