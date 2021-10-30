package cmd

import (
	"context"
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
	if f != nil {
		return f.(fusefs.FileGetattrer).Getattr(ctx, out)
	}
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
