// Copyright 2015 Gary Burd. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"strings"
	"testing"
)

const completeTestFile = `
package main

import (
    p1 "github.com/user/repo1"
    "github.com/user/repo2"
    "github.com/user/repo3"
)
`

var completeTests = []struct {
	in  string
	out string
}{
	{"", "p1\nrepo2\nrepo3"},
	{"repo", "repo2\nrepo3"},

	// The following tests depend on directories in the developer's workspace.
	{"/github.com", "/github.com/"},
	{"/go/", "/go/ast/\n/go/build/\n/go/doc/\n/go/format/\n/go/parser/\n/go/printer/\n/go/scanner/\n/go/token/"},
}

func TestComplete(t *testing.T) {
	for _, tt := range completeTests {
		var buf bytes.Buffer
		doComplete(&buf, strings.NewReader(completeTestFile), []string{tt.in, "x " + tt.in, ""})
		out := buf.String()
		if out != tt.out {
			t.Errorf("reseolve(%q) = %q, want %q", tt.in, out, tt.out)
		}
	}
}

var resolveTests = []struct {
	in  string
	out string
}{
	{"p1", "github.com/user/repo1"},
	{"repo2", "github.com/user/repo2"},
	{"/github.com/user/repo3", "github.com/user/repo3"},
}

func TestResolve(t *testing.T) {
	for _, tt := range resolveTests {
		var buf bytes.Buffer
		doResolve(&buf, strings.NewReader(completeTestFile), []string{tt.in})
		out := buf.String()
		if out != tt.out {
			t.Errorf("reseolve(%q) = %q, want %q", tt.in, out, tt.out)
		}
	}
}
