package runner

import "os/exec"

type OS struct{}

func (r *OS) Execute(name string, args ...string) ([]byte, error) {
	return exec.Command(name, args...).Output()
}
