package hugofs

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/afero"
)

var (
	_ FilesystemUnwrapper = (*DebugFs)(nil)
)

func NewDebugFs(fs afero.Fs) afero.Fs {
	return &DebugFs{fs: fs}
}

type DebugFs struct {
	fs afero.Fs
}

func (fs *DebugFs) Create(name string) (afero.File, error) {
	f, err := fs.fs.Create(name)
	fmt.Printf("Create: %q, %v\n", name, err)
	return f, err
}

func (fs *DebugFs) Mkdir(name string, perm os.FileMode) error {
	err := fs.fs.Mkdir(name, perm)
	fmt.Printf("Mkdir: %q, %v\n", name, err)
	return err
}

func (fs *DebugFs) MkdirAll(path string, perm os.FileMode) error {
	err := fs.fs.MkdirAll(path, perm)
	fmt.Printf("MkdirAll: %q, %v\n", path, err)
	return err
}

func (fs *DebugFs) Open(name string) (afero.File, error) {
	f, err := fs.fs.Open(name)
	fmt.Printf("Open: %q, %v\n", name, err)
	return f, err
}

func (fs *DebugFs) OpenFile(name string, flag int, perm os.FileMode) (afero.File, error) {
	f, err := fs.fs.OpenFile(name, flag, perm)
	fmt.Printf("OpenFile: %q, %v\n", name, err)
	return f, err
}

func (fs *DebugFs) Remove(name string) error {
	err := fs.fs.Remove(name)
	fmt.Printf("Remove: %q, %v\n", name, err)
	return err
}

func (fs *DebugFs) RemoveAll(path string) error {
	err := fs.fs.RemoveAll(path)
	fmt.Printf("RemoveAll: %q, %v\n", path, err)
	return err
}

func (fs *DebugFs) Rename(oldname string, newname string) error {
	err := fs.fs.Rename(oldname, newname)
	fmt.Printf("Rename: %q, %q, %v\n", oldname, newname, err)
	return err
}

func (fs *DebugFs) Stat(name string) (os.FileInfo, error) {
	fi, err := fs.fs.Stat(name)
	fmt.Printf("Stat: %q, %v\n", name, err)
	return fi, err
}

func (fs *DebugFs) Name() string {
	return "DebugFs"
}

func (fs *DebugFs) Chmod(name string, mode os.FileMode) error {
	err := fs.fs.Chmod(name, mode)
	fmt.Printf("Chmod: %q, %v\n", name, err)
	return err
}

func (fs *DebugFs) Chown(name string, uid int, gid int) error {
	err := fs.fs.Chown(name, uid, gid)
	fmt.Printf("Chown: %q, %v\n", name, err)
	return err
}

func (fs *DebugFs) Chtimes(name string, atime time.Time, mtime time.Time) error {
	err := fs.fs.Chtimes(name, atime, mtime)
	fmt.Printf("Chtimes: %q, %v\n", name, err)
	return err
}

func (fs *DebugFs) UnwrapFilesystem() afero.Fs {
	return fs.fs
}
