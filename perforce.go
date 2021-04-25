package perforce

// Package perforce - provide access to a couple of basic Perforce commands
//   Prerequisites:
//					p4 installed and in path
//
//
//   Limitation: timeouts not managed (relies on p4 cli implementation)

// New()                create an instance/workspace

import (
	"errors"
	"fmt"
	"io"
	"log"
	"os/exec"
	"time"
)

type Perforce struct {
	user            string // optional p4 user
	workspace       string // optional p4 workspace (required by some functions)
	p4Cmd           string // p4 command and path
	logWriter       io.Writer
	debug           bool
	diffignorespace bool // when set diff ignore spaces and eol
}

// Create a new instance
// - lookup path to p4 command
// - Returns instance and error code
func New(user string, workspace string) (*Perforce, error) {
	p := &Perforce{} // Create instance

	var err error
	p.user = user
	p.workspace = workspace
	// Try accessing the command p4 to make sure it is installed and can be called
	p.p4Cmd, err = exec.LookPath("p4")
	if err != nil {
		return nil, errors.New(fmt.Sprintf("Unable to find path to p4 command - %v", err))
	}
	p.debug = false // default
	return p, nil
}

// SetUser()
func (p *Perforce) SetUser(user string) {
	p.user = user
}

// SetWorkspace()
func (p *Perforce) SetWorkspace(workspace string) {
	p.workspace = workspace
}

// GetUser()
func (p *Perforce) GetUser() (user string) {
	return (p.user)
}

// GetWorkspace()
func (p *Perforce) GetWorkspace() (workspace string) {
	return (p.workspace)
}

// SetDiffIgnoreSpace()
func (p *Perforce) SetDiffIgnoreSpace() {
	p.diffignorespace = true
}

// SetDiffIgnoreSpace()
func (p *Perforce) ResetDiffIgnoreSpace() {
	p.diffignorespace = false
}

// GetDiffIgnoreSpace()
func (p *Perforce) GetDiffIgnoreSpace() (flag bool) {
	return (p.diffignorespace)
}

// ---------------------------------------
// Debug functions

// Test connection to server
// 	Execute p4 info command - recommended to check connection to server
func (p *Perforce) P4Info() (output string, err error) {
	p.logThis("\nP4Info()")
	out, err := exec.Command(p.p4Cmd, "info").CombinedOutput()
	if err != nil {
		return "", errors.New(fmt.Sprintf("\"p4 info\" exec error: %v %s", err, out))
	}
	return string(out), nil
}

// SetDebug - traces errors if it's set to true.
func (p *Perforce) SetDebug(debug bool, logWriter io.Writer) {
	p.debug = debug
	p.logWriter = logWriter
}

// Log writer
func (p *Perforce) logThis(a interface{}) {
	if p.debug {
		if p.logWriter != nil {
			timestamp := time.Now().Format(time.RFC3339)
			msg := fmt.Sprintf("%v: %v", timestamp, a)
			fmt.Fprintln(p.logWriter, msg)
		} else {
			log.Println("p4", a)
		}
	}
}
