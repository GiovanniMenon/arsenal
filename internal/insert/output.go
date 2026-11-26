// Writes a command to a file or runs it, for the -o and -e flags.

package insert

import (
	"fmt"
	"io"
	"os"
	"os/exec"
)

func WriteFile(path, line string) error {
	return os.WriteFile(path, []byte(line), 0o600)
}

func Run(line string, stdin io.Reader, stdout, stderr io.Writer) error {
	_, err := RunCode(line, stdin, stdout, stderr)
	return err
}

func RunCode(line string, stdin io.Reader, stdout, stderr io.Writer) (int, error) {
	cmd := exec.Command("/bin/sh", "-c", line)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = stdin, stdout, stderr
	if err := cmd.Run(); err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			return ee.ExitCode(), nil
		}
		return 0, fmt.Errorf("cannot run the command: %w", err)
	}
	return 0, nil
}
