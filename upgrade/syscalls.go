//go:generate ../hack/tools/bin/mockgen -destination ./mocks/syscalls_mock.go -package mocks . SysCalls

package upgrade

import (
	"context"
	"os"
	"os/exec"
)

/*
type SysCalls interface {
	WriteFile(string, []byte, os.FileMode) error
	ReadFile(string) ([]byte, error)
	OpenFile(string, int, os.FileMode) (*os.File, error)
	Stat(string) (os.FileInfo, error)
	Executable() (string, error)
	ExecCommand(context.Context, string, ...string) ([]byte, error)
	MkdirAll(string, os.FileMode) error
}
*/

type SysCalls struct {
	WriteFile   func(string, []byte, os.FileMode) error
	ReadFile    func(string) ([]byte, error)
	OpenFile    func(string, int, os.FileMode) (*os.File, error)
	Stat        func(string) (os.FileInfo, error)
	Executable  func() (string, error)
	ExecCommand func(context.Context, string, ...string) ([]byte, error)
	MkdirAll    func(string, os.FileMode) error
}

/*
	func (s *SysCalls) WriteFile(name string, data []byte, perm os.FileMode) error {
		return os.WriteFile(name, data, perm)
	}

	func (s *SysCalls) ReadFile(name string) ([]byte, error) {
		return os.ReadFile(name)
	}

	func (s *SysCalls) OpenFile(name string, flag int, perm os.FileMode) (*os.File, error) {
		return os.OpenFile(name, flag, perm)
	}

	func (s *SysCalls) Stat(name string) (fs.FileInfo, error) {
		return os.Stat(name)
	}

	func (s *SysCalls) MkdirAll(name string, perm os.FileMode) error {
		return os.MkdirAll(name, perm)
	}

	func (s *SysCalls) Executable() (string, error) {
		return os.Executable()
	}

	func (s *SysCalls) ExecCommand(ctx context.Context, name string, arg ...string) ([]byte, error) {
		return exec.CommandContext(ctx, name, arg...).CombinedOutput()
	}
*/

func ExecCommand(ctx context.Context, name string, arg ...string) ([]byte, error) {
	return exec.CommandContext(ctx, name, arg...).CombinedOutput()
}

func NewSysCalls() SysCalls {
	return SysCalls{
		WriteFile:   os.WriteFile,
		ReadFile:    os.ReadFile,
		OpenFile:    os.OpenFile,
		Stat:        os.Stat,
		Executable:  os.Executable,
		ExecCommand: ExecCommand,
		MkdirAll:    os.MkdirAll,
	}
}