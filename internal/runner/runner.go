package runner

import (
	"os"
	"os/exec"
)

type OS struct{}

func (r *OS) Execute(name string, args ...string) ([]byte, error) {
	return exec.Command(name, args...).Output()
}

func (r *OS) ExecuteStreaming(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
