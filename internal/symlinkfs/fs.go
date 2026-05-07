package symlinkfs

import (
	"context"
	"path/filepath"
	"syscall"

	"github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"
	"github.com/johnyoonh/symlink-logger/internal/logger"
)

type Node struct {
	fs.LoopbackNode
	logger *logger.Logger
	root   *fs.Inode
}

func NewRoot(targetPath string, accessLogger *logger.Logger) (fs.InodeEmbedder, error) {
	var st syscall.Stat_t
	if err := syscall.Stat(targetPath, &st); err != nil {
		return nil, err
	}

	rootData := &fs.LoopbackRoot{
		Path: targetPath,
		Dev:  uint64(st.Dev),
	}
	rootData.NewNode = func(root *fs.LoopbackRoot, parent *fs.Inode, name string, _ *syscall.Stat_t) fs.InodeEmbedder {
		return &Node{
			LoopbackNode: fs.LoopbackNode{RootData: root},
			logger:       accessLogger,
		}
	}

	root := &Node{
		LoopbackNode: fs.LoopbackNode{RootData: rootData},
		logger:       accessLogger,
	}
	rootData.RootNode = root
	return root, nil
}

func (n *Node) OnAdd(ctx context.Context) {
	if n.root == nil {
		n.root = n.LoopbackNode.RootData.RootNode.EmbeddedInode()
	}
}

func (n *Node) relative() string {
	root := n.root
	if root == nil && n.LoopbackNode.RootData.RootNode != nil {
		root = n.LoopbackNode.RootData.RootNode.EmbeddedInode()
	}
	if root == nil {
		return "."
	}
	rel := n.Path(root)
	if rel == "" {
		return "."
	}
	return rel
}

func (n *Node) childRelative(name string) string {
	if n.relative() == "." {
		return name
	}
	return filepath.Join(n.relative(), name)
}

func (n *Node) Lookup(ctx context.Context, name string, out *fuse.EntryOut) (*fs.Inode, syscall.Errno) {
	n.logger.Record(ctx, "lookup", n.childRelative(name), 0)
	return n.LoopbackNode.Lookup(ctx, name, out)
}

func (n *Node) Getattr(ctx context.Context, f fs.FileHandle, out *fuse.AttrOut) syscall.Errno {
	n.logger.Record(ctx, "getattr", n.relative(), 0)
	return n.LoopbackNode.Getattr(ctx, f, out)
}

func (n *Node) Open(ctx context.Context, flags uint32) (fs.FileHandle, uint32, syscall.Errno) {
	n.logger.Record(ctx, "open", n.relative(), flags)
	return n.LoopbackNode.Open(ctx, flags)
}

func (n *Node) OpendirHandle(ctx context.Context, flags uint32) (fs.FileHandle, uint32, syscall.Errno) {
	n.logger.Record(ctx, "opendir", n.relative(), flags)
	return n.LoopbackNode.OpendirHandle(ctx, flags)
}

func (n *Node) Readdir(ctx context.Context) (fs.DirStream, syscall.Errno) {
	n.logger.Record(ctx, "readdir", n.relative(), 0)
	return n.LoopbackNode.Readdir(ctx)
}

func (n *Node) Setattr(ctx context.Context, f fs.FileHandle, in *fuse.SetAttrIn, out *fuse.AttrOut) syscall.Errno {
	n.logger.Record(ctx, "setattr", n.relative(), in.Valid)
	return n.LoopbackNode.Setattr(ctx, f, in, out)
}

func (n *Node) Create(ctx context.Context, name string, flags uint32, mode uint32, out *fuse.EntryOut) (*fs.Inode, fs.FileHandle, uint32, syscall.Errno) {
	n.logger.Record(ctx, "create", n.childRelative(name), flags)
	return n.LoopbackNode.Create(ctx, name, flags, mode, out)
}

func (n *Node) Mkdir(ctx context.Context, name string, mode uint32, out *fuse.EntryOut) (*fs.Inode, syscall.Errno) {
	n.logger.Record(ctx, "mkdir", n.childRelative(name), mode)
	return n.LoopbackNode.Mkdir(ctx, name, mode, out)
}

func (n *Node) Unlink(ctx context.Context, name string) syscall.Errno {
	n.logger.Record(ctx, "unlink", n.childRelative(name), 0)
	return n.LoopbackNode.Unlink(ctx, name)
}

func (n *Node) Rmdir(ctx context.Context, name string) syscall.Errno {
	n.logger.Record(ctx, "rmdir", n.childRelative(name), 0)
	return n.LoopbackNode.Rmdir(ctx, name)
}
