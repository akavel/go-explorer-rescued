// Copyright 2015 Gary Burd. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"flag"
	"io"
	"os"
)

func init() {
	var fs flag.FlagSet
	commands["def"] = &command{
		fs: &fs,
		do: func() { os.Exit(doDef(os.Stdout, fs.Args())) },
	}
}

func doDef(out io.Writer, args []string) int {
	return 0
}
