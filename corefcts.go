package perforce

// Publicly available high level functions

import (
	"fmt"
	"io"
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
	p.logThis(fmt.Sprintf("GetP4Where(%s)", depotFile))

	var out []byte

	if len(p.user) > 0 {
		out, err = exec.Command(p.p4Cmd, "-ztag", "-u", p.user, "-c", p.workspace, "where", depotFile).CombinedOutput()
		// fmt.Printf("P4 command line result - %s\n %s\n", err, out)
	} else {
		out, err = exec.Command(p.p4Cmd, "-ztag", "-c", p.workspace, "where", depotFile).CombinedOutput()
	}
	if err != nil {
		return fileName, fmt.Errorf("p4 command line error %s - %s ", err, out)
	}
	p.logThis(fmt.Sprintf("	Response=%s", string(out)))

	// Parse result
	fields := strings.Split(string(out), "... path ")
	if len(fields) < 2 {
		return fileName, fmt.Errorf("p4 command line parsing result error %s - %s ", err, fields)
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
	p.logThis(fmt.Sprintf("GetFile(%s, %d)", depotFile, rev))

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
		return tempFile, fileName, fmt.Errorf("Unable to create a temp file - %v", err)
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
		return tempFile, fileName, fmt.Errorf("p4 command line error %s - %s ", err, out)
	}

	// 2EME PROBLEME POURQUOI CA PANIQUE SI ERROR DS GETFILE (REMOVE USER)

	// Unfortunately p4 print status in linux is not reliable.
	// err != nil when syntax err but not if file doesn't exist.
	// So manually checking if a file was created:
	if _, err = os.Stat(tempFile); err != nil {
		if os.IsNotExist(err) { // file does not exist
			return tempFile, fileName, fmt.Errorf("P4 no file created %v - %v ", out, err)
		} else { // Can't get file stat
			return tempFile, fileName, fmt.Errorf("Can't access the status of file produced %v - %v ", out, err)
		}
	}
	return tempFile, fileName, nil // everything is fine returns file and file name
}

type T_FilesProperties struct {
	DepotfileLoc string
	HeadRevision int
	Action       string // Action taken at hte head
	ChangeList   int
	FileType     string
}

// GetP4Files()
//	Get all the info returned by "p4 files" in a slice.
//	Exclude deleted, purged, or archived files. The files that remain
//	are those available for syncing or integration.
// 	depotFilePattern: file path and name or pattern in P4.
//										May return several matches.
//  Returns a slice with 1 line of details per file. If empty, means no match. Doesn't return an error!
func (p *Perforce) GetP4Files(depotFilePattern string) (properties []T_FilesProperties, err error) {
	p.logThis(fmt.Sprintf("GetP4Files(%s)", depotFilePattern))

	var out []byte

	if len(p.user) > 0 {
		// fmt.Printf(p4Cmd + " -u " + user + " files -e" + " " + depotFile + "\n")
		out, err = exec.Command(p.p4Cmd, "-u", p.user, "files", "-e", depotFilePattern).CombinedOutput()
		//fmt.Printf("P4 command line result - err=%s\n out=%s\n", err, out)
	} else {
		//fmt.Printf(p.p4Cmd  + " files -e" + " " + depotFileName + "\n")
		out, err = exec.Command(p.p4Cmd, "files", "-e", depotFilePattern).CombinedOutput()
		// out, err := exec.Command(p.p4Cmd, "info").Output()
	}
	if err != nil {
		return properties, fmt.Errorf("P4 command line error %v  out=%s", err, out)
	}

	p.logThis(fmt.Sprintf("	received from P4: %s", out))

	// Parse response
	if strings.HasSuffix(strings.TrimRight(string(out), "\t\r\n "), "no such file(s).") {
		return properties, nil // If p4 returns that file not found, skip the rest and return empty list but no error.
	}

	pattern, err := regexp.Compile(`(?m)^(//.*)#([0-9]*) - ([a-z]*) change ([0-9]*) \((.*)\)[\r\n]*`)
	if err != nil {
		return properties, fmt.Errorf("Regex compile error: %v", err)
	}

	list := pattern.FindAllSubmatch(out, -1)

	for _, line := range list {
		if len(line) < 6 {
			return properties, fmt.Errorf("Parsing error - %d field found in %s ", len(line), line)
		}

		var det T_FilesProperties
		det.DepotfileLoc = string(line[1])
		rev, err := strconv.Atoi(string(line[2])) // Check format
		if err != nil {
			return properties, fmt.Errorf("Format error conv to number: %v", err)
		}
		det.HeadRevision = rev
		det.Action = string(line[3])
		cl, err := strconv.Atoi(string(line[4])) // Check format
		if err != nil {
			return properties, fmt.Errorf("Format error conv to number: %v", err)
		}
		det.ChangeList = cl
		det.FileType = string(line[5])

		properties = append(properties, det)
	}

	return properties, nil
}

// GetHeadRev()
//	Get from P4 the head revision number of a file from depot
// 	depotFileName: file path and name in P4
//	If file is not found returns rev negative
//	err not nil if processing error
func (p *Perforce) GetHeadRev(depotFileName string) (rev int, err error) {
	p.logThis(fmt.Sprintf("GetHeadRev(%s)", depotFileName))

	res, err := p.GetP4Files(depotFileName)

	if len(res) > 0 {
		rev = res[0].HeadRevision
	} else {
		if err != nil {
			rev = -1 // File not found
		}
	}

	return rev, err
}

// CheckFileExitsInDepot()
//	Check if a path exists in the depot.
// 	depotFileName: file path and name in P4
//	Returns a boolean and err.
func (p *Perforce) CheckFileExitsInDepot(depotFileName string) (exists bool, err error) {
	p.logThis(fmt.Sprintf("CheckFileExitsInDepot(%s)", depotFileName))

	res, err := p.GetP4Files(depotFileName)

	if len(res) > 0 {
		exists = true
	}

	return exists, err
}

// GetCLContent()
//	Get content from a Change List
//	Do a: p4 -uxxxxx describe -s 6102201
// 	Input:
//		- Change List number
//  Return:
//		- a map of files (depot path and name) and rev numbers or nil if CL empty
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
type T_CLFileProperties struct {
	Rev    int
	Action string
}
type T_CLProperties struct {
	CLNb      int
	User      string
	Workspace string
	DateStamp string
	Pending   bool
	Comment   string
	List      map[string]T_CLFileProperties // map file path/name and properties
}

func (p *Perforce) GetCLContent(changeList int) (properties T_CLProperties, err error) {
	p.logThis(fmt.Sprintf("GetCLContent(%d)", changeList))

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
		return properties, fmt.Errorf("P4 command line error %v  out=%s", err, out)
	}

	// Parse response
	if strings.HasSuffix(strings.TrimRight(string(out), "\t\r\n "), "no such changelist.") {
		return properties, fmt.Errorf("No such change list: %d", changeList) // If p4 returns that cl does not exit, skip the rest and returns an error.
	}

	pattern, err := regexp.Compile(`(?m)^Change ([0-9]*) by ([^ @]*)@([^ @]*) on ([0-9/]* [0-9:]*)([a-z\* ]*)[\r\n]*^((.|\r|\n)*[\r\n]*)^Affected files ...[\r\n]*`)
	if err != nil {
		return properties, fmt.Errorf("Regex compile error: %v", err)
	}

	matches := pattern.FindSubmatch(out)
	if len(matches) < 6 { // Not enough fields identified and parsed
		return properties, fmt.Errorf("Error parsing - nb field read: %d received from p4: %s", len(matches), out)
	}

	// Record CL global properties
	properties.CLNb, err = strconv.Atoi(string(matches[1]))
	if err != nil {
		return properties, fmt.Errorf("Error parsing - Format error conv to number: %v", err)
	}

	properties.User = strings.Trim(string(matches[2]), " \r\n\t")
	properties.Workspace = strings.Trim(string(matches[3]), " \r\n\t")
	properties.DateStamp = strings.Trim(string(matches[4]), " \r\n\t")
	if strings.Trim(string(matches[5]), " \r\n\t") == "*pending*" {
		properties.Pending = true
	}
	properties.Comment = strings.Trim(string(matches[6]), " \r\n\t")

	// Strip beginning of the response now that we've retrieved what we needed
	pattern, err = regexp.Compile(`(?m)^Affected files \.\.\.[\r\n]*(\.\.\.) //`)
	if err != nil {
		return properties, fmt.Errorf("regex compile error: %v", err)
	}
	idxs := pattern.FindSubmatchIndex(out)
	if len(idxs) < 3 {
		// return properties, fmt.Errorf("Parsing CL - can't find a list of files")
		return properties, nil // Empty change list
	}
	// fmt.Printf("match=%s\n", out[idxs[2]:idxs[3]])

	out = out[idxs[2]:] // Keep list of files only - trash everything before

	// Get all the files
	pattern, err = regexp.Compile(`(?m)^\.\.\. (//.*)#([0-9]*) ([a-z]*)[\r\n]*`)
	if err != nil {
		return properties, fmt.Errorf("regex compile error: %v", err)
	}
	list := pattern.FindAllSubmatch(out, -1)

	p.logThis(fmt.Sprintf(" Nb files in CL: %d", len(list)))

	// Check result validity
	if len(list) > 0 {
		if len(list[0]) < 4 {
			return properties, fmt.Errorf("Parsing workspace error - reading files in CL incorrect: %s", out)
		}
		// Get results in map
		properties.List = make(map[string]T_CLFileProperties)
		for _, v := range list {
			rev, err := strconv.Atoi(string(v[2]))
			if err != nil {
				return properties, fmt.Errorf("Error parsing - Format error conv to number: %v", err)
			}
			filename := strings.Trim(string(v[1]), " \t\r\n")
			action := strings.Trim(string(v[3]), " \t\r\n")
			properties.List[filename] = T_CLFileProperties{Rev: rev, Action: action}
		}
	}

	// Check that we received the correct CL - returns the properties even if wrong
	if changeList != properties.CLNb {
		return properties, fmt.Errorf("Perforce error - Wrong change list data received: %d, %v", properties.CLNb, properties)
	}

	return properties, nil
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
	p.logThis(fmt.Sprintf("GetPendingCLContent(%d)", changeList))

	m_files = make(map[string]int)

	res, err := p.GetCLContent(changeList)
	if err == nil {
		for k, v := range res.List {
			m_files[k] = v.Rev
		}
		user = res.User
		workspace = res.Workspace
	}
	return m_files, user, workspace, err
}

// GetFileInDepotProperties()
//	Get the properties from a file in the depot from: p4 -c wwww -u xxxxx p4 filelog -m 1
//  User and workspace don't seem to be necessary but leaving them anyway
//	We get a truncated version of the comments (no -l or -L). They are ' delimited so safer to parse that way.
// 	Input:
//		- path to file in depot
//  Return:
//		- structure with properties
//		- err code, nil if okay

type T_FileProperties struct {
	Path        string
	LastVersion int
	CL          int
	Action      string
	EditDate    string
	Owner       string
	Workspace   string
	Type        string
	Comment     string // Short version truncated to 31 characters
}

func (p *Perforce) GetFileInDepotProperties(FileInDepot string) (properties T_FileProperties, err error) {
	p.logThis(fmt.Sprintf("GetFileInDepotProperties(%s)", FileInDepot))

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
		return properties, fmt.Errorf("P4 command line error %v  out=%s", err, out)
	}

	// Get the individual parameters
	pattern, err := regexp.Compile(`(?m)^(//.*)$[\n\r]*^... #([0-9]*) change ([0-9]*) ([a-z]*) on ([0-9/]*) by ([^@]*)@([^ ]*) \((.*)\) '(.*)'`)
	if err != nil {
		return properties, fmt.Errorf("regex compile error: %v", err)
	}

	matches := pattern.FindSubmatch(out)
	if len(matches) < 10 { // Not enough fields identified and parsed
		return properties, fmt.Errorf("Error parsing - nb field read: %d received from p4: %s", len(matches), out)
	}

	properties.Path = strings.Trim(string(matches[1]), " \r\n\t")
	if properties.Path != FileInDepot {
		return properties, fmt.Errorf("Error parsing - wrong file properties returned by p4: %s", properties.Path)
	}
	properties.LastVersion, err = strconv.Atoi(string(matches[2]))
	if err != nil {
		return properties, fmt.Errorf("Error parsing - Format error conv to number: %v", err)
	}
	properties.CL, err = strconv.Atoi(string(matches[3]))
	if err != nil {
		return properties, fmt.Errorf("Error parsing - Format error conv to number: %v", err)
	}
	properties.Action = strings.Trim(string(matches[4]), " \r\n\t")
	properties.EditDate = strings.Trim(string(matches[5]), " \r\n\t")
	properties.Owner = strings.Trim(string(matches[6]), " \r\n\t")
	properties.Workspace = strings.Trim(string(matches[7]), " \r\n\t")
	properties.Type = strings.Trim(string(matches[8]), " \r\n\t")
	properties.Comment = strings.Trim(string(matches[9]), " \r\n\t") // We get a truncated to 31 characters version

	return properties, nil
}

// GetWorkspaceProperties()
//	Get workspace properties from: p4 -c wwww -u xxxxx p4 client -o
// 	Input:
//		- workspace - optional if not present uses current workspace
//  Return:
//		- structure with properties
//		- err code, nil if okay

type T_WSProperties struct {
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

func (p *Perforce) GetWorkspaceProperties(workspace string) (properties T_WSProperties, err error) {
	p.logThis(fmt.Sprintf("GetWorkspaceProperties(%s)", workspace))

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
		return properties, fmt.Errorf("P4 command line error %v  out=%s", err, out)
	}

	// Get the individual parameters
	pattern, err := regexp.Compile(`(?m).*^Client:(.*)[\n\r]*^Update:(.*)[\n\r]*^Access:(.*)[\n\r]*^Owner:(.*)[\n\r]*^Description:[\n\r]{1,2}(.*)[\n\r]*^Root:(.*)[\n\r]*^Options:(.*)[\n\r]*^SubmitOptions:(.*)[\n\r]*^LineEnd:(.*)[\n\r]*^View:[\n\r]*\t(//.*) "?(//.*)"?`)
	if err != nil {
		return properties, fmt.Errorf("regex compile error: %v", err)
	}

	matches := pattern.FindSubmatch(out)
	if len(matches) < 10 { // Not enough fields identified and parsed
		return properties, fmt.Errorf("Error parsing - nb field read: %d received from p4: %s", len(matches), out)
	}

	properties.Name = strings.Trim(string(matches[1]), " \t\r\n")
	properties.Update = strings.Trim(string(matches[2]), " \t\r\n")
	properties.Access = strings.Trim(string(matches[3]), " \t\r\n")
	properties.Owner = strings.Trim(string(matches[4]), " \t\r\n")
	properties.Description = strings.Trim(string(matches[5]), " \t\r\n")
	properties.Root = strings.Trim(string(matches[6]), " \t\r\n")
	properties.Options = strings.Split(strings.Trim(string(matches[7]), " \t\r\n"), " ")
	properties.SubmitOptions = strings.Split(strings.Trim(string(matches[8]), " \t\r\n"), " ")

	// Get the list of files depot/workspace
	// First find the index of the begining of the list dans le Buffer
	pattern, err = regexp.Compile(`(?m).*^(View:[\n\r]*)\t//.* "?//.*"?`)
	if err != nil {
		return properties, fmt.Errorf("regex compile error: %v", err)
	}
	idxs := pattern.FindSubmatchIndex(out)
	if len(idxs) < 3 {
		return properties, fmt.Errorf("Parsing workspace error - can't find list of depot/ws files")
	}
	// fmt.Printf("match=%s\n", out[idxs[2]:idxs[3]])

	out = out[idxs[3]:] // Keep list of of depot/ws files only - trash everything before

	// Get all the pairs depot/ws files
	pattern, err = regexp.Compile(`(?m).*^\t(//.*) "?(//[^"\n\n]*)`)
	if err != nil {
		return properties, fmt.Errorf("regex compile error: %v", err)
	}
	list := pattern.FindAllSubmatch(out, -1)

	// Check result validity
	if len(list) > 0 {
		if len(list[0]) < 3 {
			return properties, fmt.Errorf("Parsing workspace error - reading pair depot/workspace file incorrect: %s", out)
		}
		// Get results in a map
		properties.View = make(map[string]string)
		for _, v := range list {
			properties.View[strings.Trim(string(v[1]), " \t\r\n")] = strings.Trim(string(v[2]), " \t\r\n")
		}
	}

	return properties, nil
}

// Changelist specification definiton:
type T_CLSpecProperties struct {
	ChangeList  int    // The change list number. (-1) on a new changelist.
	Date        string // The date this specification was last modified.
	Client      string // The client (workspace) on which the changelist was created.  Read-only.
	User        string // The user who created the changelist.
	Status      string // Either 'pending' or 'submitted'. Read-only. Or 'new'!!
	Type        string // Either 'public' or 'restricted'. Default is 'public'.
	Description string // Comments about the changelist.  Required.
	// can't test: ImportedBy		string			// The user who fetched or pushed this change to this server.
	// can't test: Identity			string			// Identifier for this change.
	// can't test: Jobs					[]string		// What opened jobs are to be closed by this changelist.
	// You may delete jobs from this list.  (New changelists only.)
	// can't test: Stream				[]string  	// What opened stream is to be added to this changelist.
	// You may remove an opened stream from this list.
	Files map[string]string // File/action. What opened files from the default changelist are to be added
	// to this changelist.  You may delete files from this list.
	// (New changelists only.)
	Eol 				string // Detect what kind of end of line we receive from the server.
	Form 				string // Form as it was received
}


// GetCLSpecProperties()
//	Get a CL specification properties from a p4 change -o command.
//	Probably the main use of this function: if cl == 0
//	then moves default changelist into a numbered changelist.
//
func (p *Perforce) GetCLSpecProperties(cl int) (properties T_CLSpecProperties, err error) {
	p.logThis(fmt.Sprintf("GetCLSpecProperties(%d)", cl))

	var out []byte

	var sCL string
	if cl > 0 {
		sCL = strconv.Itoa(cl)
	}

	if len(p.user) > 0 {
		out, err = exec.Command(p.p4Cmd, "-u", p.user, "-c", p.workspace, "change", "-o", sCL).CombinedOutput()
	} else {
		out, err = exec.Command(p.p4Cmd, "-c", p.workspace, "change", "-o", sCL).CombinedOutput()
	}
	if err != nil {
		return properties, fmt.Errorf("P4 command line error %v  out=%s", err, out)
	}

	// Parsing result

	// Detect what kind of end of line we receive from the perforce server
	if strings.Count(string(out),"\n") >= strings.Count(string(out),"\r\n") {
		properties.Eol = "\n"
	} else {
		properties.Eol = "\r\n"
	}

	// Get Change list number - check 1st if it's a 'new' changelist
	pattern, err := regexp.Compile(`(?m)[\n\r]*^Change:\t(new|[0-9]*)[\n\r]*`)
	if err != nil {
		return properties, fmt.Errorf("Regex (changelist#) compile error: %v", err)
	}

	matches := pattern.FindSubmatch(out)
	if len(matches) > 0 {
		if string(matches[1]) != "new" {
			properties.ChangeList, err = strconv.Atoi(string(matches[1]))
			if err != nil {
				return properties, fmt.Errorf("Parsing changelist# error %v  out=%s", err, out)
			}
		} else {
			properties.ChangeList = -1  // 'new' changelist
		}
	}

	// Get Date
	pattern, err = regexp.Compile(`(?m)[\n\r]*^Date:\t([0-9/ :]*)[\n\r]*`)
	if err != nil {
		return properties, fmt.Errorf("Regex (date) compile error: %v", err)
	}
	matches = pattern.FindSubmatch(out)
	if len(matches) > 0 {
		properties.Date = string(matches[1])
	}
	// Get Client (workspace)
	pattern, err = regexp.Compile(`(?m)[\n\r]*^Client:\t(.*)[\n\r]*`)
	if err != nil {
		return properties, fmt.Errorf("Regex (workspace) compile error: %v", err)
	}
	matches = pattern.FindSubmatch(out)
	if len(matches) > 0 {
		properties.Client = string(matches[1])
	}

	// Get User
	pattern, err = regexp.Compile(`(?m)[\n\r]*^User:\t(.*)[\n\r]*`)
	if err != nil {
		return properties, fmt.Errorf("Regex (user) compile error: %v", err)
	}
	matches = pattern.FindSubmatch(out)
	if len(matches) > 0 {
		properties.User = string(matches[1])
	}

	// Get Status
	pattern, err = regexp.Compile(`(?m)[\n\r]*^Status:\t(new|pending|submitted)[\n\r]*`)
	if err != nil {
		return properties, fmt.Errorf("Regex (status) compile error: %v", err)
	}
	matches = pattern.FindSubmatch(out)
	if len(matches) > 0 {
		properties.Status = string(matches[1])
	}

	// Get Type
	pattern, err = regexp.Compile(`(?m)[\n\r]*^Type:\t(public|restricted)[\n\r]*`)
	if err != nil {
		return properties, fmt.Errorf("Regex (type) compile error: %v", err)
	}
	matches = pattern.FindSubmatch(out)
	if len(matches) > 0 {
		properties.Type = string(matches[1])
	}

	// Get Description
	pattern, err = regexp.Compile(`(?m)[\n\r]*^Description:[\n\r]*((^\t.*[\n\r]*)*[\n\r]*)^[\n\r]*`)
	if err != nil {
		return properties, fmt.Errorf("Regex (description) compile error: %v", err)
	}

	matches = pattern.FindSubmatch(out)
	if len(matches) > 0 {
		str := strings.TrimPrefix(string(matches[1]), "\t") // Remove the leading tabs
		str  = strings.ReplaceAll(str, "\r\n\t", "\r\n")		// windows
		str  = strings.ReplaceAll(str, "\n\t", "\n")				// linux
		properties.Description = str
	}

	// Get Files
	pattern, err = regexp.Compile(`(?m)[\n\r]*^Files:[\n\r]*^(\t//.*)\t# .*[\n\r]*`)
	if err != nil {
		return properties, fmt.Errorf("Regex (files) compile error: %v", err)
	}

	idx := pattern.FindIndex(out)  // find Files of 1st occurrence if any
	if idx != nil {

		pattern, err = regexp.Compile(`(?m)^\t(//.*)\t# (.*)[\n\r]*`)
		if err != nil {
			return properties, fmt.Errorf("Regex (files) compile error: %v", err)
		}

		list := pattern.FindAllSubmatch(out[idx[0]:], -1) // Get the other Files if any
		if len(list) > 0 {
			if len(list[0]) <= 0 {
				return properties, fmt.Errorf("Parsing workspace error - reading file details: %s", out)
			}
			properties.Files = make(map[string]string)
			for _, v := range list {
				properties.Files[string(v[1])] = string(v[2])
			}
		}
	}

	properties.Form = string(out)

	return properties, nil
}


// PutCLSpecProperties()
//	Write a CL specification properties using a p4 change -i command.
//	Get properties from a T_CLSpecProperties var
//	Returns a changelist number
//
func (p *Perforce) PutCLSpecProperties(properties T_CLSpecProperties) (CL int, err error) {
	p.logThis(fmt.Sprintf("PutCLSpecProperties()"))

  var cmd *exec.Cmd
	if len(p.user) > 0 {
		cmd = exec.Command(p.p4Cmd, "-u", p.user, "-c", p.workspace, "change", "-i")
		// out, err = exec.Command(p.p4Cmd, "-u", p.user, "-c", p.workspace, "change", "-o", sCL).CombinedOutput()
	} else {
		cmd = exec.Command(p.p4Cmd, "-u", "-c", p.workspace, "change", "-i")
	}
	if err != nil {
	}

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return 0, fmt.Errorf("P4 command line error (pipe) %v", err)
	}

	// Build and write the CL specification to stdin
	go func() {
		defer stdin.Close()
		eol := properties.Eol
		if properties.ChangeList == -1 {
			io.WriteString(stdin, eol + "Change:\t" + "new" + eol)
		} else {
			io.WriteString(stdin, eol + "Change:\t" + strconv.Itoa(properties.ChangeList) + "new" + eol)
		}
		if len(properties.Date) > 0 {
			io.WriteString(stdin, "Date:\t" + properties.Date + eol)
		}
		if len(properties.Client) > 0 {
			io.WriteString(stdin, "Client:\t" + properties.Client + eol)
		}
		if len(properties.User) > 0 {
			io.WriteString(stdin, "User:\t" + properties.User + eol)
		}
		if len(properties.Status) > 0 {
			io.WriteString(stdin, "Status:\t" + properties.Status + eol)
		}
		if len(properties.Type) > 0 {
			io.WriteString(stdin, "Type:\t" + properties.Type + eol)
		}
		descr := "\t" + properties.Description  // Prefix each line with a tab
		descr = strings.ReplaceAll(descr, "\n", "\n\t")
		descr = strings.TrimSuffix(descr, "\t")
		io.WriteString(stdin, "Description:" + eol)
		io.WriteString(stdin, descr + eol)
		if len(properties.Files) > 0 {
			io.WriteString(stdin, "Files:" + eol)
			for k, v := range properties.Files {
				io.WriteString(stdin, "\t" + k + " #" + v + eol)
			}
		}
	}()

	// Read response string
	out, err := cmd.CombinedOutput()
	if err != nil {
		return 0, fmt.Errorf("P4 command line error (CombinedOutput) %v - out=%s", err, out)
	}

	// Parse response
	// "Change 1234567 created with 23 open file(s)." is good anything else error
	pattern, err := regexp.Compile(`^Change ([0-9]*) created with ([0-9]*) open file\(s\)`)
	if err != nil {
		return 0, fmt.Errorf("Regex compile error: %v", err)
	}

	matches := pattern.FindSubmatch(out)
	if len(matches) < 3 {
		return 0, fmt.Errorf("Error unexpected response. Received %s", out)
	}

	cl, err := strconv.Atoi(string(matches[1]))
	if err != nil {
		return 0, fmt.Errorf("Error changelist format: %v, received %s", err, out)
	}
	p.logThis(fmt.Sprintf("Changelist#: %d", cl))
	nbfiles, err :=  strconv.Atoi(string(matches[2]))
	if err != nil { // just a warning since a apparenlty valid cl has been received
		p.logThis(fmt.Sprintf("Warning - nb files format incorrect: %v, received %s", err, out))
	} else {
		if nbfiles != len(properties.Files) { // sanity check
			p.logThis(fmt.Sprintf("Warning - nb files in response doesn't match changelist: %d vs %d", nbfiles, len(properties.Files)))
		}
	}

	return cl, nil
}
