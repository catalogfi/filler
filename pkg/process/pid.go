package process

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"github.com/catalogfi/cobi/utils"
)

func NewPidManager(uid string) PidManager {
	pidDirPath := utils.DefaultCobiPids()
	pidPath := filepath.Join(pidDirPath, PidFile(uid))

	return &pid{
		PidPath: pidPath,
	}
}

func (p *pid) Write() error {
	if p.IsActive() {
		return fmt.Errorf("executor already running")
	}
	pid := strconv.Itoa(os.Getpid())
	err := os.WriteFile(p.PidPath, []byte(pid), 0644)
	if err != nil {
		return fmt.Errorf("failed to write pid, err:%v", err)
	}
	return nil
}

func (p *pid) Remove() error {
	if p.IsActive() {
		err := os.Remove(p.PidPath)
		if err != nil {
			return fmt.Errorf("failed to delete executor pid file, err:%v", err)
		}
	} else {
		return fmt.Errorf("pid file not found")
	}
	return nil
}

func (p *pid) IsActive() bool {
	if _, err := os.Stat(p.PidPath); err != nil {
		return false
	}
	return true
}
