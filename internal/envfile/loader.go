package envfile

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

func Load(path string) (map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("envfile load %s: %w", path, err)
	}
	defer f.Close()

	vars := make(map[string]string)
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		v = strings.Trim(v, `"'`)
		vars[k] = v
	}
	return vars, scanner.Err()
}

func Apply(vars map[string]string) {
	for k, v := range vars {
		os.Setenv(k, v)
	}
}
