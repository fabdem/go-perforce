// Package p4Wrapper - provide access to a couple of basic Perforce commands
//   Prerequisites:
//					p4 installed and in path
//   Open wkspace: p4 client -i < sandboxP4Definition.txt
//   Sync files: p4 sync -s
//   Checkout files: p4 -x tf2list.txt edit
//   Revert unchanged files: p4 -x tf2list.txt revert -a
//   Submit: p4 submit -d "[tf2] Updating loc files."
//   close
//
//   reentrant?
//   no timeout?

// New()                create an instance/workspace

//  Local paths are systematically enclosed with double quotes in ws def but not in def files.

package perforce

import (
	"errors"
	"fmt"
	"io"
	"io/ioutil"
 	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

type Perforce struct {
	userId				string 			// optional p4 user
	p4Cmd 				string 			// p4 command and path
	logWriter     io.Writer
	debugFlg			bool
}


// Create an instance
// - lookup path to p4 command
// - Returns instance and error code
func New(user string) (*Perforce, error) {
	p := &Perforce{} // Create instance

	var err error
	p.userId = user
	// Try accessing the command p4 to make sure it is installed and can be called
	p.p4Cmd, err = exec.LookPath("p4")
	if err != nil {
		return nil, errors.New(fmt.Sprintf("Unable to find path to p4 command - %v", err))
	}
	return p, nil
}

// Get a file from depot
// 	Depot file base name expected
// 	Revision number or 0 if head rev is needed
//  Return:
//		- the file in a temp file
//		- its 'perfore name' with revision number for info only
//		- err code, nil if okay
func (p *Perforce) GetFile(depotFile string, rev int) ( *os.File ,string, error) {
	p.log("GetFile()")

	fileName := filepath.Base(depotFile) // extract filename
	ext := filepath.Ext(depotFile)       // Read extension

	if rev > 0 { // If a specific version is requested
		fileName = fileName[0:len(fileName)-len(ext)] + "#" + strconv.Itoa(rev) + ext
	} else { // Get head rev
		r, err := p.GetHeadRev(depotFile)
		if err != nil {
			return nil,"", err
		}
		fileName = fileName[0:len(fileName)-len(ext)] + "#" + strconv.Itoa(r) + ext
	} // fileName is provided as a convenience

	tempFile, err := ioutil.TempFile("", "crowdin_update")
	if err != nil {
	    msg := fmt.Sprintf("Unable to create a temp file - %v", err)
			p.log(msg)
			return nil,"", errors.New(msg)
	}
	defer os.Remove(tempFile.Name())

	var out []byte

	if len(p.userId) > 0 {
		out, err = exec.Command(p.p4Cmd, "-u", p.userId, "print", "-k", "-q", "-o", tempFile.Name(), depotFile + "#" + strconv.Itoa(rev) ).CombinedOutput()
		// fmt.Printf("P4 command line result - %s\n %s\n", err, out)
	} else {
		out, err = exec.Command(p.p4Cmd, "print", "-k", "-q", "-o", tempFile.Name(), depotFile + "#" + strconv.Itoa(rev)).CombinedOutput()
	}
	if err != nil {
		msg := fmt.Sprintf("p4 command line error %v - %v ",out, err)
		p.log(msg)
		return nil,"", errors.New(msg)
	}

	// Unfortunately p4 print status in linux is not reliable.
	// err != nil when syntax err but not if file doesn't exist.
	// So manually checking if a file was created:
	if _, err = os.Stat(tempFile.Name()); err != nil {
		if os.IsNotExist(err) { // file does not exist
			msg := fmt.Sprintf("p4 no file produced %v - %v ",out, err)
			p.log(msg)
			return nil,"", errors.New(msg)
		} else { // Can't get file stat
			msg := fmt.Sprintf("Can't access the status of file produced %v - %v ",out, err)
			p.log(msg)
			return nil,"", errors.New(msg)
		}
	}
	return tempFile,fileName, nil  // everything is fine returns file and file name
}


// Get from P4 the head revision number of a file
// 	depotFileName: file path and name in P4
func (p *Perforce) GetHeadRev(depotFileName string) (rev int, err error) {
	p.log("GetHeadRev()")

	var out []byte
	if len(p.userId) > 0 {
		// fmt.Printf(p4Cmd + " -u " + user + " files " + " " + depotFile + "\n")
		out, err = exec.Command(p.p4Cmd, "-u", p.userId, "files", depotFileName).Output()
	} else {
		// fmt.Printf(p4Cmd + " files " + depotFileName + "\n")
		out, err = exec.Command(p.p4Cmd, "files", depotFileName).Output()
	}
	if err != nil {
		msg := fmt.Sprintf("P4 command line error - %v", err)
		p.log(msg)
		return 0, errors.New(msg)
	}

	// Read version
	// e.g. //Project/dev/localization/afile_bulgarian.txt#8 - edit change 4924099 (utf16)
	idxBeg := strings.LastIndex(string(out), "#") + len("#")
	idxEnd := strings.LastIndex(string(out), " - ")
	// Check response to prevent out of bound index
	if idxBeg <= -1 || idxEnd <= -1 || idxBeg >= idxEnd {
		msg := fmt.Sprintf("Format error in P4 response: %s  %v", string(out), err)
		p.log(msg)
		return 0, errors.New(msg)
	}
	sRev := string(out[idxBeg:idxEnd])

	rev, err = strconv.Atoi(sRev) // Check format
	if err != nil {
		msg := fmt.Sprintf("Format error conv to number: %v", err)
		p.log(msg)
		return 0, errors.New(msg)
	}

	return rev, nil
}



// ---------------------------------------
// Debug functions

// Test connection to server
// 	Execute p4 info command - recommended to check connection to server
func (p *Perforce) P4Info() (output string, err error) {
	p.log("P4Info()")
	out, err := exec.Command(p.p4Cmd, "info").Output()
	if err != nil {
		msg := fmt.Sprintf("\"p4 info\" exec error: %v", err)
		p.log(msg)
		return "", errors.New(msg)
	}
	return string(out), nil
}

// SetDebug - traces errors if it's set to true.
func (p *Perforce) SetDebug(debug bool, logWriter io.Writer) {
	p.debugFlg = debug
	p.logWriter = logWriter
}

// log - traces errors if debug set to true.
func (p *Perforce) log(inf interface{}) {
	if p.debugFlg {
		log.Println(inf)
		if p.logWriter != nil {
			msg := fmt.Sprintf("%v", inf)
			fmt.Fprintln(p.logWriter, msg)
		}
	}
}
