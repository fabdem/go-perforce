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
	p.logThis(fmt.Sprintf("\nGetP4Where(%s)", depotFile))

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
	p.logThis(fmt.Sprintf("	Response=%s", string(out)))

	// Parse result
	fields := strings.Split(string(out), "... path ")
	if len(fields) < 2 {
		return fileName, errors.New(fmt.Sprintf("p4 command line parsing result error %s - %s ", err, fields))
	}
	fileName = fields[1]
	fileName = strings.Trim(fields[1], "\r\n")
	p.logThis(fmt.Sprintf("	filename=%s", fileName))

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
	p.logThis(fmt.Sprintf("\nGetFile(%s, %d)",depotFile,rev))

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

	p.logThis(fmt.Sprintf("	fileName=%s rev=%d", fileName, rev))

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
	p.logThis(fmt.Sprintf("\nGetHeadRev(%s)",depotFileName))

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

	p.logThis(fmt.Sprintf("	received from P4: %s", out))

	// Read version
	// e.g. //Project/dev/localization/afile_bulgarian.txt#8 - edit change 4924099 (utf16)
	idxBeg := strings.LastIndex(string(out), "#") + len("#")
	idxEnd := strings.LastIndex(string(out), " - ")
	// Check response to prevent out of bound index
	if idxBeg <= -1 || idxEnd <= -1 || idxBeg >= idxEnd {
		return 0, errors.New(fmt.Sprintf("Format error in P4 response: %s  %v", string(out), err))
	}
	sRev := string(out[idxBeg:idxEnd])

	p.logThis(fmt.Sprintf("	Revision: %s", sRev))

	rev, err = strconv.Atoi(sRev) // Check format
	if err != nil {
		return 0, errors.New(fmt.Sprintf("Format error conv to number: %v", err))
	}

	return rev, nil
}

// GetCLContent()
//	Get content from a Change List
//	Do a: p4 -uxxxxx describe -s 6102201
// 	Input:
//		- Change List number
//  Return:
//		- a map of files (depot path and name) and rev numbers
//		- CL's user and workspace for sanity check
//		- err code, nil if okay
/*
p4 -uxxxx describe -s 6102201
Change 6102201 by xxxx@yyyyyyyy on 2020/09/20 21:02:41 *pending*
	Test diff
Affected files ...
... //zzzzzz/dev/locScriptTesting/main_french.json#1 edit
... //zzzzzz/dev/locScriptTesting/yy_french.txt#18 edit
... //zzzzzz/dev/locScriptTesting/yy_german.txt#8 edit
*/
type T_CLFileDetails struct {
	Rev 		int
	Action 	string
}
type T_CLDetails struct {
	CLNb				int
	User				string
	Workspace		string
	DateStamp		string
	Pending			bool
	Comment			string
	List				map[string] T_CLFileDetails
}

func (p *Perforce) GetCLContent(changeList int) (details T_CLDetails, err error) {
	p.logThis(fmt.Sprintf("\nGetCLContent(%d),changeList"))

	var out []byte

	if len(p.user) > 0 {
		// fmt.Printf(p4Cmd + " -u " + user + " describe " + "-s " + strconv.Itoa(changeList) + "\n")
		out, err = exec.Command(p.p4Cmd, "-u", p.user, "describe", "-s", strconv.Itoa(changeList)).CombinedOutput()
		//fmt.Printf("P4 command line result - err=%s\n out=%s\n", err, out)
	} else {
		//fmt.Printf(p.p4Cmd  + " describe " + "-s " + strconv.Itoa(changeList) + "\n")
		out, err = exec.Command(p.p4Cmd, "describe", "-s", strconv.Itoa(changeList)).CombinedOutput()
		// out, err := exec.Command(p.p4Cmd, "info").Output()
	}
	if err != nil {
		return details, errors.New(fmt.Sprintf("P4 command line error %v  out=%s", err, out))
	}

	// Parse response
	pattern, err := regexp.Compile(`(?m)^Change ([0-9]*) by ([^ @]*)@([^ @]*) on ([0-9/]* [0-9:]*)([a-z\* ]*)[\r\n]^((.|\r|\n)*[\r\n])^Affected files ...[\r\n][\r\n]... //`)
	if err != nil {
		return details, errors.New(fmt.Sprintf("Regex compile error: %v", err))
	}

	matches := pattern.FindSubmatch(out)
	if len(matches) < 6 { // Not enough fields identified and parsed
		return details, errors.New(fmt.Sprintf("Error parsing - nb field read: %d received from p4: %s", len(matches), out))
	}

	// Record CL global details
	details.CLNb, err = strconv.Atoi(string(matches[1]))
	if err != nil {
		return details, errors.New(fmt.Sprintf("Error parsing - Format error conv to number: %v", err))
	}
	details.User = strings.Trim(string(matches[2]), " \r\n\t")
	details.Workspace = strings.Trim(string(matches[3]), " \r\n\t")
	details.DateStamp = strings.Trim(string(matches[4]), " \r\n\t")
	if strings.Trim(string(matches[5]), " \r\n\t") == "*pending*" {
		details.Pending = true
	}
	details.Comment = strings.Trim(string(matches[6]), " \r\n\t")

	// Strip beginning of the response now that we've retrieved what we needed
	pattern, err = regexp.Compile(`(?m)^Affected files \.\.\.[\r\n][\r\n](\.\.\.) //`)
	if err != nil {
		return details, errors.New(fmt.Sprintf("regex compile error: %v", err))
	}
	idxs := pattern.FindSubmatchIndex(out)
	if len(idxs) < 3 {
		return details, errors.New(fmt.Sprintf("Parsing CL - can't find a list of files"))
	}
	// fmt.Printf("match=%s\n", out[idxs[2]:idxs[3]])

	out = out[idxs[2]:] // Keep list of files only - trash everything before

	// Get all the files
	pattern, err = regexp.Compile(`(?m)^\.\.\. (//.*)#([0-9]*) ([a-z]*)$`)
	if err != nil {
		return details, errors.New(fmt.Sprintf("regex compile error: %v", err))
	}
	list := pattern.FindAllSubmatch(out, -1)

	// Check result validity
	if len(list) > 0 {
		if len(list[0]) < 4 {
			return details, errors.New(fmt.Sprintf("Parsing workspace error - reading files in CL incorrect: %s", out))
		}
		// Get results in map
		details.List = make(map[string] T_CLFileDetails)
		for _, v := range list {
			rev, err := strconv.Atoi(string(v[2]))
			if err != nil {
				return details, errors.New(fmt.Sprintf("Error parsing - Format error conv to number: %v", err))
			}
			filename := strings.Trim(string(v[1]), " \t\r\n")
			action :=  strings.Trim(string(v[3]), " \t\r\n")
			details.List[filename] = T_CLFileDetails {Rev: rev, Action: action}
		}
	}

	return details, nil
}


// GetPendingCLContent() DEPRECATED USE GetCLContent()INSTEAD
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
	p.logThis(fmt.Sprintf("\nGetChangeListContent(%d),changeList"))

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

// GetFileInDepotDetails()
//	Get the details from a file in the depot from: p4 -c wwww -u xxxxx p4 filelog -m 1
//  User and workspace don't seem to be necessary but leaving them anyway
//	We get a truncated version of the comments (no -l or -L). They are ' delimited so safer to parse that way.
// 	Input:
//		- path to file in depot
//  Return:
//		- structure with details
//		- err code, nil if okay

type T_FileDetails struct {
	Path        string
	LastVersion int
	CL          int
	Action			string
	EditDate    string
	Owner       string
	Workspace   string
	Type        string
	Comment     string // Short version truncated to 31 characters
}

func (p *Perforce) GetFileInDepotDetails(FileInDepot string) (details T_FileDetails, err error) {
	p.logThis(fmt.Sprintf("\nGetFileInDepotDetails(%s)", FileInDepot))

	var out []byte

	if len(p.user) > 0 {
		// fmt.Printf(p4Cmd + " -u " + user + "-c" + p.workspace + " filelog -m 1 ",FileInDepot,"\n")
		out, err = exec.Command(p.p4Cmd, "-u", p.user, "-c", p.workspace, "filelog", "-m 1", FileInDepot).CombinedOutput()
		//fmt.Printf("P4 command line result - err=%s\n out=%s\n", err, out)
	} else {
		// fmt.Printf(p4Cmd + "-c" + workspace + " filelog -m 1 ",FileInDepot,"\n")
		out, err = exec.Command(p.p4Cmd, "-c", p.workspace, "filelog", "-m 1", FileInDepot).CombinedOutput()
		// out, err := exec.Command(p.p4Cmd, "info").Output()
	}
	if err != nil {
		return details, errors.New(fmt.Sprintf("P4 command line error %v  out=%s", err, out))
	}

	// Get the individual parameters
	pattern, err := regexp.Compile(`(?m)^(//.*)$[\n\r]*^... #([0-9]*) change ([0-9]*) ([a-z]*) on ([0-9/]*) by ([^@]*)@([^ ]*) \((.*)\) '(.*)'`)
	if err != nil {
		return details, errors.New(fmt.Sprintf("regex compile error: %v", err))
	}

	matches := pattern.FindSubmatch(out)
	if len(matches) < 10 { // Not enough fields identified and parsed
		return details, errors.New(fmt.Sprintf("Error parsing - nb field read: %d received from p4: %s", len(matches), out))
	}

	details.Path = strings.Trim(string(matches[1]), " \r\n\t")
	if details.Path != FileInDepot {
		return details, errors.New(fmt.Sprintf("Error parsing - wrong file details returned by p4: %s", details.Path))
	}
	details.LastVersion, err = strconv.Atoi(string(matches[2]))
	if err != nil {
		return details, errors.New(fmt.Sprintf("Error parsing - Format error conv to number: %v", err))
	}
	details.CL, err = strconv.Atoi(string(matches[3]))
	if err != nil {
		return details, errors.New(fmt.Sprintf("Error parsing - Format error conv to number: %v", err))
	}
	details.Action = strings.Trim(string(matches[4]), " \r\n\t")
	details.EditDate = strings.Trim(string(matches[5]), " \r\n\t")
	details.Owner = strings.Trim(string(matches[6]), " \r\n\t")
	details.Workspace = strings.Trim(string(matches[7]), " \r\n\t")
	details.Type = strings.Trim(string(matches[8]), " \r\n\t")
	details.Comment = strings.Trim(string(matches[9]), " \r\n\t") // We get a truncated to 31 characters version

	return details, nil
}

// GetWorkspaceDetails()
//	Get workspace details from: p4 -c wwww -u xxxxx p4 client -o
// 	Input:
//		- workspace - optional if not present uses current workspace
//  Return:
//		- structure with details
//		- err code, nil if okay

type T_WSDetails struct {
	Name          string
	Update        string
	Access        string
	Owner         string
	Description   string
	Root          string
	Options       []string
	SubmitOptions []string
	LineEnd       string
	View          map[string]string
}

func (p *Perforce) GetWorkspaceDetails(workspace string) (details T_WSDetails, err error) {
	p.logThis(fmt.Sprintf("\nGetWorkspaceDetails(%s)", workspace))

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
	pattern, err := regexp.Compile(`(?m).*^Client:(.*)[\n\r]*^Update:(.*)[\n\r]*^Access:(.*)[\n\r]*^Owner:(.*)[\n\r]*^Description:[\n\r]{1,2}(.*)[\n\r]*^Root:(.*)[\n\r]*^Options:(.*)[\n\r]*^SubmitOptions:(.*)[\n\r]*^LineEnd:(.*)[\n\r]*^View:[\n\r]*\t(//.*) "?(//.*)"?`)
	if err != nil {
		return details, errors.New(fmt.Sprintf("regex compile error: %v", err))
	}

	matches := pattern.FindSubmatch(out)
	if len(matches) < 10 { // Not enough fields identified and parsed
		return details, errors.New(fmt.Sprintf("Error parsing - nb field read: %d received from p4: %s", len(matches), out))
	}

	details.Name = strings.Trim(string(matches[1]), " \t\r\n")
	details.Update = strings.Trim(string(matches[2]), " \t\r\n")
	details.Access = strings.Trim(string(matches[3]), " \t\r\n")
	details.Owner = strings.Trim(string(matches[4]), " \t\r\n")
	details.Description = strings.Trim(string(matches[5]), " \t\r\n")
	details.Root = strings.Trim(string(matches[6]), " \t\r\n")
	details.Options = strings.Split(strings.Trim(string(matches[7]), " \t\r\n"), " ")
	details.SubmitOptions = strings.Split(strings.Trim(string(matches[8]), " \t\r\n"), " ")

	// Get the list of files depot/workspace
	// First find the index of the begining of the list dans le Buffer
	pattern, err = regexp.Compile(`(?m).*^(View:[\n\r]*)\t//.* "?//.*"?`)
	if err != nil {
		return details, errors.New(fmt.Sprintf("regex compile error: %v", err))
	}
	idxs := pattern.FindSubmatchIndex(out)
	if len(idxs) < 3 {
		return details, errors.New(fmt.Sprintf("Parsing workspace error - can't find list of depot/ws files"))
	}
	// fmt.Printf("match=%s\n", out[idxs[2]:idxs[3]])

	out = out[idxs[3]:] // Keep list of of depot/ws files only - trash everything before

	// Get all the pairs depot/ws files
	pattern, err = regexp.Compile(`(?m).*^\t(//.*) "?(//[^"\n]*)`)
	if err != nil {
		return details, errors.New(fmt.Sprintf("regex compile error: %v", err))
	}
	list := pattern.FindAllSubmatch(out, -1)

	// Check result validity
	if len(list) > 0 {
		if len(list[0]) < 3 {
			return details, errors.New(fmt.Sprintf("Parsing workspace error - reading pair depot/workspace file incorrect: %s", out))
		}
		// Get results in a map
		details.View = make(map[string]string)
		for _, v := range list {
			details.View[strings.Trim(string(v[1]), " \t\r\n")] = strings.Trim(string(v[2]), " \t\r\n")
		}
	}

	return details, nil
}
