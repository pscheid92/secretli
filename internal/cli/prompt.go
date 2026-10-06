package cli

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"golang.org/x/term"
)

// terminal is where questions are asked: the controlling terminal when there
// is one, so a pipe on stdin does not get in the way.
type terminal struct {
	in  *os.File
	out io.Writer
	fd  int
	own bool
}

var errNoTerminal = errors.New("not at a terminal")

func (e *env) terminal() (*terminal, error) {
	if tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0); err == nil {
		return &terminal{in: tty, out: tty, fd: int(tty.Fd()), own: true}, nil
	}
	if e.stdinTTY {
		return &terminal{in: e.stdin, out: e.stderr, fd: int(e.stdin.Fd())}, nil
	}
	return nil, errNoTerminal
}

func (t *terminal) close() {
	if t.own {
		_ = t.in.Close()
	}
}

// askLine prints the prompt and reads one line.
func (t *terminal) askLine(prompt string) (string, error) {
	_, _ = fmt.Fprint(t.out, prompt)
	line, err := bufio.NewReader(t.in).ReadString('\n')
	if err != nil && (!errors.Is(err, io.EOF) || line == "") {
		return "", err
	}
	return strings.TrimRight(line, "\r\n"), nil
}

// askSecret prints the prompt and reads a line without echoing it.
func (t *terminal) askSecret(prompt string) (string, error) {
	_, _ = fmt.Fprint(t.out, prompt)
	b, err := term.ReadPassword(t.fd)
	_, _ = fmt.Fprintln(t.out)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// confirm asks a yes/no question; anything but y or yes is no.
func (t *terminal) confirm(prompt string) (bool, error) {
	answer, err := t.askLine(prompt + " [y/N] ")
	if err != nil {
		return false, err
	}
	answer = strings.ToLower(strings.TrimSpace(answer))
	return answer == "y" || answer == "yes", nil
}

// passwordSource is how a command gets a password: a file, the environment,
// or a prompt.
type passwordSource struct {
	flag bool
	file string
}

// resolve returns the password, or "" when none was asked for. required
// makes it ask at a terminal even without the flag, since the secret needs one.
func (p passwordSource) resolve(e *env, required, confirm bool) (string, error) {
	switch {
	case p.file != "":
		b, err := os.ReadFile(p.file)
		if err != nil {
			return "", fmt.Errorf("read password file: %w", err)
		}
		line, _, _ := strings.Cut(string(b), "\n")
		return strings.TrimRight(line, "\r"), nil
	case !p.flag && !required:
		return "", nil
	}
	if pw := os.Getenv("SECRETLI_PASSWORD"); pw != "" {
		return pw, nil
	}
	t, err := e.terminal()
	if err != nil {
		return "", errors.New("a password is needed: use --password-file or SECRETLI_PASSWORD when not at a terminal")
	}
	defer t.close()
	pw, err := t.askSecret("Password: ")
	if err != nil {
		return "", err
	}
	if pw == "" {
		return "", errors.New("the password is empty")
	}
	if confirm {
		again, err := t.askSecret("Again: ")
		if err != nil {
			return "", err
		}
		if again != pw {
			return "", errors.New("the passwords differ")
		}
	}
	return pw, nil
}
