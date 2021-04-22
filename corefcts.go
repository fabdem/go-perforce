package perforce

// Publicly available high level functions

import (
	// "bufio"
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


type T_WSDetails struct {
	Name			string
	Update			string
	Access			string
	Owner   		string
	Description		string
	Root			string
	Options			[]string
	SubmitOptions	[]string
	LineEnd			string
	View			map[string]string
}

// GetWorkspaceDetails()
//	Get workspace details from: p4 -w wwww -u xxxxx p4 client -o
// 	Input:
//		- workspace - optional if not present uses current workspace
//  Return:
//		- structure with details
//		- err code, nil if okay

func (p *Perforce) GetWorkspaceDetails(workspace string) (details T_WSDetails, err error) {
	p.log(fmt.Sprintf("GetWorkspaceDetails(%s)\n", workspace))

	var out []byte

	if len(workspace) <= 0 {
		workspace = p.workspace
	}

	if len(p.user) > 0 {
		// fmt.Printf(p4Cmd + " -u " + user + "-c" + workspace + " client -o\n")
		out, err = exec.Command(p.p4Cmd, "-u", p.user, "-c", workspace, "client", "-o").CombinedOutput()
		//fmt.Printf("P4 command line result - err=%s\n out=%s\n", err, out)
	} else {
		// fmt.Printf(p4Cmd + "-c" + workspace + " client -o\n")
		out, err = exec.Command(p.p4Cmd, "-c", workspace, "client", "-o").CombinedOutput()
		// out, err := exec.Command(p.p4Cmd, "info").Output()
	}
	if err != nil {
		return details, errors.New(fmt.Sprintf("P4 command line error %v  out=%s", err, out))
	}

	// Get the individual parameters
	pattern, err := regexp.Compile(`(?m).*^Client:(.*)\n\n^Update:(.*)\n\n^Access:(.*)\n\n^Owner:(.*)\n\n^Description:\n(.*)\n\n^Root:(.*)\n\n^Options:(.*)\n\n^SubmitOptions:(.*)\n\n^LineEnd:(.*)\n\n^View:\n\t(//.*) "?(//.*)"?`)
	if err != nil {
		return details, errors.New(fmt.Sprintf("regex compile error: %v", err))
	}

	matches := pattern.FindSubmatch(out)
	if len(matches) < 10 { // Not enough fields identified and parsed
		return details, errors.New(fmt.Sprintf("Error parsing: %s", out))
	}

	details.Name  			= strings.TrimLeft(string(matches[1])," \t")
	details.Update 			= strings.TrimLeft(string(matches[2])," \t")
	details.Access			= strings.TrimLeft(string(matches[3])," \t")
	details.Owner   		= strings.TrimLeft(string(matches[4])," \t")
	details.Description	= strings.TrimLeft(string(matches[5])," \t")
	details.Root				= strings.TrimLeft(string(matches[6])," \t")
	details.Options			= strings.Split(strings.TrimLeft(string(matches[7])," \t"), " ")
	details.SubmitOptions	= strings.Split(strings.TrimLeft(string(matches[8])," \t"), " ")

	// Get the list of files depot/workspace
	// First find the index of the begining of the list dans le Buffer
	pattern, err = regexp.Compile(`(?m).*^(View:\n)\t//.* "?//.*"?`)
	if err != nil {
		return details, errors.New(fmt.Sprintf("regex compile error: %v", err))
	}
	idxs := pattern.FindSubmatchIndex(out)
	if len(idxs) < 3 {
		return details, errors.New(fmt.Sprintf("Parsing workspace error - can't find list of depot/ws files"))
	}
	fmt.Printf("match=%s\n", out[idxs[2]:idxs[3]])

	out = out[idxs[3]:]  // Keep list of of depot/ws files only - trash everything before

	// Get all the pairs depot/ws files
	pattern, err = regexp.Compile(`(?m).*^\t(//.*) "?(//[^"\n]*)`)
	if err != nil {
		return details, errors.New(fmt.Sprintf("regex compile error: %v", err))
	}
	list := pattern.FindAllSubmatch(out,-1)
	fmt.Printf("len(list)=%d   len(list[0])=%d    len(list[1])=%d\n", len(list),len(list[0]), len(list[1]))
	fmt.Printf("list[0]=%s\n", list[0])
	fmt.Printf("list[0][0]=%s\n", list[0][0])
	fmt.Printf("list[0][1]=%s\n", list[0][1])
	fmt.Printf("list[0][2]=%s\n", list[0][2])
	fmt.Printf("list[1][0]=%s\n", list[1][0])
	fmt.Printf("list[1][1]=%s\n", list[1][1])
	fmt.Printf("list[1][2]=%s\n", list[1][2])


	//fmt.Printf("details=%v\n", details)
	return details, nil
}
