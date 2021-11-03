package cmd

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"io/fs"
	"path"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const (
	// NOTE Assume that the remote pod run on linux
	S_IFBLK  = 0x6000
	S_IFCHR  = 0x2000
	S_IFDIR  = 0x4000
	S_IFIFO  = 0x1000
	S_IFLNK  = 0xa000
	S_IFMT   = 0xf000
	S_IFREG  = 0x8000
	S_IFSOCK = 0xc000
)

type PodDirEntry struct {
	name string
	mode fs.FileMode
	fs   fs.StatFS
}

func (e *PodDirEntry) Name() string               { return e.name }
func (e *PodDirEntry) IsDir() bool                { return e.mode.IsDir() }
func (e *PodDirEntry) Type() fs.FileMode          { return e.mode }
func (e *PodDirEntry) Info() (fs.FileInfo, error) { return e.fs.Stat(e.name) }

type PodFS struct {
	Executor Executor
	Pwd      string
}

func (f *PodFS) Open(name string) (fs.File, error) {
	content, err := f.Executor.RunRead(context.TODO(), []string{
		"cat",
		path.Join(f.Pwd, name),
	})

	if err != nil {
		return nil, err
	}
	file := PodFile{
		name:    name,
		fs:      f,
		content: content,
	}
	return &file, nil
}

func (f *PodFS) ReadDir(name string) ([]fs.DirEntry, error) {
	p := path.Join(f.Pwd, name)
	inf, err := f.Stat(name)
	if err != nil {
		return nil, err
	}
	if !inf.IsDir() {
		return nil, &fs.PathError{Op: "readdirent", Path: p, Err: syscall.ENOTDIR}
	}
	output, err := f.Executor.Run(context.TODO(), []string{
		"ls",
		"-A",
		p,
	})
	if err != nil {
		return nil, err
	}
	files := strings.Split(string(output), "\n")
	files = files[0 : len(files)-1]

	subdir, err := f.Sub(name)
	if err != nil {
		return nil, err
	}
	entries := make([]fs.DirEntry, len(files))
	for i, file := range files {
		inf, err := fs.Stat(subdir, file)
		if err != nil {
			// TODO handle file not found
			return nil, err
		}
		entries[i] = &PodDirEntry{
			name: file,
			mode: inf.Mode(),
		}
	}
	return entries, nil
}

type LinuxStat_t struct {
	Dev     uint64
	Ino     uint64
	Nlink   uint64
	Mode    uint32
	Uid     uint32
	Gid     uint32
	Rdev    uint64
	Size    int64
	Blksize int64
	Blocks  int64
	Atim    syscall.Timespec
	Mtim    syscall.Timespec
	Ctim    syscall.Timespec
}

func (f *PodFS) Stat(name string) (fs.FileInfo, error) {
	// TODO handle a symlink file
	output, err := f.Executor.Run(context.TODO(), []string{
		"stat",
		"--format",
		strings.Join([]string{"%n", "%i", "%s", "%B", "%b", "%f", "%X", "%Y", "%Z", "%u", "%g"}, "\t"),
		path.Join(f.Pwd, name),
	})
	if err != nil {
		return nil, err
	}
	output = output[:len(output)-1] // trim a trailing new-line
	parts := bytes.Split(output, []byte{'\t'})
	if len(parts) != 11 {
		return nil, fmt.Errorf("unexpected stat output: %s", output)
	}
	ino, err := strconv.ParseUint(string(parts[1]), 10, 64)
	if err != nil {
		return nil, err
	}
	size, err := strconv.ParseInt(string(parts[2]), 10, 64)
	if err != nil {
		return nil, err
	}
	blksize, err := strconv.ParseInt(string(parts[3]), 10, 32)
	if err != nil {
		return nil, err
	}
	blocks, err := strconv.ParseInt(string(parts[4]), 10, 64)
	if err != nil {
		return nil, err
	}
	rawmode, err := strconv.ParseUint(string(parts[5]), 16, 32)
	if err != nil {
		return nil, err
	}
	atime, err := strconv.ParseInt(string(parts[6]), 10, 64)
	if err != nil {
		return nil, err
	}
	mtime, err := strconv.ParseInt(string(parts[7]), 10, 64)
	if err != nil {
		return nil, err
	}
	ctime, err := strconv.ParseInt(string(parts[8]), 10, 64)
	if err != nil {
		return nil, err
	}
	uid, err := strconv.ParseUint(string(parts[9]), 10, 32)
	if err != nil {
		return nil, err
	}
	gid, err := strconv.ParseUint(string(parts[10]), 10, 32)
	if err != nil {
		return nil, err
	}
	mode := fs.FileMode(rawmode & 0777)
	switch rawmode & S_IFMT {
	case S_IFBLK:
		mode |= fs.ModeDevice
	case S_IFCHR:
		mode |= fs.ModeDevice | fs.ModeCharDevice
	case S_IFDIR:
		mode |= fs.ModeDir
	case S_IFIFO:
		mode |= fs.ModeNamedPipe
	case S_IFLNK:
		mode |= fs.ModeSymlink
	case S_IFREG:
		// nothing to do
	case S_IFSOCK:
		mode |= fs.ModeSocket
	}
	inf := &PodFileInfo{
		name: path.Base(string(parts[0])),
		mode: mode,
		sys: LinuxStat_t{
			Ino:     ino,
			Mode:    uint32(rawmode),
			Uid:     uint32(uid),
			Gid:     uint32(gid),
			Size:    size,
			Blksize: blksize,
			Blocks:  blocks,
			Atim:    syscall.Timespec{Sec: atime},
			Mtim:    syscall.Timespec{Sec: mtime},
			Ctim:    syscall.Timespec{Sec: ctime},
		},
	}
	return inf, nil
}

func (f *PodFS) Sub(dir string) (fs.FS, error) {
	return &PodFS{
		Executor: f.Executor,
		Pwd:      path.Join(f.Pwd, dir),
	}, nil
}

type PodFile struct {
	name    string
	fs      fs.StatFS
	content io.ReadCloser
}

func (f *PodFile) Stat() (fs.FileInfo, error) {
	return f.fs.Stat(f.name)
}

func (f *PodFile) Read(b []byte) (int, error) {
	return f.content.Read(b)
}

func (f *PodFile) Close() error {
	return f.content.Close()
}

type PodFileInfo struct {
	name string
	size int64
	mode fs.FileMode
	sys  LinuxStat_t
}

func (i *PodFileInfo) Name() string       { return i.name }
func (i *PodFileInfo) Size() int64        { return i.size }
func (i *PodFileInfo) Mode() fs.FileMode  { return i.mode }
func (i *PodFileInfo) ModTime() time.Time { return time.Unix(i.sys.Mtim.Sec, i.sys.Mtim.Nsec) }
func (i *PodFileInfo) IsDir() bool        { return i.mode.IsDir() }
func (i *PodFileInfo) Sys() interface{}   { return &i.sys }
