package handlers

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"syscall"

	"github.com/catalogfi/cobi/utils"
)

// Set implements pflag.Value.
func (s *Service) Set(v string) error {
	switch v {
	case "executor", "autofiller", "autocreator":
		*s = Service(v)
		return nil
	default:
		return fmt.Errorf("invalid service type: %s", v)
	}
}

// String implements pflag.Value.
func (s *Service) String() string {
	return string(*s)
}

// Type implements pflag.Value.
func (s *Service) Type() string {
	return "service"
}

func Kill(service KillSerivce) error {
	var pidFilePath string
	if service.ServiceType == Executor {
		pidFilePath = filepath.Join(utils.DefaultCobiPids(), fmt.Sprintf("%s_account_%d.pid", service.ServiceType, service.Account))
	} else {
		pidFilePath = filepath.Join(utils.DefaultCobiPids(), fmt.Sprintf("%s.pid", service.ServiceType))
	}
	// Open the file that contains the PID
	data, err := os.ReadFile(pidFilePath)
	if err != nil {
		return fmt.Errorf("error reading PID file: %v", err)
	}

	// Convert the PID from bytes to string and then to integer
	pidStr := string(data)
	pid, err := strconv.Atoi(pidStr)
	if err != nil {
		return fmt.Errorf("error converting PID to integer: %v", err)
	}

	process, err := os.FindProcess(pid)
	if err != nil {
		return fmt.Errorf("error finding process: %v", err)
	}

	err = process.Signal(syscall.SIGQUIT)
	if err != nil {
		return fmt.Errorf("error killing process: %v", err)
	}

	err = os.Remove(pidFilePath)
	if err != nil {
		return fmt.Errorf("error deleting procpid file for %s, err : %v", pidFilePath, err)
	}

	return nil
}
