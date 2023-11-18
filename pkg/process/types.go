package process

import (
	"fmt"
	"io"
	"os"
	"syscall"
)

const (
	DefaultStopSignal      = syscall.SIGQUIT
	DefaultRestartSignal   = syscall.SIGHUP
	DefaultSuccessfulMsg   = "success"
	DefaultMaxPipeReadSize = 1024
)

// Pipe represents a simple wrapper for a bi-directional pipe.
type pipe struct {
	stdin  *io.PipeReader
	stdout *io.PipeWriter
}

type process struct {
	Uid      string
	LogsPath string
	PidPath  string
	Pipe     Pipe
}

type pid struct {
	PidPath string
}

type PidManager interface {
	Write() error
	Remove() error
}

type Pipe interface {
	Write(data []byte) (int, error)
	Read(data []byte) (int, error)
	Close() error
}

type ProcessManager interface {
	// returns pid from the file, doest check if the process is running with that pid is running or not
	GetPid() (int, error)
	// return uid of the process manager
	GetUid() string
	WriteToPipe(msg []byte) error
	ReadFromPipe() (int, []byte, error)
	Start(binaryPath string, args []string) (int, []byte, error)
	Stop(signal ...os.Signal) error
	Restart(signal ...os.Signal) error
	IsActive() bool
}

func PidFile(name string) string {
	return fmt.Sprintf("%s.pid", name)
}

func LogFile(name string) string {
	return fmt.Sprintf("%s.log", name)
}
