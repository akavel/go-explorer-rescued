// Copyright 2015 Gary Burd. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bufio"
	"bytes"
	"flag"
	"fmt"
	"go/format"
	"go/parser"
	"go/token"
	"io"
	"io/ioutil"
	"os"

	"golang.org/x/tools/imports"
)

func init() {
	var fs flag.FlagSet
	fmtonly := fs.Bool("fmtonly", false, "use gofmt instead of goimports")
	commands["fmt"] = &command{
		fs: &fs,
		do: func() { doFormat(os.Stdout, os.Stdin, fs.Args(), *fmtonly) },
	}
}

func doFormat(wunbuf io.Writer, r io.Reader, args []string, fmtonly bool) {
	w := bufio.NewWriter(wunbuf)
	defer w.Flush()

	fname := ""
	if len(args) == 1 {
		fname = args[0]
	}

	in, err := ioutil.ReadAll(r)
	if err != nil {
		fmt.Fprintf(w, "ERR\n%s", err)
		return
	}

	var out []byte

	if fmtonly {
		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, fname, in, parser.ParseComments)
		if err != nil {
			fmt.Fprintf(w, "ERR\n%s", err)
			return
		}
		var buf bytes.Buffer
		if err := format.Node(&buf, fset, file); err != nil {
			fmt.Fprintf(w, "ERR\n%s", err)
			return
		}
		out = buf.Bytes()
	} else {
		var err error
		out, err = imports.Process(fname, in, nil)
		if err != nil {
			fmt.Fprintf(w, "ERR\n%s", err)
			return
		}
	}

	// Input does not contain trailing newline, trim trailing newline from
	// output to match.
	if len(out) > 0 && out[len(out)-1] == '\n' {
		out = out[:len(out)-1]
	}

	if bytes.Equal(in, out) {
		fmt.Fprintf(w, "OK")
		return
	}

	linesIn := bytes.Split(in, []byte{'\n'})
	linesOut := bytes.Split(out, []byte{'\n'})

	n := len(linesOut)
	if len(linesIn) < len(linesOut) {
		n = len(linesIn)
	}

	head := 0
	for ; head < n; head++ {
		if !bytes.Equal(linesIn[head], linesOut[head]) {
			break
		}
	}

	n -= head

	tail := 0
	for ; tail < n; tail++ {
		if !bytes.Equal(linesIn[len(linesIn)-1-tail], linesOut[len(linesOut)-1-tail]) {
			break
		}
	}

	fmt.Fprintf(w, "REPL %d %d", head+1, len(linesIn)-tail)
	for _, l := range linesOut[head : len(linesOut)-tail] {
		fmt.Fprintf(w, "\n%s", l)
	}
}
