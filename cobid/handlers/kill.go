package handlers

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
)

type Service string

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

const (
	Executor    Service = "executor"
	Autofiller  Service = "autofiller"
	AutoCreator Service = "autocreator"
)

func Kill(service Service) error {
	pidFilePath := filepath.Join("cmd", "cobid", fmt.Sprintf("%s.pid", service))

	// Open the file that contains the PID
	data, err := os.ReadFile(pidFilePath)
	if err != nil {
		return fmt.Errorf("Error reading PID file: %v", err)
	}

	// Convert the PID from bytes to string and then to integer
	pidStr := string(data)
	pid, err := strconv.Atoi(pidStr)
	if err != nil {
		return fmt.Errorf("Error converting PID to integer: %v", err)
	}

	process, err := os.FindProcess(pid)
	if err != nil {
		return fmt.Errorf("Error finding process: %v", err)
	}

	err = process.Kill()
	if err != nil {
		return fmt.Errorf("Error killing process: %v", err)
	}

	return nil
}
