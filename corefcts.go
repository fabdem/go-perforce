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

// GetFiile()
//	Get a file from depot
// 	Depot file base name expected
// 	Revision number or 0 if head rev is needed
//  The caller needs to dispose of the temp file
//  Return:
//		- the file in a temp file in os.TempDir()
//		- its 'perfore name' with revision number for info. This is not the temp file name
//		- err code, nil if okay
func (p *Perforce) GetFile(depotFile string, rev int) ( tempFile *os.File ,fileName string, err error) {
	p.log("GetFile()\n")

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

// GetHeadRev()
//	Get from P4 the head revision number of a file
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

// GetPendingCLContent()
//	Get content from a pending Change List
//	Do a: p4 -uxxxxx describe 6102201
// 	Input:
//		- Change List number
//  Return:
//		- a map of files (depot path and name) and rev numbers
//		- CL's user and workspace
//		- err code, nil if okay
/*
p4 -uxxxx describe 6102201
Change 6102201 by xxxx@yyyyyyyy on 2020/09/20 21:02:41 *pending*

	Test diff

Affected files ...

... //zzzzzz/dev/locScriptTesting/main_french.json#1 edit
... //zzzzzz/dev/locScriptTesting/yy_french.txt#18 edit
... //zzzzzz/dev/locScriptTesting/yy_german.txt#8 edit

*/

func (p *Perforce) GetPendingCLContent(changeList int) ( m_files map[string]int, user string, workspace string, err error) {
	p.log("GetChangeListContent()\n")

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
		return m_files,"","", errors.New(fmt.Sprintf("P4 command line error %v  out=%s", err, out))
	}

	// Get CL user and workspace - TBD replace with a regex search
	cue1 := "Change " + strconv.Itoa(changeList) + " by "

	idx := strings.Index(string(out), cue1)
	if idx == -1 {
		return m_files,"","", errors.New(fmt.Sprintf("Error parsing P4 response - missing user and workspace %s in out=%s", cue1, out))
	}

	endFieldIdx := strings.Index(string(out[idx + len(cue1):])," ") + len(cue1)
	if idx == -1 {
		return m_files,"","", errors.New(fmt.Sprintf("Error parsing P4 response - malformed responsein out=%s", out))
	}

	field := string(out[idx + len(cue1): endFieldIdx])
	usrAndWs := strings.Split(field, "@")
	if len(usrAndWs) != 2 {
		return m_files,"","", errors.New(fmt.Sprintf("Error parsing P4 response - issue getting user and workspace %s in out=%s", field, out))
	}
	user = usrAndWs[0]
	workspace = usrAndWs[1]

	// Get list of affected files
	cue2 := "Affected files ..."
	cue3 := "... //"
	cue4 := "... "
	cue5 := " edit"
	idx = strings.Index(string(out), cue2)
	if idx == -1 {
		return m_files,"","", errors.New(fmt.Sprintf("Error parsing P4 response - missing field %s in out=%s", cue2, out))
	}
	idx = strings.Index(string(out), cue3)
	if idx == -1 {
		return m_files,"","", errors.New(fmt.Sprintf("Error parsing P4 response - missing field %s in out=%s", cue3, out))
	}

	lines := strings.Split(string(out[idx:]),"\n")
	for _, line := range lines {
		// fmt.Printf("line %s\n", line)
		if strings.Index(line, cue3) == -1 { // If there is no "... //" we're done
			break
		}
		line = strings.TrimPrefix(line, cue4) // Isolate path
		line = strings.TrimSuffix(line, cue5)
		fileAndVersion := strings.Split(line,"#")
		file := fileAndVersion[0]
		version,err :=  strconv.Atoi(fileAndVersion[1])
		if err !=nil || len(file) <= 0 || version <= 0 {
			return m_files,"","", errors.New(fmt.Sprintf("Error parsing P4 response - file details %s in out=%s", line, out))
		}
		m_files[file] = version // populate the map
	}

	return m_files, user, workspace, nil
}

// DiffHeadnWorkspace()
// 	Diff head rev and workspace in simplified form.
//	p4 diff returns a number of added, modified and deleted lines.
// 	Do a: p4 -uxxxxx -wyyyyy diff //workspacefile
//	A workspace name needs to be defined
//  If p.diffignorespace is set changes in spaces and eol will be ignored.
// 	Input:
//		- File in depot to diff - p4 will automatically determine workspace path
//  Return:
//		- added, deleted and modified number of lines
//		- err code, nil if okay
/*
p4 -ca_workspace -ua_user diff -ds //path_and_name_of_a_file_in_depot
==== path_and_name_of_a_file_in_depot - path_and_name_of_a_file_in_workspace ====
add 3 chunks 8 lines
deleted 2 chunks 7 lines
changed 1 chunks 1 / 2 lines
*/
func (p *Perforce) DiffHeadnWorkspace(aFileInDepot string) ( diffedFileInDepot string,diffedFileInWorkspace string, addedLines int,removedLines int,changedLines int, err error) {
	p.log("DiffHeadnWorkspace()\n")

	var out []byte
	option := "-ds"  // Summary output
	if p.diffignorespace {
		option += "b" // plus changes within spaces will be ignored
	}

	if len(p.workspace) <= 0 {
		return "","",0,0,0,errors.New(fmt.Sprintf("P4 command line error - a workspace needs to be defined"))
	}
	if len(p.user) > 0 {
		// fmt.Printf(p4Cmd + " -u " + user + " -c " + workspace + " diff -ds " + " " + aFileInDepot + "\n")
		out, err = exec.Command(p.p4Cmd, "-u", p.user,"-c", p.workspace, "diff",option, aFileInDepot).CombinedOutput()
		//fmt.Printf("P4 command line result - err=%s\n out=%s\n", err, out)
	} else {
		// fmt.Printf(p4Cmd + " -c " + workspace + " diff -ds " + " " + aFileInDepot + "\n")
		out, err = exec.Command(p.p4Cmd,"-c", p.workspace, "diff","option", aFileInDepot).CombinedOutput()
		// out, err := exec.Command(p.p4Cmd, "info").Output()
	}
	if err != nil {
		return "","",0,0,0, errors.New(fmt.Sprintf("P4 command line error %v  out=%s", err, out))
	}

	// Parse result
	cue1 := "===="
	cue2 := "==== "
	cue3 := " ===="
	cue4 := " - "
	fields := strings.Split(string(out),cue1)
	if len(fields) < 1 {
		return "","",0,0,0, errors.New(fmt.Sprintf("P4 command line - parsing error  out=%s", out))
	}
	line := fields[1] // 1st line is supposed to contain path of files in depot and workspace.
	line = strings.TrimPrefix(line, cue2) // Isolate paths
	line = strings.TrimSuffix(line, cue3)
	fields = strings.Split(line, cue4)
	if len(fields) < 2 {
		return "","",0,0,0, errors.New(fmt.Sprintf("P4 command line - parsing error in %s\n out=%s",line, out))
	}
	diffedFileInDepot = fields[0]
	diffedFileInWorkspace = fields[1]

	fields = strings.Split(string(out),cue3)  // Split to get section with line stats
	if len(fields) < 2 {
		return "","",0,0,0, errors.New(fmt.Sprintf("1 - P4 command line - parsing error in out=%s\n",out))
	}
	// fmt.Printf("\n\n\nfields[1]\n%s\n\n",fields[1])

	lines := strings.Split(fields[1],"\n") // Get the section with line stats
	if len(lines) < 4 {
		return "","",0,0,0, errors.New(fmt.Sprintf("2 - P4 command line - parsing error in %s\n out=%s\n",lines,out))
	}
	// fmt.Printf("\n\nlines[]\n%s\n%s\n%s\n",lines[1],lines[2],lines[3])

	/*
		add 3 chunks 8 lines
		deleted 2 chunks 7 lines
		changed 1 chunks 1 / 2 lines
	*/
	if (strings.Index(lines[1],"add") == -1)||(strings.Index(lines[2],"deleted") == -1)||(strings.Index(lines[3],"changed") == -1) {
		return "","",0,0,0, errors.New(fmt.Sprintf("3 - P4 command line - parsing error in:\n%s\n%s\n%s\n out=%s\n",lines,out))
	}
	addLine := strings.Fields(lines[1])
	removeLine := strings.Fields(lines[2])
	changeLine := strings.Fields(lines[3])

	//fmt.Printf("addLine:%v\nremoveLine:%v\nchangeLine:%v\n",addLine,removeLine,changeLine)
	if (len(addLine) < 4)||(len(removeLine) < 4)||(len(changeLine) < 5) {
		return "","",0,0,0, errors.New(fmt.Sprintf("4 - P4 command line - parsing error out=%s\n",out))
	}
	var err1, err2, err3, err4 error
	addedLines,err1 = strconv.Atoi(addLine[3])
	removedLines,err2 = strconv.Atoi(removeLine[3])
	changedLines1,err3 := strconv.Atoi(changeLine[3])
	changedLines2,err4 := strconv.Atoi(changeLine[5])
	changedLines = changedLines1 + changedLines2  // Not too clear how p4 compute this - see https://community.perforce.com/s/article/10639
	if (err1 != nil)||(err2 != nil)||(err3 != nil)||(err4 != nil) {
		return "","",0,0,0, errors.New(fmt.Sprintf("5 - P4 command line - parsing error out=%s\n",out))
	}

	return diffedFileInDepot,diffedFileInWorkspace,addedLines,removedLines,changedLines,nil
}
