package fuse

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path"
	"path/filepath"
	"runtime"
	"sort"
	"sync"
	"syscall"
	"time"

	"bazil.org/fuse"
	"bazil.org/fuse/fs"
	"golang.org/x/sys/unix"

	"c2FmZQ/internal/client"
	"c2FmZQ/internal/log"
	"c2FmZQ/internal/stingle"
)

var mounts map[string]*filesys
var mountsMutex sync.Mutex

func init() {
	mounts = make(map[string]*filesys)
}

// Mount mounts the client filesystem.
func Mount(c *client.Client, mnt string) error {
	conn, err := fuse.Mount(mnt, fuse.FSName("c2FmZQ"))
	if err != nil {
		return err
	}
	defer conn.Close()
	log.Infof("Mounted %s", mnt)

	go func() {
		ch := make(chan os.Signal, 1)
		signal.Notify(ch, unix.SIGINT)
		signal.Notify(ch, unix.SIGTERM)
		sig := <-ch
		log.Infof("Received signal %d (%s)", sig, sig)
		log.Infof("Unmounting %s", mnt)
		if err := Unmount(mnt); err != nil {
			log.Errorf("fuse.Unmount(%q): %v", mnt, err)
		}
	}()

	conf := &fs.Config{}
	if log.Level > log.DebugLevel {
		conf.Debug = func(msg interface{}) {
			log.Debug("FUSE:", msg)
		}
	}
	srv := fs.New(conn, conf)
	return srv.Serve(initFS(c, srv, mnt))
}

func Unmount(mnt string) error {
	mountsMutex.Lock()
	f := mounts[mnt]
	f.mutations.Wait()
	delete(mounts, mnt)
	mountsMutex.Unlock()
	//f.debug()
	return fuse.Unmount(mnt)
}

func initFS(c *client.Client, srv *fs.Server, mnt string) *filesys {
	f := &filesys{
		c:          c,
		fuse:       srv,
		mountPoint: mnt,
		nodes:      make(map[fuse.NodeID]fs.Node),
		attrs:      make(map[string]fuse.Attr),
	}
	f.root = f.newDirNode("Node ROOT DIR")
	f.root.nodeID = fuse.RootID
	f.root.item.IsDir = true
	f.nodes[fuse.RootID] = f.root
	f.root.update()
	mountsMutex.Lock()
	mounts[mnt] = f
	mountsMutex.Unlock()
	return f
}

type filesys struct {
	c          *client.Client
	fuse       *fs.Server
	mountPoint string
	root       *dirNode

	mu    sync.Mutex
	nodes map[fuse.NodeID]fs.Node
	attrs map[string]fuse.Attr

	// Keeps track of ongoing mutations.
	mutations sync.WaitGroup
}

func (f *filesys) Root() (fs.Node, error) {
	return f.root, nil
}

func (f *filesys) debug() {
	for k, v := range f.attrs {
		log.Debugf("ATTR %s Size:%d Mode:%o Ctime:%d Mtime:%d Atime:%d", k, v.Size, v.Mode, v.Ctime.UnixNano(), v.Mtime.UnixNano(), v.Atime.UnixNano())
	}
}

func (f *filesys) setNodeID(id fuse.NodeID, n fs.Node) {
	f.mu.Lock()
	f.nodes[id] = n
	f.mu.Unlock()
}

func (f *filesys) nodeByID(id fuse.NodeID) fs.Node {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.nodes[id]
}

func (f *filesys) attr(file string) (fuse.Attr, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	a, ok := f.attrs[file]
	return a, ok
}

func (f *filesys) setAttr(file string, a fuse.Attr) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.attrs[file] = a
}

func (f *filesys) newDirNode(label string) *dirNode {
	return &dirNode{
		node:     node{label: label, f: f},
		children: make(map[string]fs.Node),
	}
}

func (f *filesys) newFileNode(label string) *fileNode {
	return &fileNode{node: node{label: label, f: f}}
}

type node struct {
	label  string
	f      *filesys
	item   client.ListItem
	nodeID fuse.NodeID

	mu sync.Mutex
}

func (n node) String() string {
	return n.label
}

func (n *node) lock() {
	log.Debugf("Lock %s", n)
	n.mu.Lock()
}

func (n *node) unlock() {
	log.Debugf("Unlock %s", n)
	n.mu.Unlock()
}

func (n *node) checkHeader(ctx context.Context, nn fs.Node, hdr fuse.Header) error {
	if hdr.Uid != uint32(os.Getuid()) {
		log.Errorf("checkHeader: UID doesn't match, %d != %d", hdr.Uid, os.Getuid())
		return syscall.EPERM
	}
	if n.nodeID == 0x0 {
		log.Debugf("Setting NodeID %v", hdr.Node)
	} else if n.nodeID != hdr.Node {
		log.Errorf("ERROR NodeID mismatch: %v != %v", n.nodeID, hdr.Node)
	}
	n.nodeID = hdr.Node
	n.f.setNodeID(n.nodeID, nn)

	select {
	case <-ctx.Done():
		return syscall.EINTR
	default:
	}
	return nil
}

var _ fs.Node = (*dirNode)(nil)
var _ fs.HandleReadDirAller = (*dirNode)(nil)
var _ fs.NodeCreater = (*dirNode)(nil)

type dirNode struct {
	node

	children map[string]fs.Node
}

var _ fs.Node = (*fileNode)(nil)

type fileNode struct {
	node
}

func (n *dirNode) child(name string) (fs.Node, bool) {
	nn, ok := n.children[name]
	return nn, ok
}

func (n *dirNode) childPath(name string) string {
	p := n.item.Filename
	if p != "" {
		p += "/"
	}
	return p + name
}

func (n *dirNode) update() {
	n.lock()
	defer n.unlock()
	n.updateLocked()
}

func caller() string {
	pc, _, _, ok := runtime.Caller(2)
	if !ok {
		return "??"
	}
	frame, _ := runtime.CallersFrames([]uintptr{pc}).Next()
	_, file := filepath.Split(frame.File)
	return fmt.Sprintf("%s:%d %s()", file, frame.Line, frame.Function)
}

func (n *dirNode) updateLocked() {
	log.Debugf("updateLocked %s START %s", n, caller())
	defer log.Debugf("updateLocked %s DONE %s", n, caller())
	li, err := n.f.c.GlobFiles([]string{n.childPath("*")}, client.GlobOptions{MatchDot: true, ExactMatchExceptLast: true, Quiet: true})
	if err != nil {
		log.Debugf("GlobFiles: %v", err)
		return
	}

	newList := make(map[string]struct{})
	for _, item := range li {
		_, name := path.Split(item.Filename)
		newList[name] = struct{}{}

		nn := n.children[name]
		if _, ok := nn.(*dirNode); ok && !item.IsDir {
			delete(n.children, name)
			nn = nil
		} else if _, ok := nn.(*fileNode); ok && item.IsDir {
			delete(n.children, name)
			nn = nil
		}
		if nn == nil {
			if item.IsDir {
				nn = n.f.newDirNode(fmt.Sprintf("Node %s DIR", name))
			} else {
				nn = n.f.newFileNode(fmt.Sprintf("Node %s", name))
			}
			n.children[name] = nn
		}
		switch v := nn.(type) {
		case *dirNode:
			v.item = item
		case *fileNode:
			size := v.item.Header.DataSize
			if v.item = item; v.item.Header.DataSize == 0 {
				v.item.Header.DataSize = size
			}
		default:
			log.Fatalf("unexpected node type: %T", nn)
		}
	}
	for name := range n.children {
		if _, ok := newList[name]; !ok {
			delete(n.children, name)
		}
	}
}

// Attr returns the node's attributes.
func (n *dirNode) Attr(ctx context.Context, a *fuse.Attr) error {
	log.Debugf("Attr called on %s", n)
	n.lock()
	defer n.unlock()
	n.attrLocked(ctx, a)
	return nil
}

func (n *dirNode) attrLocked(_ context.Context, a *fuse.Attr) error {
	log.Debugf("attrLocked called on %s", n)

	if attr, ok := n.f.attr(n.item.Filename); ok {
		*a = attr
		return nil
	}
	a.Mode = os.ModeDir | 0o700
	a.Uid = uint32(os.Getuid())
	a.Gid = uint32(os.Getgid())
	if n.item.Album != nil {
		ctime, _ := n.item.Album.DateCreated.Int64()
		a.Ctime = time.Unix(ctime/1000, ctime%1000)
		mtime, _ := n.item.Album.DateModified.Int64()
		a.Mtime = time.Unix(mtime/1000, mtime%1000)
	}
	n.f.setAttr(n.item.Filename, *a)
	return nil
}

// Setattr receives attribute changes for the directory.
func (n *dirNode) Setattr(ctx context.Context, req *fuse.SetattrRequest, resp *fuse.SetattrResponse) error {
	log.Debugf("Setattr(%s) called on %s", req.Valid, n)
	n.lock()
	defer n.unlock()
	if err := n.checkHeader(ctx, n, req.Header); err != nil {
		return err
	}
	n.attrLocked(ctx, &resp.Attr)

	if req.Valid.Size() {
		log.Debugf("New size %d", req.Size)
		resp.Attr.Size = req.Size
	}
	if req.Valid.Mode() {
		log.Debugf("New mode %o", req.Mode)
		resp.Attr.Mode = req.Mode
	}
	if req.Valid.Atime() {
		log.Debugf("New atime %d", req.Atime.UnixNano())
		resp.Attr.Atime = req.Atime
	}
	if req.Valid.Mtime() {
		log.Debugf("New mtime %d", req.Mtime.UnixNano())
		resp.Attr.Mtime = req.Mtime
	}
	n.f.setAttr(n.item.Filename, resp.Attr)
	return nil
}

// Lookup looks for a specific file in the directory.
func (n *dirNode) Lookup(ctx context.Context, req *fuse.LookupRequest, resp *fuse.LookupResponse) (fs.Node, error) {
	log.Debugf("Lookup(%q) called on %s", req.Name, n)
	n.lock()
	defer n.unlock()
	if err := n.checkHeader(ctx, n, req.Header); err != nil {
		return nil, err
	}
	if child, ok := n.child(req.Name); ok {
		if nn, ok := child.(*dirNode); ok {
			nn.update()
		}
		return child, nil
	}
	return nil, syscall.ENOENT
}

// ReadDirAll returns all the directory entries.
func (n *dirNode) ReadDirAll(context.Context) ([]fuse.Dirent, error) {
	log.Debugf("ReadDirAll() called on %s", n)
	n.lock()
	defer n.unlock()
	n.updateLocked()

	var out []fuse.Dirent
	for name, nn := range n.children {
		var t fuse.DirentType
		switch v := nn.(type) {
		case *dirNode:
			t = fuse.DT_Dir
		case *fileNode:
			t = fuse.DT_File
		default:
			log.Fatalf("unexpected node type: %T", v)
		}
		out = append(out, fuse.Dirent{
			Type: t,
			Name: name,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	log.Debugf("ReadDirAll %s", n)
	for _, i := range out {
		log.Debugf("ReadDirAll  %s %v", i.Name, i.Type)
	}
	return out, nil
}

// Poll checks whether a handle is ready for I/O.
func (node) Poll(ctx context.Context, req *fuse.PollRequest, resp *fuse.PollResponse) error {
	return syscall.ENOSYS
}

func (node) Getxattr(ctx context.Context, req *fuse.GetxattrRequest, resp *fuse.GetxattrResponse) error {
	return syscall.ENOSYS
}

// Mkdir creates a subdirectory.
func (n *dirNode) Mkdir(ctx context.Context, req *fuse.MkdirRequest) (fs.Node, error) {
	log.Debugf("Mkdir(%q) called on %s", req.Name, n)
	n.f.mutations.Add(1)
	defer n.f.mutations.Done()
	n.lock()
	defer n.unlock()
	if err := n.checkHeader(ctx, n, req.Header); err != nil {
		return nil, err
	}
	if _, ok := n.child(req.Name); ok {
		return nil, syscall.EEXIST
	}
	path := n.childPath(req.Name)
	if err := n.f.c.AddAlbums([]string{path}); err != nil {
		log.Debugf("AddAlbums(%q) failed: %v", path, err)
		return nil, syscall.EINVAL
	}
	n.updateLocked()
	if nn, ok := n.child(req.Name); ok {
		return nn, nil
	}
	return nil, syscall.EINVAL
}

// Remove deletes a node.
func (n *dirNode) Remove(ctx context.Context, req *fuse.RemoveRequest) error {
	log.Debugf("Remove(%q) called on %s", req.Name, n)
	n.f.mutations.Add(1)
	defer n.f.mutations.Done()
	n.lock()
	defer n.unlock()
	if err := n.checkHeader(ctx, n, req.Header); err != nil {
		return err
	}
	if _, ok := n.child(req.Name); !ok {
		return syscall.ENOENT
	}
	path := n.childPath(req.Name)
	if err := n.f.c.Delete([]string{path}, true); err != nil {
		log.Debugf("Delete(%q) failed: %v", path, err)
		return syscall.EINVAL
	}
	n.updateLocked()
	return nil
}

// Forget tells us this node will not receive any further requests.
func (n node) Forget() {
	log.Debugf("Forget called on %s", n)
}

// Rename changes the name of a directory entry.
func (n *dirNode) Rename(ctx context.Context, req *fuse.RenameRequest, newDir fs.Node) error {
	log.Debugf("Rename(%q -> %v / %q) called on %s", req.OldName, req.NewDir, req.NewName, n)
	defer log.Debugf("Rename returned (%q -> %v / %q) called on %s", req.OldName, req.NewDir, req.NewName, n)
	n.f.mutations.Add(1)
	defer n.f.mutations.Done()
	n.lock()
	defer n.unlock()
	if err := n.checkHeader(ctx, n, req.Header); err != nil {
		return err
	}
	if _, ok := n.child(req.OldName); !ok {
		return syscall.ENOENT
	}
	v := n.f.nodeByID(req.NewDir)
	nn, ok := v.(*dirNode)
	if !ok || nn == nil {
		log.Debugf("nodeByID(%v) returned %#v", req.NewDir, v)
		return syscall.EINVAL
	}
	src := n.childPath(req.OldName)
	dst := nn.childPath(req.NewName)
	if err := n.f.c.Move([]string{src}, dst, true); err != nil {
		log.Debugf("Move(%q, %q) failed: %v", src, dst, err)
		return syscall.EINVAL
	}
	n.updateLocked()
	return nil
}

// Create creates a new file.
func (n *dirNode) Create(ctx context.Context, req *fuse.CreateRequest, resp *fuse.CreateResponse) (fs.Node, fs.Handle, error) {
	log.Debugf("Create(%s, %s) called on %s", req.Name, req.Flags, n)
	n.f.mutations.Add(1)
	defer n.f.mutations.Done()
	n.lock()
	defer n.unlock()
	if err := n.checkHeader(ctx, n, req.Header); err != nil {
		return nil, nil, err
	}
	if !req.Flags.IsWriteOnly() && !req.Flags.IsReadWrite() {
		log.Debugf("Create can only open a file WRONLY or RDWR: %s", req.Flags)
		return nil, nil, syscall.ENOTSUP
	}
	w, err := n.f.c.FuseImport(req.Name, n.item)
	if err != nil {
		log.Errorf("FuseImport(%q, %s) failed: %v", req.Name, n, err)
		return nil, nil, syscall.EPERM
	}
	n.updateLocked()
	ch, _ := n.child(req.Name)
	nn, ok := ch.(*fileNode)
	if !ok || ch == nil {
		log.Errorf("Create: child is nil: %#v", ch)
		return nil, nil, syscall.EINVAL
	}

	h := &handle{name: nn.item.Filename, n: nn, w: w}
	if req.Flags.IsReadWrite() {

		r, err := nn.openRead()
		if err != nil {
			return nil, nil, err
		}
		h.r = r

	}
	nn.attrLocked(ctx, &resp.Attr)
	resp.Attr.Mode = req.Mode
	n.f.setAttr(nn.item.FSFile.File, resp.Attr)
	//resp.Flags = fuse.OpenDirectIO
	n.f.mutations.Add(1)
	return nn, h, nil
}

// Link creates a hard link.
func (n *dirNode) Link(ctx context.Context, req *fuse.LinkRequest, old fs.Node) (fs.Node, error) {
	log.Debugf("Link(%s, %s) called on %s", req.OldNode, req.NewName, n)
	n.f.mutations.Add(1)
	defer n.f.mutations.Done()
	n.lock()
	defer n.unlock()
	if err := n.checkHeader(ctx, n, req.Header); err != nil {
		return nil, err
	}
	nn, ok := old.(*fileNode)
	if !ok {
		log.Debugf("old node is not a file: %s", old)
		return nil, syscall.EINVAL
	}
	if _, ok := n.child(req.NewName); ok {
		return nil, syscall.EEXIST
	}
	src := nn.item.Filename
	dst := n.childPath(req.NewName)
	if err := n.f.c.Copy([]string{src}, dst, true); err != nil {
		log.Debugf("Copy(%q, %q) failed: %v", src, dst, err)
		return nil, syscall.EINVAL
	}
	n.updateLocked()
	nnn, _ := n.child(req.NewName)
	return nnn, nil
}

// Attr returns the attributes of the file.
func (n *fileNode) Attr(ctx context.Context, a *fuse.Attr) error {
	log.Debugf("Attr called on %s", n)
	n.lock()
	defer n.unlock()
	return n.attrLocked(ctx, a)
}

func (n *fileNode) attrLocked(_ context.Context, a *fuse.Attr) error {
	log.Debugf("attrLocked called on %s", n)

	if attr, ok := n.f.attr(n.item.FSFile.File); ok {
		*a = attr
		return nil
	}
	a.Mode = 0o400
	a.Uid = uint32(os.Getuid())
	a.Gid = uint32(os.Getgid())
	a.Size = uint64(n.item.Header.DataSize)
	ctime, _ := n.item.FSFile.DateCreated.Int64()
	a.Ctime = time.Unix(ctime/1000, ctime%1000)
	mtime, _ := n.item.FSFile.DateModified.Int64()
	a.Mtime = time.Unix(mtime/1000, mtime%1000)
	n.f.setAttr(n.item.FSFile.File, *a)
	return nil
}

// Setattr receives attribute changes for the file.
func (n *fileNode) Setattr(ctx context.Context, req *fuse.SetattrRequest, resp *fuse.SetattrResponse) error {
	log.Debugf("Setattr(%s) called on %s", req.Valid, n)
	n.lock()
	defer n.unlock()
	if err := n.checkHeader(ctx, n, req.Header); err != nil {
		return err
	}
	n.attrLocked(ctx, &resp.Attr)

	if req.Valid.Size() {
		log.Debugf("New size %d", req.Size)
		resp.Attr.Size = req.Size
	}
	if req.Valid.Mode() {
		log.Debugf("New mode %o", req.Mode)
		resp.Attr.Mode = req.Mode
	}
	if req.Valid.Atime() {
		log.Debugf("New atime %d", req.Atime.UnixNano())
		resp.Attr.Atime = req.Atime
	}
	if req.Valid.Mtime() {
		log.Debugf("New mtime %d", req.Mtime.UnixNano())
		resp.Attr.Mtime = req.Mtime
	}
	n.f.setAttr(n.item.FSFile.File, resp.Attr)
	return nil
}

// Open opens a file. Only reading is supported.
func (n *fileNode) Open(ctx context.Context, req *fuse.OpenRequest, resp *fuse.OpenResponse) (h fs.Handle, err error) {
	log.Debugf("Open(%s) called on %s", req.Flags, n)
	n.lock()
	defer n.unlock()
	defer func() { log.Debugf("Open(%s) returned: %p, %v", req.Flags, h, err) }()
	if err := n.checkHeader(ctx, n, req.Header); err != nil {
		return nil, err
	}
	if req.Dir {
		return nil, syscall.ENOTDIR
	}
	if !req.Flags.IsReadOnly() {
		return nil, syscall.ENOTSUP
	}
	r, err := n.openRead()
	if err != nil {
		return nil, err
	}
	h = &handle{name: n.item.Filename, n: n, r: r}
	//resp.Flags = fuse.OpenDirectIO
	return h, nil
}

func (n *fileNode) openRead() (io.ReadSeekCloser, error) {
	log.Debugf("openRead called on %s", n)
	var f io.ReadSeekCloser
	var err error
	if f, err = os.Open(n.item.FilePath); errors.Is(err, os.ErrNotExist) {
		f, err = n.f.c.DownloadGet(n.item.FSFile.File, n.item.Set)
	}
	if err != nil {
		log.Errorf("Open(%s) failed: %v", n.item.FilePath, err)
		return nil, syscall.EIO
	}
	if err := stingle.SkipHeader(f); err != nil {
		log.Errorf("SkipHeader() failed: %v", err)
		f.Close()
		return nil, syscall.EIO
	}
	return stingle.DecryptFile(f, n.item.Header), nil
}

var _ fs.HandleReleaser = (*handle)(nil)
var _ fs.HandleFlusher = (*handle)(nil)
var _ fs.HandleReader = (*handle)(nil)
var _ fs.HandleWriter = (*handle)(nil)

//var _ fs.HandlePoller = (*handle)(nil)

// handle is a file handle for reading or writing.
type handle struct {
	name string
	n    *fileNode
	r    io.ReadSeekCloser
	w    io.WriteCloser
	size int64

	mu sync.Mutex
}

func (h handle) String() string {
	mode := "-"
	if h.r != nil && h.w != nil {
		mode = "Read/Write"
	} else if h.r != nil {
		mode = "Read"
	} else if h.w != nil {
		mode = "Write"
	}
	return fmt.Sprintf("%s handle %s", mode, h.name)
}

func (h *handle) lock() {
	h.mu.Lock()
	log.Debugf("Lock %s", h)
}

func (h *handle) unlock() {
	log.Debugf("Unlock %s", h)
	h.mu.Unlock()
}

func (h *handle) checkHeader(ctx context.Context, hdr fuse.Header) error {
	return h.n.checkHeader(ctx, h.n, hdr)
}

// Release is called when the file is closed.
func (h *handle) Release(ctx context.Context, req *fuse.ReleaseRequest) error {
	h.lock()
	defer h.unlock()
	log.Debugf("Release called on %s %d", h, uint64(req.Handle))
	if h.r != nil {
		h.r.Close()
		h.r = nil
	}
	if h.w != nil {
		h.n.lock()
		var attr fuse.Attr
		h.n.attrLocked(ctx, &attr)
		attr.Size = uint64(h.size)
		h.n.f.setAttr(h.n.item.FSFile.File, attr)
		h.n.unlock()

		log.Debugf("Release starting Close %s", h)
		h.w.Close()
		h.w = nil
		log.Debugf("Release Close returned %s", h)
		h.n.f.mutations.Done()
	}
	return nil
}

// Flush is called when the file is closed.
func (h *handle) Flush(ctx context.Context, req *fuse.FlushRequest) error {
	h.lock()
	defer h.unlock()
	log.Debugf("Flush called on %s %X", h, uint64(req.Handle))
	if err := h.n.checkHeader(ctx, h.n, req.Header); err != nil {
		return err
	}
	if h.w != nil {
		h.n.lock()
		h.n.item.Header.DataSize = h.size
		h.n.unlock()
	}
	return nil
}

// Read reads data from the file.
func (h *handle) Read(ctx context.Context, req *fuse.ReadRequest, resp *fuse.ReadResponse) error {
	h.lock()
	defer h.unlock()
	log.Debugf("Read(off:%d size:%d) called on %s %X", req.Offset, req.Size, h, uint64(req.Handle))
	if err := h.n.checkHeader(ctx, h.n, req.Header); err != nil {
		return err
	}
	if req.Dir {
		return syscall.ENOTDIR
	}
	if h.r == nil {
		return syscall.EINVAL
	}
	_, err := h.r.Seek(req.Offset, io.SeekStart)
	if err != nil {
		log.Debugf("Seek(%d) failed: %v", req.Offset, err)
		return err
	}
	buf := make([]byte, req.Size)
	n, err := h.r.Read(buf)
	log.Debugf("Read returned %d bytes and err=%v", n, err)
	if n > 0 {
		resp.Data = buf[:n]
		return nil
	}
	return err
}

// Write writes data to the file.
func (h *handle) Write(ctx context.Context, req *fuse.WriteRequest, resp *fuse.WriteResponse) error {
	h.lock()
	defer h.unlock()
	log.Debugf("Write called on %s %X", h, uint64(req.Handle))
	if h.w == nil {
		return syscall.EINVAL
	}
	if h.size < req.Offset {
		log.Debugf("File is sparse. Offset is greater than size. Padding with %d bytes", req.Offset-h.size)
		var buf [4096]byte
		for h.size != req.Offset {
			n := req.Offset - h.size
			if n > int64(len(buf)) {
				n = int64(len(buf))
			}
			nn, err := h.w.Write(buf[:n])
			if err != nil {
				return err
			}
			h.size += int64(nn)
		}
	}
	if h.size != req.Offset {
		log.Errorf("Write offset doesn't match current position, %d != %d", h.size, req.Offset)
		return syscall.EINVAL
	}
	n, err := h.w.Write(req.Data)
	log.Debugf("Write returned n=%d, err=%v  len(data)=%d", n, err, len(req.Data))
	if err != nil {
		return err
	}
	h.size += int64(n)
	resp.Size = n

	h.n.lock()
	var attr fuse.Attr
	h.n.attrLocked(ctx, &attr)
	attr.Size = uint64(h.size)
	attr.Mtime = time.Now()
	h.n.f.setAttr(h.n.item.FSFile.File, attr)
	h.n.unlock()
	return nil
}

// Poll checks whether a handle is ready for I/O.
func (h *handle) Poll(context.Context, *fuse.PollRequest, *fuse.PollResponse) error {
	log.Debugf("Poll called on %s", h)
	return syscall.ENOSYS
}

func (h *handle) Getxattr(ctx context.Context, req *fuse.GetxattrRequest, resp *fuse.GetxattrResponse) error {
	log.Debugf("Getxattr called on %s", h)
	return syscall.ENOSYS
}
