package perforce

// Publicly available high level functions

import (
	"errors"
	"fmt"
	//"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// Get a file from depot
// 	Depot file base name expected
// 	Revision number or 0 if head rev is needed
//  The caller needs to dispose of the temp file
//  Return:
//		- the file in a temp file
//		- its 'perfore name' with revision number for info only
//		- err code, nil if okay
func (p *Perforce) GetFile(depotFile string, rev int) ( tempFile *os.File ,fileName string, err error) {
	p.log("GetFile()")

	fileName = filepath.Base(depotFile) // extract filename
	ext := filepath.Ext(depotFile)       // Read extension

	if rev > 0 { // If a specific version is requested
		fileName = fileName[0:len(fileName)-len(ext)] + "#" + strconv.Itoa(rev) + ext
	} else { // Get head rev
		rev, err = p.GetHeadRev(depotFile)
		if err != nil {
			return tempFile,fileName, err
		}
		fileName = fileName[0:len(fileName)-len(ext)] + "#" + strconv.Itoa(rev) + ext
	} // fileName is provided as a convenience

	p.log(fmt.Sprintf("fileName=%s rev=%d\n",fileName, rev))

	tempFile, err = ioutil.TempFile("", "crowdin_update")
	if err != nil {
			return tempFile,fileName, errors.New(fmt.Sprintf("Unable to create a temp file - %v", err))
	}
	defer tempFile.Close()

	var out []byte

	if len(p.user) > 0 {
		out, err = exec.Command(p.p4Cmd, "-u", p.user, "print", "-k", "-q", "-o", tempFile.Name(), depotFile + "#" + strconv.Itoa(rev) ).CombinedOutput()
		// fmt.Printf("P4 command line result - %s\n %s\n", err, out)
	} else {
		out, err = exec.Command(p.p4Cmd, "print", "-k", "-q", "-o", tempFile.Name(), depotFile + "#" + strconv.Itoa(rev)).CombinedOutput()
	}
	if err != nil {
		return tempFile,fileName, errors.New(fmt.Sprintf("p4 command line error %s - %s ",err, out))
	}

	// 2EME PROBLEME POURQUOI CA PANIQUE SI ERROR DS GETFILE (REMOVE USER)

	// Unfortunately p4 print status in linux is not reliable.
	// err != nil when syntax err but not if file doesn't exist.
	// So manually checking if a file was created:
	if _, err = os.Stat(tempFile.Name()); err != nil {
		if os.IsNotExist(err) { // file does not exist
			return tempFile,fileName, errors.New(fmt.Sprintf("P4 no file created %v - %v ",out, err))
		} else { // Can't get file stat
			return tempFile,fileName, errors.New(fmt.Sprintf("Can't access the status of file produced %v - %v ",out, err))
		}
	}
	return tempFile,fileName, nil  // everything is fine returns file and file name
}


// Get from P4 the head revision number of a file
// 	depotFileName: file path and name in P4
func (p *Perforce) GetHeadRev(depotFileName string) (rev int, err error) {
	p.log("GetHeadRev()")

	var out []byte

	if len(p.user) > 0 {
		// fmt.Printf(p4Cmd + " -u " + user + " files " + " " + depotFile + "\n")
		out, err = exec.Command(p.p4Cmd, "-u", p.user, "files", depotFileName).CombinedOutput()
		//fmt.Printf("P4 command line result - err=%s\n out=%s\n", err, out)
	} else {
		//fmt.Printf(p.p4Cmd  + " files " + " " + depotFileName + "\n")
		out, err = exec.Command(p.p4Cmd, "files", depotFileName).CombinedOutput()
		// out, err := exec.Command(p.p4Cmd, "info").Output()
	}
	if err != nil {
		return 0, errors.New(fmt.Sprintf("P4 command line error %v  out=%s", err, out))
	}

	// Read version
	// e.g. //Project/dev/localization/afile_bulgarian.txt#8 - edit change 4924099 (utf16)
	idxBeg := strings.LastIndex(string(out), "#") + len("#")
	idxEnd := strings.LastIndex(string(out), " - ")
	// Check response to prevent out of bound index
	if idxBeg <= -1 || idxEnd <= -1 || idxBeg >= idxEnd {
		return 0, errors.New(fmt.Sprintf("Format error in P4 response: %s  %v", string(out), err))
	}
	sRev := string(out[idxBeg:idxEnd])

	p.log(fmt.Sprintf("Revision: %s", sRev))

	rev, err = strconv.Atoi(sRev) // Check format
	if err != nil {
		return 0, errors.New(fmt.Sprintf("Format error conv to number: %v", err))
	}

	return rev, nil
}
