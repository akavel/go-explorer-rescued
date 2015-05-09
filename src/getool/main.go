// Copyright 2015 Gary Burd. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"sort"
	"strings"
)

type command struct {
	fs *flag.FlagSet
	do func()
}

var (
	commands = map[string]*command{}
	logFile  = flag.String("log", "", "write error output to `file`")
	cwd      = flag.String("cwd", ".", "resolve relative paths from `dir`")
)

func main() {
	log.SetFlags(0)

	flag.Usage = printUsage
	flag.Parse()

	if *logFile != "" {
		f, err := os.Create(*logFile)
		if err != nil {
			log.Fatal(err)
		}
		os.Stderr = f
		log.SetOutput(f)
		defer f.Close()
	}

	args := flag.Args()
	if len(args) >= 1 {
		if c, ok := commands[args[0]]; ok {
			c.fs.Usage = func() {
				c.fs.PrintDefaults()
				os.Exit(1)
			}
			c.fs.SetOutput(os.Stderr)
			c.fs.Parse(args[1:])
			c.do()
			return
		}
	}
	log.Fatal("unknown command")
}

func printUsage() {
	var names []string
	for name, _ := range commands {
		names = append(names, name)
	}
	sort.Strings(names)
	fmt.Fprintf(os.Stderr, "%s %s\n", os.Args[0], strings.Join(names, "|"))
	flag.PrintDefaults()
}
