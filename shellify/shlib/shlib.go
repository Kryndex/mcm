// Copyright 2016 The Minimal Configuration Manager Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package shlib

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"strconv"

	"github.com/zombiezen/mcm/catalog"
	"github.com/zombiezen/mcm/internal/depgraph"
)

// WriteScript converts a catalog into a bash script and writes it to w.
func WriteScript(w io.Writer, c catalog.Catalog) error {
	res, _ := c.Resources()
	graph, err := depgraph.New(res)
	if err != nil {
		return err
	}
	g := &gen{ew: errWriter{w: w}}
	g.p(script("#!/bin/bash"))
	g.p(script("_() {"))
	g.in()
	g.p(script("set -e"))
	for g.ew.err == nil && !graph.Done() {
		ready := append([]uint64(nil), graph.Ready()...)
		if len(ready) == 0 {
			return errors.New("graph not done, but has nothing to do")
		}
		for _, id := range ready {
			if err := g.resource(graph.Resource(id)); err != nil {
				return fmt.Errorf("resource ID=%d: %v", id, err)
			}
			graph.Mark(id)
		}
	}
	g.out()
	g.p(script("}"))
	g.p(script(`_ "$0" "$@"`))
	return g.ew.err
}

type gen struct {
	ew     errWriter
	indent int
}

func (g *gen) p(args ...interface{}) {
	if len(args) == 0 {
		g.ew.Write([]byte{'\n'})
		return
	}
	var buf bytes.Buffer
	for i := 0; i < g.indent; i++ {
		buf.WriteString("  ")
	}
	for _, a := range args {
		switch a := a.(type) {
		case string:
			buf.WriteString(shellQuote(a))
		case script:
			buf.WriteString(string(a))
		case uint64:
			buf.WriteString(strconv.FormatUint(a, 10))
		default:
			panic(fmt.Errorf("unknown type: %T", a))
		}
	}
	buf.WriteByte('\n')
	buf.WriteTo(&g.ew)
}

func (g *gen) in()  { g.indent++ }
func (g *gen) out() { g.indent-- }

func (g *gen) resource(r catalog.Resource) error {
	g.p()
	if c, _ := r.Comment(); c != "" {
		g.p(script("# "), script(c))
	} else {
		g.p(script("# Resource ID="), r.ID())
	}
	f, _ := r.File()
	path, err := f.Path()
	if err != nil {
		return fmt.Errorf("reading file path: %v", err)
	} else if path == "" {
		return errors.New("file path is empty")
	}
	switch f.Which() {
	case catalog.File_Which_plain:
		// TODO(soon): touch, even if no content
		// TODO(soon): respect file mode
		if f.Plain().HasContent() {
			g.p(script("base64 -d > "), path, script(" <<!EOF!"))
			content, _ := f.Plain().Content()
			enc := base64.NewEncoder(base64.StdEncoding, &g.ew)
			enc.Write(content)
			enc.Close()
			g.ew.WriteString("\n!EOF!\n")
		}
	case catalog.File_Which_directory:
		// TODO(soon): respect file mode
		g.p(script("if [[ ! -d "), path, script(" ]]; then"))
		g.in()
		g.p(script("mkdir "), path)
		g.out()
		g.p(script("fi"))
	default:
		return fmt.Errorf("unsupported file directive %v", f.Which())
	}
	return nil
}

type errWriter struct {
	w   io.Writer
	err error
}

func (ew *errWriter) Write(p []byte) (n int, err error) {
	if ew.err != nil {
		return 0, ew.err
	}
	n, ew.err = ew.w.Write(p)
	return n, ew.err
}

func (ew *errWriter) WriteString(s string) (n int, err error) {
	if ew.err != nil {
		return 0, ew.err
	}
	n, ew.err = io.WriteString(ew.w, s)
	return n, ew.err
}

// script is properly escaped bash.
type script string

func (s script) String() string {
	return string(s)
}

func shellQuote(s string) string {
	if s == "" {
		return "''"
	}
	safe := true
	for i := 0; i < len(s); i++ {
		if !isShellSafe(s[i]) {
			safe = false
			break
		}
	}
	if safe {
		return s
	}
	buf := make([]byte, 0, len(s)+2)
	buf = append(buf, '\'')
	for i := 0; i < len(s); i++ {
		if s[i] == '\'' {
			buf = append(buf, '\'', '\\', '\'', '\'')
		} else {
			buf = append(buf, s[i])
		}
	}
	buf = append(buf, '\'')
	return string(buf)
}

func isShellSafe(b byte) bool {
	return b >= 'A' && b <= 'Z' || b >= 'a' && b <= 'z' || b >= '0' && b <= '9' || b == '-' || b == '_' || b == '/'
}
