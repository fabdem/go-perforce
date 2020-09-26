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
//		- the file in a temp file in os.TempDir()
//		- its 'perfore name' with revision number for info only
//		- err code, nil if okay
func (p *Perforce) GetFile(depotFile string, rev int) ( tempFile *os.File ,fileName string, err error) {
	p.log("GetFile()")

	fileName = filepath.Base(depotFile) // extract filename
	ext := filepath.Ext(depotFile)       // Read extension

	if rev > 0 { // If a specific version is requested
		fileName = fileName[0:len(fileName)-len(ext)] + "#" + strconv.Itoa(rev) + ext
	} else {    // Get head rev
		rev, err = p.GetHeadRev(depotFile)
		if err != nil {
			return tempFile,fileName, err
		}
		fileName = fileName[0:len(fileName)-len(ext)] + "#" + strconv.Itoa(rev) + ext
	} // fileName is provided as a convenience

	p.log(fmt.Sprintf("fileName=%s rev=%d\n",fileName, rev))

	tempFile, err = ioutil.TempFile("", "perforce_getfile_")
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


// Get content from a pending Change List
// Do a: p4 -uxxxxx describe 6102201
// 	Input:
//		- Change List number
//  Return:
//		- a map of files (depot path and name) and rev numbers
//		- err code, nil if okay
/*
Change 6102201 by xxxx@yyyyyyyy on 2020/09/20 21:02:41 *pending*

	Test diff

Affected files ...

... //zzzzzz/dev/locScriptTesting/main_french.json#1 edit
... //zzzzzz/dev/locScriptTesting/yy_french.txt#18 edit
... //zzzzzz/dev/locScriptTesting/yy_german.txt#8 edit

*/
//
func (p *Perforce) GetPendingCLContent(changeList int) ( m_files map[string]int,  err error) {
	p.log("GetChangeListContent()")

	var out []byte
	m_files = make(map[string]int)

	if len(p.user) > 0 {
		// fmt.Printf(p4Cmd + " -u " + user + " describe " + " " + strconv.Itoa(changeList) + "\n")
		out, err = exec.Command(p.p4Cmd, "-u", p.user, "describe", strconv.Itoa(changeList)).CombinedOutput()
		//fmt.Printf("P4 command line result - err=%s\n out=%s\n", err, out)
	} else {
		//fmt.Printf(p.p4Cmd  + " describe " + " " + strconv.Itoa(changeList) + "\n")
		out, err = exec.Command(p.p4Cmd, "describe", strconv.Itoa(changeList)).CombinedOutput()
		// out, err := exec.Command(p.p4Cmd, "info").Output()
	}
	if err != nil {
		return m_files, errors.New(fmt.Sprintf("P4 command line error %v  out=%s", err, out))
	}

	// Interpreting P4 response - expecting it to be in a specific format
	cue1 := "Affected files ..."
	cue2 := "... //"
	idx := strings.Index(string(out), cue1)
	if idx == -1 {
		return m_files, errors.New(fmt.Sprintf("Error interpreting P4 response - missing field %s in out=%s", cue1, out))
	}
	idx = strings.Index(string(out), cue2)
	if idx == -1 {
		return m_files, errors.New(fmt.Sprintf("Error interpreting P4 response - missing field %s in out=%s", cue2, out))
	}

	lines := strings.Split(string(out[idx:]),"\n")
	for _, line := range lines {
		fmt.Printf("line %s\n", line)
		if strings.Index(line, cue2) == -1 { // If there is no "... //" we're done
			break
		}
		fields := strings.Fields(line)
		fileAndVersion := strings.Split(fields[1],"#")
		file := fileAndVersion[0]
		version,err :=  strconv.Atoi(fileAndVersion[1])
		if err !=nil || len(file) <= 0 || version <= 0 {
			return m_files, errors.New(fmt.Sprintf("Error interpreting P4 response - file details %s in out=%s", line, out))
		}
		m_files[file] = version // populate the map
	}
	return m_files, nil

}
