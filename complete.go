// Copyright 2015 Gary Burd. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// The complete and resolve commands support three forms of package
// specifications:
//
//  /importpath - Import path with "/" prefix.
//  ./relpath   - Relative path.
//  name        - Name of imported package.
//
// The complete and resolve commands silently ignore errors. It is assumed that
// downstream uses of the command results will detect and handle errors in some
// way.

package main

import (
	"flag"
	"fmt"
	"go/ast"
	"go/build"
	"go/parser"
	"go/token"
	"io"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

func init() {
	var cfs flag.FlagSet
	commands["complete"] = &command{
		fs: &cfs,
		do: func() { os.Exit(doComplete(os.Stdout, os.Stdin, cfs.Args())) },
	}
	var rfs flag.FlagSet
	commands["resolve"] = &command{
		fs: &rfs,
		do: func() { os.Exit(doResolve(os.Stdout, os.Stdin, rfs.Args())) },
	}
}

func doComplete(out io.Writer, in io.Reader, args []string) int {
	if len(args) != 3 {
		fmt.Fprint(out, "complete: three arguments required\n")
		return 1
	}
	argLead := args[0]
	cmdLine := args[1]
	f := strings.Fields(cmdLine)
	var completions []string
	if len(f) <= 2 {
		completions = completePackage(in, argLead)
	} else {
		completions = completeID(resolvePackageSpec(in, f[1]), argLead)
	}
	io.Copy(ioutil.Discard, in)
	sort.Strings(completions)
	out.Write([]byte(strings.Join(completions, "\n")))
	return 0
}

func completePackage(in io.Reader, arg string) (completions []string) {
	switch {
	case strings.HasPrefix(arg, "."):
		// TODO
	case strings.Contains(arg, "/"):
		argDir, argFile := path.Split(arg[1:])
		for _, srcDir := range build.Default.SrcDirs() {
			fis, err := ioutil.ReadDir(filepath.Join(srcDir, filepath.FromSlash(argDir)))
			if err != nil {
				continue
			}
			for _, fi := range fis {
				if !fi.IsDir() || strings.HasPrefix(fi.Name(), ".") {
					continue
				}
				if strings.HasPrefix(fi.Name(), argFile) {
					completions = append(completions, path.Join("/", argDir, fi.Name())+"/")
				}
			}
		}
	default:
		for n := range readImports(in) {
			if strings.HasPrefix(n, arg) {
				completions = append(completions, n)
			}
		}
	}
	return completions
}

func completeID(importPath string, arg string) (completions []string) {
	return completions
}

func resolvePackageSpec(in io.Reader, spec string) string {
	path := spec
	switch {
	case strings.HasPrefix(spec, "."):
		if pkg, err := build.Import(spec, *cwd, build.FindOnly); err != nil {
			path = pkg.ImportPath
		}
	case strings.Contains(spec, "/"):
		path = strings.Trim(spec, "/ \t\n")
	default:
		if p, ok := readImports(in)[spec]; ok {
			path = p
		}
	}
	return path
}

func doResolve(out io.Writer, in io.Reader, args []string) int {
	if len(args) != 1 {
		fmt.Fprint(out, "resolve: one argument required\n")
		return 1
	}
	path := resolvePackageSpec(in, args[0])
	io.Copy(ioutil.Discard, in)
	io.WriteString(out, path)
	return 0
}

// readImports returns the imports in the Go source file read from r. Errors
// are silently ignored.
func readImports(r io.Reader) map[string]string {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "", r, parser.ImportsOnly)
	if err != nil {
		return nil
	}
	paths := map[string]string{}
	set := map[string]bool{}
	for _, decl := range file.Decls {
		d, ok := decl.(*ast.GenDecl)
		if !ok {
			continue
		}
		for _, dspec := range d.Specs {
			spec, ok := dspec.(*ast.ImportSpec)
			if !ok || spec.Path == nil {
				continue
			}
			quoted := spec.Path.Value
			path, err := strconv.Unquote(quoted)
			if err != nil || path == "C" {
				continue
			}
			if spec.Name != nil {
				paths[spec.Name.Name] = path
				set[spec.Name.Name] = true
			} else {
				name := guessNameFromPath(path)
				if !set[path] {
					paths[name] = path
				}
			}
		}
	}
	return paths
}
