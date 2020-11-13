package perforce

// Publicly available high level functions

import (
	"bufio"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// GetP4Where()
//	Get workspace filename and path of a file from depot
// 	Depot path and filename expected
//
//  Returns:
//		- filename and path in workspace
//		- err code, nil if okay
//
//	p4 -ztag -c<workspace> -u<user> where <path to file in depot>
//	Returns:
//		... depotFile //somewhere/in/depot/a/file
//		... clientFile //somewhere/in/client/a/file
//		... path D:\a\local\path\file
//
func (p *Perforce) GetP4Where(depotFile string) (fileName string, err error) {
	p.log(fmt.Sprintf("GetP4Where(%s)\n", depotFile))

	var out []byte

	if len(p.user) > 0 {
		out, err = exec.Command(p.p4Cmd, "-ztag", "-u", p.user, "-c", p.workspace, "where", depotFile).CombinedOutput()
		// fmt.Printf("P4 command line result - %s\n %s\n", err, out)
	} else {
		out, err = exec.Command(p.p4Cmd, "-ztag", "-c", p.workspace, "where", depotFile).CombinedOutput()
	}
	if err != nil {
		return fileName, errors.New(fmt.Sprintf("p4 command line error %s - %s ", err, out))
	}
	p.log(fmt.Sprintf("Response=%s\n", string(out)))

	// Parse result
	fields := strings.Split(string(out), "... path ")
	if len(fields) < 2 {
		return fileName, errors.New(fmt.Sprintf("p4 command line parsing result error %s - %s ", err, fields))
	}
	fileName = fields[1]
	fileName = strings.Trim(fields[1], "\r\n")
	p.log(fmt.Sprintf("filename=%s\n", fileName))

	return fileName, nil
}

// GetFile()
//	Get a file from depot
// 	Depot file base name expected
// 	Revision number or 0 if head rev is needed
//  The caller needs to dispose of the temp file
//  Return:
//		- the file in a temp file in os.TempDir()
//		- its 'perfore name' with revision number for info. This is not the temp file name
//		- err code, nil if okay
func (p *Perforce) GetFile(depotFile string, rev int) (tempFile string, fileName string, err error) {
	p.log("GetFile()\n")

	fileName = filepath.Base(depotFile) // extract filename
	ext := filepath.Ext(depotFile)      // Read extension

	if rev > 0 { // If a specific version is requested
		fileName = fileName[0:len(fileName)-len(ext)] + "#" + strconv.Itoa(rev) + ext
	} else { // Get head rev
		rev, err = p.GetHeadRev(depotFile)
		if err != nil {
			return tempFile, fileName, err
		}
		fileName = fileName[0:len(fileName)-len(ext)] + "#" + strconv.Itoa(rev) + ext
	} // fileName is provided as a convenience

	p.log(fmt.Sprintf("fileName=%s rev=%d\n", fileName, rev))

	tempf, err := ioutil.TempFile("", "perforce_getfile_") // Create a temporary file placeholder.
	if err != nil {
		return tempFile, fileName, errors.New(fmt.Sprintf("Unable to create a temp file - %v", err))
	}
	tempFile = tempf.Name()
	tempf.Close()

	var out []byte

	if len(p.user) > 0 {
		out, err = exec.Command(p.p4Cmd, "-u", p.user, "print", "-k", "-q", "-o", tempFile, depotFile+"#"+strconv.Itoa(rev)).CombinedOutput()
		// fmt.Printf("P4 command line result - %s\n %s\n", err, out)
	} else {
		out, err = exec.Command(p.p4Cmd, "print", "-k", "-q", "-o", tempFile, depotFile+"#"+strconv.Itoa(rev)).CombinedOutput()
	}
	if err != nil {
		return tempFile, fileName, errors.New(fmt.Sprintf("p4 command line error %s - %s ", err, out))
	}

	// 2EME PROBLEME POURQUOI CA PANIQUE SI ERROR DS GETFILE (REMOVE USER)

	// Unfortunately p4 print status in linux is not reliable.
	// err != nil when syntax err but not if file doesn't exist.
	// So manually checking if a file was created:
	if _, err = os.Stat(tempFile); err != nil {
		if os.IsNotExist(err) { // file does not exist
			return tempFile, fileName, errors.New(fmt.Sprintf("P4 no file created %v - %v ", out, err))
		} else { // Can't get file stat
			return tempFile, fileName, errors.New(fmt.Sprintf("Can't access the status of file produced %v - %v ", out, err))
		}
	}
	return tempFile, fileName, nil // everything is fine returns file and file name
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

	p.log(fmt.Sprintf("received from P4: %s", out))

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
//		- CL's user and workspace for sanity check
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

func (p *Perforce) GetPendingCLContent(changeList int) (m_files map[string]int, user string, workspace string, err error) {
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
		return m_files, "", "", errors.New(fmt.Sprintf("P4 command line error %v  out=%s", err, out))
	}

	// Parse result file
	cue1 := "Change "
	cue2 := " by "
	cue3 := "@"
	cue4 := " on "
	cue5 := "Affected files ..."
	cue6 := "... "
	cue7 := "#"
	cue8 := " edit"
	cue9 := " add"

	// Get sCL
	r, _ := regexp.Compile(cue1 + `\d+` + cue2)
	sCL := r.FindString(string(out))
	sCL = strings.TrimPrefix(sCL, cue1)
	sCL = strings.TrimSuffix(sCL, cue2)
	cl, _ := strconv.Atoi(sCL)

	// Get User
	r, _ = regexp.Compile(cue2 + `[^` + cue3 + `]+` + cue3)
	sUSER := r.FindString(string(out))
	sUSER = strings.TrimPrefix(sUSER, cue2)
	sUSER = strings.TrimSuffix(sUSER, cue3)

	// Get workspace
	r, _ = regexp.Compile(cue3 + `[^ ]+` + cue4)
	sWS := r.FindString(string(out))
	sWS = strings.TrimPrefix(sWS, cue3)
	sWS = strings.TrimSuffix(sWS, cue4)

	// Add a check on the values above since they are mandatory
	if sCL == "" || sUSER == "" || sWS == "" || cl != changeList {
		return m_files, "", "", errors.New(fmt.Sprintf("Error parsing P4 response - missing or incorrect  field(s) %s %s %s in out=%s", sCL, sUSER, sWS, out))
		return
	}

	// Move start at the beginning of the file list
	idx := strings.Index(string(out), cue5)
	if idx == -1 {
		return m_files, "", "", errors.New(fmt.Sprintf("Error parsing P4 response - missing field %s in out=%s", cue5, out))
	}
	idx += strings.Index(string(out[idx:]), cue6)
	if idx == -1 {
		return m_files, "", "", errors.New(fmt.Sprintf("Error parsing P4 response - missing field %s in out=%s", cue6, out))
	}

	// Prep regexs
	r_file, _ := regexp.Compile(cue6 + `//[^` + cue7 + `]+` + cue7)
	r_rev, _ := regexp.Compile(cue7 + `\d+(` + cue8 + `|` + cue9 + `)`)  

	lines := strings.Split(string(out[idx:]), "\n")

	// Get files
	for _, line := range lines {
		// fmt.Printf("line %s\n", line)
		sFILE := r_file.FindString(line)
		sFILE = strings.TrimPrefix(sFILE, cue6)
		sFILE = strings.TrimSuffix(sFILE, cue7)
		// fmt.Printf("FILE=%s\n",sFILE)

		sREV := r_rev.FindString(line)
		sREV = strings.TrimPrefix(sREV, cue7)
		if strings.Index(sREV, cue8) != -1 {
			sREV = strings.TrimSuffix(sREV, cue8)
		} else {
			sREV = strings.TrimSuffix(sREV, cue9)
		}
		// fmt.Printf("REV=%s\n",sREV)

		if sFILE == "" || sREV == "" { // If empty we're done
			break
		}

		version, err := strconv.Atoi(sREV)
		if err != nil {
			return m_files, "", "", errors.New(fmt.Sprintf("Error parsing P4 response - incorrect revision number in line=%s,  err=%s", line, err))
		}

		m_files[sFILE] = version // populate the map
	}

	return m_files, sUSER, sWS, nil
}

// P4Diff()
// 	Diff head rev and workspace in simplified form.
//  Uses perforce p4 diff with option summary and ignore line endings.
//	p4 diff returns a number of added, modified and deleted lines.
// 	Do a: p4 -uxxxxx -wyyyyy diff //workspacefile
//	A workspace name needs to be defined
//  If p.diffignorespace is set changes in spaces and eol will be ignored.
// 	Input:
//		- File in depot to diff - p4 will automatically determine workspace path
//  Return:
//		- added, deleted and modified number of lines
//		- err code, nil if okay
//
//  To be noted that utf16 encoded files are correctly processed.
//
//
/*
p4 -ca_workspace -ua_user diff -ds //path_and_name_of_a_file_in_depot
==== path_and_name_of_a_file_in_depot - path_and_name_of_a_file_in_workspace ====
add 3 chunks 8 lines
deleted 2 chunks 7 lines
changed 1 chunks 3 / 3 lines
*/
type T_p4DiffRes struct {
	fileHR				string
	fileWS				string
	addedLines		int
	removedLines	int
	changedLines	int
}

func (p *Perforce) P4Diff(aFileInDepot string) (res T_p4DiffRes, err error) {
	p.log("DiffHeadnWorkspace()\n")

	var out []byte
	option := "-dls" // Summary output and ignore line endings
	if p.diffignorespace {
		option += "b" // plus changes within spaces will be ignored
	}

	if len(p.workspace) <= 0 {
		return res, errors.New(fmt.Sprintf("P4 command line error - a workspace needs to be defined"))
	}
	if len(p.user) > 0 {
		// fmt.Printf(p4Cmd + " -u " + user + " -c " + workspace + " diff -ds " + " " + aFileInDepot + "\n")
		out, err = exec.Command(p.p4Cmd, "-u", p.user, "-c", p.workspace, "diff", option, aFileInDepot).CombinedOutput()
		//fmt.Printf("P4 command line result - err=%s\n out=%s\n", err, out)
	} else {
		// fmt.Printf(p4Cmd + " -c " + workspace + " diff -ds " + " " + aFileInDepot + "\n")
		out, err = exec.Command(p.p4Cmd, "-c", p.workspace, "diff", "option", aFileInDepot).CombinedOutput()
		// out, err := exec.Command(p.p4Cmd, "info").Output()
	}
	if err != nil {
		return res, errors.New(fmt.Sprintf("P4 command line error %v  out=%s", err, out))
	}

	// Parse result
	cue1 := "===="
	cue2 := "==== "
	cue3 := " ===="
	cue4 := " - "
	fields := strings.Split(string(out), cue1)
	if len(fields) < 1 {
		return res, errors.New(fmt.Sprintf("P4 command line - parsing error  out=%s", out))
	}
	line := fields[1]                     // 1st line is supposed to contain path of files in depot and workspace.
	line = strings.TrimPrefix(line, cue2) // Isolate paths
	line = strings.TrimSuffix(line, cue3)
	fields = strings.Split(line, cue4)
	if len(fields) < 2 {
		return res, errors.New(fmt.Sprintf("P4 command line - parsing error in %s\n out=%s", line, out))
	}
	fileHR := fields[0]
	fileWS := fields[1]

	fields = strings.Split(string(out), cue3) // Split to get section with line stats
	if len(fields) < 2 {
		return res, errors.New(fmt.Sprintf("1 - P4 command line - parsing error in out=%s\n", out))
	}
	// fmt.Printf("\n\n\nfields[1]\n%s\n\n",fields[1])

	lines := strings.Split(fields[1], "\n") // Get the section with line stats
	if len(lines) < 4 {
		return res, errors.New(fmt.Sprintf("2 - P4 command line - parsing error in %s\n out=%s\n", lines, out))
	}
	// fmt.Printf("\n\nlines[]\n%s\n%s\n%s\n",lines[1],lines[2],lines[3])

	/*
		add 3 chunks 8 lines
		deleted 2 chunks 7 lines
		changed 1 chunks 3 / 3 lines
	*/
	if (strings.Index(lines[1], "add") == -1) || (strings.Index(lines[2], "deleted") == -1) || (strings.Index(lines[3], "changed") == -1) {
		return res, errors.New(fmt.Sprintf("3 - P4 command line - parsing error in:\n%s\n%s\n%s\n out=%s\n", lines, out))
	}
	addLine := strings.Fields(lines[1])
	removeLine := strings.Fields(lines[2])
	changeLine := strings.Fields(lines[3])

	//fmt.Printf("addLine:%v\nremoveLine:%v\nchangeLine:%v\n",addLine,removeLine,changeLine)
	if (len(addLine) < 5) || (len(removeLine) < 5) || (len(changeLine) < 7) {
		return res, errors.New(fmt.Sprintf("4 - P4 command line - parsing error out=%s\n", out))
	}
	var err1, err2, err3 /*, err4 */ error
	addedLines, err1 := strconv.Atoi(addLine[3])
	removedLines, err2 := strconv.Atoi(removeLine[3])
	// changedLines1, err4 := strconv.Atoi(changeLine[3])
	changedLines, err3 := strconv.Atoi(changeLine[5])
	if (err1 != nil) || (err2 != nil) || (err3 != nil) /* || (err4 != nil) */ {
		return res, errors.New(fmt.Sprintf("5 - P4 command line - parsing error out=%s\n", out))
	}

	res.fileHR =				fileHR
	res.fileWS =				fileWS
	res.addedLines =		addedLines
	res.removedLines =	removedLines
	res.changedLines =	changedLines

	return res, nil
}



// CustomDiffHeadnWorkspace()
// 	Custom diff head rev and workspace in simplified form.
//
//  Simple algo to produce a view of the overall amount of changes (line count)
//	between a file in the workspace and its latest version in depot.
//
// 	There is no specific processing depending on encoding but works with utf8 and utf16.
//
//	p4 diff returns:
//			- the number of deleted and/or modified lines in previous version and,
//			- the number of added and/or modified lines in the version in workspace
//
//	A workspace name needs to be defined
//
//  If p.diffignorespace is set changes in spaces, tabs and line endings will be ignored.
//	However, works only with utf8 encoding.
//
// 	Input:
//		- File in depot to diff - p4 will automatically determine workspace path
//  Return:
//		- Name of file head rev
//		- Number of line file head rev
//		- Name file in workspace
//		- Number of line file file in workspace
//		- Added, deleted and modified number of lines
//		- Err code, nil if okay

func (p *Perforce) CustomDiffHeadnWorkspace(aFileInDepot string) (fileHR string, nbLinesHR int, fileWS string, nbLinesWS int, addedModLines int, removedModLines int, err error) {
	p.log(fmt.Sprintf("CustomDiffHeadnWorkspace(%s)\n", aFileInDepot))

	// Get head revision file
	tempHR, fileHR, err := p.GetFile(aFileInDepot, 0)
	p.log(fmt.Sprintf("	Head Rev=%s\n", fileHR))
	if err != nil {
		fmt.Printf("\nError getting head rev: %s %s\n", fileHR, err)
		os.Exit(1)
	}
	//tempName := tempHR.Name()
	tempf, err := os.Open(tempHR)
	if err != nil {
		fmt.Printf("\nError opening head rev: %s %s\n", tempHR, err)
		os.Exit(1)
	}
	defer tempf.Close()

	// Get workspace file
	fileWS, err = p.GetP4Where(aFileInDepot)
	if err != nil {
		fmt.Printf("\nError %s\n", err)
		os.Exit(1)
	}
	fileInWS, err := os.Open(fileWS) // File is directly accessible
	p.log(fmt.Sprintf("	File in WS=%s\n", fileWS))
	if err != nil {
		fmt.Printf("\nError accessing file in workspace: %s %s\n", fileWS, err)
		os.Exit(1)
	}
	defer fileInWS.Close()

	// Diff head revision and workspace file
	//  Read all head rev in a map [string]int
	m_lines := make(map[string]int)
	scanner := bufio.NewScanner(tempf)
	for scanner.Scan() {
		line := scanner.Text()
		if p.diffignorespace {
			line = strings.Trim(line, " \t\r\n")
		}
		m_lines[line]++
		nbLinesHR++
	}
	p.log(fmt.Sprintf("	Head rev file - nb lines read %d)\n", nbLinesHR))
	if err := scanner.Err(); err != nil {
		fmt.Printf("\nError parsing head rev file: %s %s\n", tempHR, err)
		os.Exit(1)
	}

	//	Read workspace and compare
	scanner = bufio.NewScanner(fileInWS)
	for scanner.Scan() {
		line := scanner.Text()
		if p.diffignorespace {
			line = strings.Trim(line, " \t\r\n")
		}
		if nb, ok := m_lines[line]; ok { // if line found
			if nb <= 0 {
				addedModLines++ // There are more occurrences of this line in new file
			} else {
				m_lines[line]--
			}
		} else { // if line not found
			addedModLines++ // This line didn't exist in old file
		}
		nbLinesWS++
	}
	p.log(fmt.Sprintf("	Workspace file - nb lines read %d)\n", nbLinesWS))

	// Check what's left in the map
	for _, v := range m_lines {
		removedModLines += v // Accrue here number of modified or deleted lines from headrev
	}

	// Delete temp head rev file
	tempf.Close()
	err = os.Remove(tempHR)
	if err != nil {
		p.log(fmt.Sprintf("Error deleting temp file %s %s)\n", tempHR, err))
	} // Non fatal error

	return fileHR, nbLinesHR, fileWS, nbLinesWS, addedModLines, removedModLines, nil
}
