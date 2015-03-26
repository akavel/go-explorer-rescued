// Copyright 2015 Gary Burd. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"errors"
	"go/ast"
	"go/build"
	"go/doc"
	"go/parser"
	"go/token"
	"io/ioutil"
	"path/filepath"
	"regexp"
)

type builder struct {
	fset     *token.FileSet
	examples []*doc.Example
	pkg      *Package
}

var linePat = regexp.MustCompile(`(?m)^//line .*$`)

func (b *builder) parseFile(name string) (*ast.File, error) {
	p, err := ioutil.ReadFile(filepath.Join(b.pkg.BPkg.Dir, name))
	if err != nil {
		return nil, err
	}
	// overwrite //line comments
	for _, m := range linePat.FindAllIndex(p, -1) {
		for i := m[0] + 2; i < m[1]; i++ {
			p[i] = ' '
		}
	}
	return parser.ParseFile(b.pkg.FSet, name, p, parser.ParseComments)
}

func simpleImporter(imports map[string]*ast.Object, path string) (*ast.Object, error) {
	pkg := imports[path]
	if pkg != nil {
		return pkg, nil
	}

	n := guessNameFromPath(path)
	if n == "" {
		return nil, errors.New("package not found")
	}

	pkg = ast.NewObj(ast.Pkg, n)
	pkg.Data = ast.NewScope(nil)
	imports[path] = pkg
	return pkg, nil
}

type Package struct {
	FSet *token.FileSet

	BPkg *build.Package

	APkg *ast.Package

	// Errors found when fetching or parsing this package.
	Errors []error
}

func loadPackage(importPath string) (*Package, error) {
	var err error
	var pkg Package
	b := builder{pkg: &pkg}

	pkg.FSet = token.NewFileSet()
	pkg.BPkg, err = build.Import(importPath, *cwd, 0)
	if err != nil {
		return nil, err
	}

	files := make(map[string]*ast.File)
	for _, name := range append(pkg.BPkg.GoFiles, pkg.BPkg.CgoFiles...) {
		file, err := b.parseFile(name)
		if err != nil {
			pkg.Errors = append(pkg.Errors, err)
			continue
		}
		files[name] = file
	}

	pkg.APkg, _ = ast.NewPackage(pkg.FSet, files, simpleImporter, nil)

	for _, name := range append(pkg.BPkg.TestGoFiles, pkg.BPkg.XTestGoFiles...) {
		file, err := b.parseFile(name)
		if err != nil {
			pkg.Errors = append(pkg.Errors, err)
			continue
		}
		b.examples = append(b.examples, doc.Examples(file)...)
	}

	return &pkg, nil
}
