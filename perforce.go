package perforce

// Package perforce - provide access to a couple of basic Perforce commands
//   Prerequisites:
//					p4 installed and in path
//
//
//   no timeouts

// New()                create an instance/workspace

//  Local paths are systematically enclosed with double quotes in ws def but not in def files.


import (
	"errors"
	"fmt"
	"io"
	//"io/ioutil"
 	"log"
	//"os"
	"os/exec"
	"time"
	//"path/filepath"
	//"strconv"
	//"strings"
)

type Perforce struct {
	user				string 			// optional p4 user
	p4Cmd 			string 			// p4 command and path
	logWriter   io.Writer
	debug				bool
}


// Create an instance
// - lookup path to p4 command
// - Returns instance and error code
func New(user string) (*Perforce, error) {
	p := &Perforce{} // Create instance

	var err error
	p.user = user
	// Try accessing the command p4 to make sure it is installed and can be called
	p.p4Cmd, err = exec.LookPath("p4")
	if err != nil {
		return nil, errors.New(fmt.Sprintf("Unable to find path to p4 command - %v", err))
	}
	p.debug = false  // default
	return p, nil
}



// ---------------------------------------
// Debug functions

// Test connection to server
// 	Execute p4 info command - recommended to check connection to server
func (p *Perforce) P4Info() (output string, err error) {
	p.log("P4Info()")
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

/* // log - traces errors if debug set to true.
func (p *Perforce) log(inf interface{}) {
	if p.debugFlg {
		if p.logWriter != nil {
			p.logWriter.Printf("%v", inf)
		} else {
			log.Println(inf)
		}
	}
} */

// Log writer
func (p *Perforce) log(a interface{}) {
	if p.debug {
		if p.logWriter != nil {
			timestamp := time.Now().Format(time.RFC3339)
			msg := fmt.Sprintf("%v: %v", timestamp, a)
			fmt.Fprintln(p.logWriter, msg)
		} else {
			log.Println(a)
		}
	}
}
