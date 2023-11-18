package process

import (
	"io"
)

// NewPipe creates a new pipe instance with standard input/output.
func NewPipe(stdin *io.PipeReader, stdout *io.PipeWriter) Pipe {

	return &pipe{
		stdin:  stdin,
		stdout: stdout,
	}
}

// Write writes data to the pipe's standard output.
func (p *pipe) Write(data []byte) (int, error) {
	return p.stdout.Write(data)
}

// Read reads data from the pipe's standard input.
func (p *pipe) Read(data []byte) (int, error) {
	return p.stdin.Read(data)
}

// Close closes the io.
func (p *pipe) Close() error {
	if err := p.stdin.Close(); err != nil {
		return err
	}
	if err := p.stdout.Close(); err != nil {
		return err
	}
	return nil
}
