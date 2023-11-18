package process

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"

	"github.com/catalogfi/cobi/utils"
)

func NewProcessManager(uid string) ProcessManager {

	pidDirPath := utils.DefaultCobiPids()
	logsDirPath := utils.DefaultCobiLogs()
	logsPath := filepath.Join(logsDirPath, LogFile(uid))
	pidPath := filepath.Join(pidDirPath, PidFile(uid))

	return &process{
		Uid:      uid,
		LogsPath: logsPath,
		PidPath:  pidPath,
	}
}
func (p *process) GetUid() string {
	return p.Uid
}
func (p *process) GetPid() (int, error) {
	if _, err := os.Stat(p.PidPath); err != nil {
		return 0, fmt.Errorf("pid file not found")
	}
	data, err := os.ReadFile(p.PidPath)
	if err != nil {
		return 0, fmt.Errorf("error reading PID file: %v", err)
	}

	// Convert the PID from bytes to string and then to integer
	pidStr := string(data)
	pid, err := strconv.Atoi(pidStr)
	if err != nil {
		return 0, fmt.Errorf("error converting PID to integer: %v", err)
	}
	if pid <= 0 {
		return 0, fmt.Errorf("invalid pid")
	}
	return pid, err
}

func (p *process) Start(binaryPath string, args []string) (int, []byte, error) {

	if p.IsActive() {
		return 0, nil, fmt.Errorf("process already running")
	}

	cmd := exec.Command(binaryPath, args...)

	reader, writer := io.Pipe()

	p.Pipe = NewPipe(reader, writer)

	cmd.Stdin = reader
	cmd.Stdout = writer

	if err := cmd.Start(); err != nil {
		return 0, nil, fmt.Errorf(fmt.Sprintf("error starting process, err:%v", err))
	}

	if cmd == nil || cmd.ProcessState != nil && cmd.ProcessState.Exited() || cmd.Process == nil {
		return 0, nil, fmt.Errorf("error starting process")
	}

	n, msg, err := p.ReadFromPipe()
	if err != nil {
		return 0, nil, fmt.Errorf("error reading process")
	}
	fmt.Println("24567oi")

	return n, msg, nil
}
func (p *process) Stop(signal ...os.Signal) error {
	pid, err := p.GetPid()
	if err != nil {
		return err
	}
	process, err := os.FindProcess(pid)
	if err != nil {
		return fmt.Errorf("error finding process: %v", err)
	}

	var sig os.Signal
	if len(signal) == 0 {
		sig = DefaultStopSignal
	} else {
		sig = signal[0]
	}

	err = process.Signal(sig)
	if err != nil {
		return fmt.Errorf("error killing process: %v", err)
	}

	return nil
}

func (p *process) Restart(signal ...os.Signal) error {
	pid, err := p.GetPid()
	if err != nil {
		return err
	}
	process, err := os.FindProcess(pid)
	if err != nil {
		return fmt.Errorf("error finding process: %v", err)
	}

	var sig os.Signal
	if len(signal) == 0 {
		sig = DefaultRestartSignal
	} else {
		sig = signal[0]
	}

	err = process.Signal(sig)
	if err != nil {
		return fmt.Errorf("error killing process: %v", err)
	}

	return nil
}

func (p *process) WriteToPipe(msg []byte) error {
	n, err := p.Pipe.Write(msg)
	if err != nil {
		return err
	}
	if len(msg) != n {
		return fmt.Errorf("error writing to pipe")
	}
	return nil
}

func (p *process) ReadFromPipe() (int, []byte, error) {
	buf := make([]byte, DefaultMaxPipeReadSize)
	n, err := p.Pipe.Read(buf)
	if err != nil && err != io.EOF {
		return 0, nil, fmt.Errorf("error reading from pipe, err:%v", err)
	}
	return n, buf, nil
}

func (p *process) IsActive() bool {
	if _, err := os.Stat(p.PidPath); err != nil {
		return false
	}
	data, err := os.ReadFile(p.PidPath)
	if err != nil {
		return false
	}

	// Convert the PID from bytes to string and then to integer
	pidStr := string(data)
	pid, err := strconv.Atoi(pidStr)
	if err != nil {
		return false
	}

	if pid <= 0 {
		return false
	}

	pr, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	err = pr.Signal(syscall.Signal(0))
	return err == nil
}
