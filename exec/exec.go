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

package main

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/zombiezen/mcm/catalog"
	"github.com/zombiezen/mcm/exec/execlib"
	"github.com/zombiezen/mcm/internal/system"
	"github.com/zombiezen/mcm/internal/version"
	"github.com/zombiezen/mcm/third_party/golang/capnproto"
)

func init() {
	flag.Usage = usage
}

func usage() {
	fmt.Fprintf(os.Stderr, "usage: %s [CATALOG]:\n", filepath.Base(os.Args[0]))
	flag.PrintDefaults()
}

func main() {
	log := new(logger)
	opts := &execlib.Options{
		Log: log,
	}
	simulate := flag.Bool("n", false, "dry-run")
	flag.BoolVar(&log.quiet, "q", false, "suppress info messages and failure output")
	logCommands := flag.Bool("s", false, "show commands run in the log")
	flag.IntVar(&opts.ConcurrentJobs, "j", 1, "set the maximum number of resources to apply simultaneously")
	flag.StringVar(&opts.Bash, "bash", execlib.DefaultBashPath, "path to bash shell")
	versionMode := flag.Bool("version", false, "display version info")
	flag.Parse()
	if *versionMode {
		version.Show()
		return
	}
	var sys system.System = system.Local{}
	if *simulate {
		sys = simulatedSystem{}
	}
	if *logCommands {
		sys = sysLogger{
			System: sys,
			log:    log,
		}
	}

	ctx := context.Background()
	var cat catalog.Catalog
	switch flag.NArg() {
	case 0:
		var err error
		cat, err = readCatalog(os.Stdin)
		if err != nil {
			log.Fatal(ctx, err)
		}
	case 1:
		// TODO(someday): read segments lazily
		f, err := os.Open(flag.Arg(0))
		if err != nil {
			log.Fatal(ctx, err)
		}
		cat, err = readCatalog(f)
		if err != nil {
			log.Fatal(ctx, err)
		}
		if err = f.Close(); err != nil {
			log.Error(ctx, err)
		}
	default:
		usage()
		os.Exit(2)
	}

	if err := execlib.Apply(ctx, sys, cat, opts); err != nil {
		log.Fatal(ctx, err)
	}
}

type sysLogger struct {
	system.System
	log *logger
}

func (l sysLogger) Mkdir(ctx context.Context, path string, mode os.FileMode) error {
	l.log.Infof(ctx, "mkdir %s", path)
	return l.System.Mkdir(ctx, path, mode)
}

func (l sysLogger) Remove(ctx context.Context, path string) error {
	l.log.Infof(ctx, "rm %s", path)
	return l.System.Remove(ctx, path)
}

func (l sysLogger) Symlink(ctx context.Context, oldname, newname string) error {
	l.log.Infof(ctx, "ln -s %s %s", oldname, newname)
	return l.System.Symlink(ctx, oldname, newname)
}

func (l sysLogger) Chmod(ctx context.Context, path string, mode os.FileMode) error {
	m := uint32(mode & os.ModePerm)
	if mode&os.ModeSticky != 0 {
		m |= 01000
	}
	if mode&os.ModeSetgid != 0 {
		m |= 02000
	}
	if mode&os.ModeSetuid != 0 {
		m |= 04000
	}
	l.log.Infof(ctx, "chmod %4o %s", m, path)
	return l.System.Chmod(ctx, path, mode)
}

func (l sysLogger) Chown(ctx context.Context, path string, uid system.UID, gid system.GID) error {
	l.log.Infof(ctx, "chown %d:%d %s", uid, gid, path)
	return l.System.Chown(ctx, path, uid, gid)
}

func (l sysLogger) CreateFile(ctx context.Context, path string, mode os.FileMode) (system.FileWriter, error) {
	l.log.Infof(ctx, "create file %s", path)
	return l.System.CreateFile(ctx, path, mode)
}

func (l sysLogger) Run(ctx context.Context, cmd *system.Cmd) (output []byte, err error) {
	l.log.Infof(ctx, "exec %s", strings.Join(cmd.Args, " "))
	return l.System.Run(ctx, cmd)
}

type simulatedSystem struct{}

func (simulatedSystem) Lstat(ctx context.Context, path string) (os.FileInfo, error) {
	return system.Local{}.Lstat(ctx, path)
}

func (simulatedSystem) Readlink(ctx context.Context, path string) (string, error) {
	return system.Local{}.Readlink(ctx, path)
}

func (simulatedSystem) Mkdir(ctx context.Context, path string, mode os.FileMode) error {
	return nil
}

func (simulatedSystem) Remove(ctx context.Context, path string) error {
	return nil
}

func (simulatedSystem) Symlink(ctx context.Context, oldname, newname string) error {
	return nil
}

func (simulatedSystem) CreateFile(ctx context.Context, path string, mode os.FileMode) (system.FileWriter, error) {
	if _, err := os.Lstat(path); err == nil {
		return nil, &os.PathError{Op: "open", Path: path, Err: os.ErrExist}
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	// TODO(someday): ensure parent directory exists and is writable
	return discardWriter{}, nil
}

func (simulatedSystem) OpenFile(ctx context.Context, path string) (system.File, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	return &readOnlyFile{f: f}, nil
}

func (simulatedSystem) Chmod(ctx context.Context, path string, mode os.FileMode) error {
	return nil
}

func (simulatedSystem) Chown(ctx context.Context, path string, uid system.UID, gid system.GID) error {
	return nil
}

func (simulatedSystem) OwnerInfo(mode os.FileInfo) (system.UID, system.GID, error) {
	return (system.Local{}).OwnerInfo(mode)
}

func (simulatedSystem) LookupUser(name string) (system.UID, error) {
	return (system.Local{}).LookupUser(name)
}

func (simulatedSystem) LookupGroup(name string) (system.GID, error) {
	return (system.Local{}).LookupGroup(name)
}

func (simulatedSystem) Run(ctx context.Context, cmd *system.Cmd) (output []byte, err error) {
	return nil, nil
}

type readOnlyFile struct {
	f     *os.File
	wrote bool
}

func (ro *readOnlyFile) Read(p []byte) (int, error) {
	if ro.wrote {
		return 0, errors.New("read after simulated write")
	}
	return ro.f.Read(p)
}

func (ro *readOnlyFile) Write(p []byte) (int, error) {
	ro.wrote = true
	return len(p), nil
}

func (ro *readOnlyFile) Seek(offset int64, whence int) (int64, error) {
	if ro.wrote {
		return 0, errors.New("seek after simulated write")
	}
	return ro.f.Seek(offset, whence)
}

func (ro *readOnlyFile) Truncate(size int64) error {
	ro.wrote = true
	return nil
}

func (ro *readOnlyFile) Close() error {
	return ro.f.Close()
}

type discardWriter struct{}

func (discardWriter) Write(p []byte) (int, error) {
	return len(p), nil
}

func (discardWriter) Close() error {
	return nil
}

type logger struct {
	quiet bool
	mu    sync.Mutex
}

func (l *logger) Infof(ctx context.Context, format string, args ...interface{}) {
	if l.quiet {
		return
	}
	now := time.Now()
	var line bytes.Buffer
	writeLogHead(&line, "INFO", now)
	fmt.Fprintf(&line, format, args...)
	if b := line.Bytes(); b[len(b)-1] != '\n' {
		line.WriteByte('\n')
	}
	defer l.mu.Unlock()
	l.mu.Lock()
	os.Stderr.Write(line.Bytes())
}

func (l *logger) Error(ctx context.Context, err error) {
	now := time.Now()
	var line bytes.Buffer
	writeLogHead(&line, "ERROR", now)
	line.WriteString(err.Error())
	if b := line.Bytes(); b[len(b)-1] != '\n' {
		line.WriteByte('\n')
	}

	var output []byte
	if !l.quiet {
		if err, ok := err.(*execlib.Error); ok && len(err.Output) > 0 {
			output = err.Output
			if n := len(output); output[n-1] == '\n' {
				new := make([]byte, n+1)
				copy(new, output)
				new[n] = '\n'
				output = new
			}
			output = err.Output
			if err.Output[len(err.Output)-1] != '\n' {
				line.WriteByte('\n')
			}
		}
	}

	defer l.mu.Unlock()
	l.mu.Lock()
	os.Stderr.Write(line.Bytes())
	if len(output) > 0 {
		os.Stderr.Write(output)
	}
}

func writeLogHead(buf *bytes.Buffer, severity string, now time.Time) {
	buf.WriteString("mcm-exec: ")
	buf.WriteString(now.Format("2006-01-02T15:04:05"))
	fmt.Fprintf(buf, " %5s: ", severity)
}

func (l *logger) Fatal(ctx context.Context, err error) {
	l.Error(ctx, err)
	os.Exit(1)
}

func readCatalog(r io.Reader) (catalog.Catalog, error) {
	msg, err := capnp.NewDecoder(r).Decode()
	if err != nil {
		return catalog.Catalog{}, fmt.Errorf("read catalog: %v", err)
	}
	c, err := catalog.ReadRootCatalog(msg)
	if err != nil {
		return catalog.Catalog{}, fmt.Errorf("read catalog: %v", err)
	}
	return c, nil
}
