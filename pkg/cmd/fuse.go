package cmd

import (
	"context"
	"io"
	"io/fs"
	"syscall"

	fusefs "github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"
)

type PodFuseNode struct {
	fusefs.Inode

	file string
	fsys fs.FS
}

var _ = (fusefs.NodeReaddirer)((*PodFuseNode)(nil))
var _ = (fusefs.NodeLookuper)((*PodFuseNode)(nil))
var _ = (fusefs.NodeGetattrer)((*PodFuseNode)(nil))
var _ = (fusefs.NodeOpener)((*PodFuseNode)(nil))
var _ = (fusefs.NodeReader)((*PodFuseNode)(nil))
var _ = (fusefs.NodeReleaser)((*PodFuseNode)(nil))
var _ = (fusefs.NodeReadlinker)((*PodFuseNode)(nil))

func (n *PodFuseNode) Readdir(ctx context.Context) (fusefs.DirStream, syscall.Errno) {
	es, err := fs.ReadDir(n.fsys, ".")
	if err != nil {
		return nil, fusefs.ToErrno(err)
	}
	entries := make([]fuse.DirEntry, len(es))
	for i, e := range es {
		mode := uint32(e.Type().Perm())
		if e.IsDir() {
			mode |= fuse.S_IFDIR
		}
		if e.Type()&fs.ModeSymlink == fs.ModeSymlink {
			mode |= fuse.S_IFLNK
		}
		entries[i] = fuse.DirEntry{
			Mode: mode,
			Name: e.Name(),
		}
	}
	return fusefs.NewListDirStream(entries), 0
}

func (n *PodFuseNode) Lookup(ctx context.Context, name string, out *fuse.EntryOut) (*fusefs.Inode, syscall.Errno) {
	inf, err := fs.Stat(n.fsys, name)
	if err != nil {
		return nil, fusefs.ToErrno(err)
	}

	var attr fusefs.StableAttr
	if stat, ok := inf.Sys().(*LinuxStat_t); ok {
		if stat.Ino == 1 {
			return nil, syscall.EPERM
		}
		attr.Ino = stat.Ino
		attr.Mode = stat.Mode
	}
	var node *PodFuseNode
	if inf.IsDir() {
		subfs, err := fs.Sub(n.fsys, name)
		if err != nil {
			return nil, fusefs.ToErrno(err)
		}
		node = &PodFuseNode{fsys: subfs}
	} else {
		node = &PodFuseNode{
			fsys: n.fsys,
			file: name,
		}
	}
	ch := n.NewInode(ctx, node, attr)
	return ch, fusefs.OK
}

func (n *PodFuseNode) Getattr(ctx context.Context, f fusefs.FileHandle, out *fuse.AttrOut) syscall.Errno {
	inf, err := fs.Stat(n.fsys, n.file)
	if err != nil {
		return fusefs.ToErrno(err)
	}

	if stat, ok := inf.Sys().(*LinuxStat_t); ok {
		out.Ino = stat.Ino
		out.Mode = stat.Mode
		out.Size = uint64(stat.Size)
		out.Blocks = uint64(stat.Blocks)
		out.Atime = uint64(stat.Atim.Sec)
		out.Atimensec = uint32(stat.Atim.Nsec)
		out.Mtime = uint64(stat.Mtim.Sec)
		out.Mtimensec = uint32(stat.Mtim.Nsec)
		out.Ctime = uint64(stat.Ctim.Sec)
		out.Ctimensec = uint32(stat.Ctim.Nsec)
		out.Nlink = uint32(stat.Nlink)
		out.Uid = uint32(stat.Uid)
		out.Gid = uint32(stat.Gid)
		out.Rdev = uint32(stat.Rdev)
	}
	return fusefs.OK
}

func (f *PodFuseNode) Open(ctx context.Context, flags uint32) (fh fusefs.FileHandle, fuseFlags uint32, errno syscall.Errno) {
	src, err := f.fsys.Open(f.file)
	if err != nil {
		return nil, 0, fusefs.ToErrno(err)
	}
	return src, fuse.FOPEN_NONSEEKABLE, fusefs.OK
}

func (f *PodFuseNode) Read(ctx context.Context, h fusefs.FileHandle, dest []byte, off int64) (fuse.ReadResult, syscall.Errno) {
	r := h.(io.ReadCloser)

	_, err := io.CopyN(io.Discard, r, off)
	if err != nil && err != io.EOF {
		if err == io.EOF {
			r.Close()
		}
		return nil, fusefs.ToErrno(err)
	}

	_, err = r.Read(dest)
	if err != nil && err != io.EOF {
		return nil, fusefs.ToErrno(err)
	}
	return fuse.ReadResultData(dest), fusefs.OK
}

func (f *PodFuseNode) Readlink(ctx context.Context) ([]byte, syscall.Errno) {
	link, err := Readlink(f.fsys, f.file)
	return []byte(link), fusefs.ToErrno(err)
}

func (f *PodFuseNode) Release(ctx context.Context, h fusefs.FileHandle) syscall.Errno {
	r := h.(io.ReadCloser)
	err := r.Close()
	if err != nil {
		return fusefs.ToErrno(err)
	}
	return fusefs.OK
}
