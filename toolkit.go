package perforce

// Publicly available high level functions

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	// "path/filepath"
	// "regexp"
	"strconv"
	"strings"
)

type T_DiffRes struct {
	utf16crlf    bool
	fileHR       string
	nbLinesHR    int
	encodingHR   string
	fileWS       string
	nbLinesWS    int
	addedLines   int
	removedLines int
	changedLines int
}

// DiffHRvsWS()
// Implementation of a diff between head revision vs workspace.
// Two algos - 1 based on p4 diff and one custom (faster?)
//   - Get the workspace files
//   - Counts number of lines and report if encoding is utf16 and line endings are cr/lf
// If it's the case the number of added and removed lines will have to be divided by 2.
//	Input params:
//		- algo "p4" or "custom"
//		- depot file path and name
//	Output params:
//   	- structure with results
//		- flag indicating that the file is utf16 encoded and line endings are cr/lf
//		- err
//
func (p *Perforce) DiffHRvsWS(algo string, depotFile string) (res T_DiffRes, err error) {
	p.log(fmt.Sprintf("P4Diff(%s)", depotFile))

	res.fileHR = depotFile

	// Get workspace file
	res.fileWS, err = p.GetP4Where(depotFile)
	if err != nil {
		return res, err
	}

	// Get its number of lines
	f, err := os.Open(res.fileWS)
	if err != nil {
		return res, err
	}
	defer f.Close()

	p.log(fmt.Sprintf("Get workspace file line count (%s)", res.fileWS))
	res.nbLinesWS, res.utf16crlf, err = lineCounter(f) // !!!!!!!maybe move it after diff is done or in switch
	if err != nil {
		return res, err
	}
	switch algo {
	case "p4":
		// Diff workspace file from head revision
		r, err := p.p4DiffHRvsWS(depotFile, f)
		if err != nil {
			return res, err
		}

		if res.utf16crlf { // Divide by 2 added and removed # of lines if encoding utf16 and line ending cr/lf
			res.addedLines = (r.addedLines << 1)
			res.removedLines = (r.removedLines << 1)
		}

		// Calculate total number of lines of the depot files because this is the one
		// we want to base the percentages on
		res.nbLinesHR = res.nbLinesWS - res.addedLines + res.removedLines
	case "custom":
		r, err := p.customDiffHRvsWS(depotFile, f)
		// ???? update res with returned values
		res.addedLines = r.addedLines
	default:
		return res, errors.New(fmt.Sprintf("DiffHRvsWS() - Invalid algorithm: %s", algo))
	}

	return res, nil
}

// p4DiffHRvsWS()
// 	Diff workspace (WS) and head rev (HR) version of a file.
//  Uses perforce p4 diff with option summary and ignore line endings.
//	p4 diff returns a number of added, modified and deleted lines.
// 	Do a: p4 -uxxxxx -wyyyyy diff //workspacefile
//	A workspace name needs to be defined
//  If p.diffignorespace is set changes in spaces will be ignored.
// 	Input:
//		- Name of file in depot to diff - p4 will automatically determine workspace path
//		- File in workspace
//  Return:
//		- added, deleted and modified number of lines
//		- err code, nil if okay
//
//  To be noted that utf16 encoded files are correctly processed.
//
//
/* p4 command and output:
p4 -ca_workspace -ua_user diff -ds //path_and_name_of_a_file_in_depot
==== path_and_name_of_a_file_in_depot - path_and_name_of_a_file_in_workspace ====
add 3 chunks 8 lines
deleted 2 chunks 7 lines
changed 1 chunks 3 / 3 lines
*/

func (p *Perforce) p4DiffHRvsWS(aFileInDepot string, aFileInWS *os.File, r T_DiffRes) (err error) {
	p.log(fmt.Sprintf("p4DiffHRvsWS(%s)\n", aFileInDepot))

	var out []byte
	option := "-dls" // Summary output and ignore line endings
	if p.diffignorespace {
		option += "b" // plus changes within spaces will be ignored
	}

	if len(p.workspace) <= 0 {
		return errors.New(fmt.Sprintf("P4 command line error - a workspace needs to be defined"))
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
		return errors.New(fmt.Sprintf("P4 command line error %v  out=%s", err, out))
	}

	// Parse result
	cue1 := "===="
	cue2 := "==== "
	cue3 := " ===="
	cue4 := " - "
	fields := strings.Split(string(out), cue1)
	if len(fields) < 1 {
		return errors.New(fmt.Sprintf("P4 command line - parsing error  out=%s", out))
	}
	line := fields[1]                     // 1st line is supposed to contain path of files in depot and workspace.
	line = strings.TrimPrefix(line, cue2) // Isolate paths
	line = strings.TrimSuffix(line, cue3)
	fields = strings.Split(line, cue4)
	if len(fields) < 2 {
		return errors.New(fmt.Sprintf("P4 command line - parsing error in %s\n out=%s", line, out))
	}
	fileHR := fields[0]
	fileWS := fields[1]

	fields = strings.Split(string(out), cue3) // Split to get section with line stats
	if len(fields) < 2 {
		return errors.New(fmt.Sprintf("1 - P4 command line - parsing error in out=%s\n", out))
	}
	// fmt.Printf("\n\n\nfields[1]\n%s\n\n",fields[1])

	lines := strings.Split(fields[1], "\n") // Get the section with line stats
	if len(lines) < 4 {
		return errors.New(fmt.Sprintf("2 - P4 command line - parsing error in %s\n out=%s\n", lines, out))
	}
	// fmt.Printf("\n\nlines[]\n%s\n%s\n%s\n",lines[1],lines[2],lines[3])

	/*
		add 3 chunks 8 lines
		deleted 2 chunks 7 lines
		changed 1 chunks 3 / 3 lines
	*/
	if (strings.Index(lines[1], "add") == -1) || (strings.Index(lines[2], "deleted") == -1) || (strings.Index(lines[3], "changed") == -1) {
		return errors.New(fmt.Sprintf("3 - P4 command line - parsing error in:\n%s\n%s\n%s\n out=%s\n", lines, out))
	}
	addLine := strings.Fields(lines[1])
	removeLine := strings.Fields(lines[2])
	changeLine := strings.Fields(lines[3])

	//fmt.Printf("addLine:%v\nremoveLine:%v\nchangeLine:%v\n",addLine,removeLine,changeLine)
	if (len(addLine) < 5) || (len(removeLine) < 5) || (len(changeLine) < 7) {
		return errors.New(fmt.Sprintf("4 - P4 command line - parsing error out=%s\n", out))
	}
	var err1, err2, err3 /*, err4 */ error
	addedLines, err1 := strconv.Atoi(addLine[3])
	removedLines, err2 := strconv.Atoi(removeLine[3])
	// changedLines1, err4 := strconv.Atoi(changeLine[3])
	changedLines, err3 := strconv.Atoi(changeLine[5])
	if (err1 != nil) || (err2 != nil) || (err3 != nil) /* || (err4 != nil) */ {
		return errors.New(fmt.Sprintf("5 - P4 command line - parsing error out=%s\n", out))
	}

	res.fileHR = fileHR
	res.fileWS = fileWS
	res.addedLines = addedLines
	res.removedLines = removedLines
	res.changedLines = changedLines

	return res, nil
}

// CustomDiffHRvsWS()
// 	Custom diff workspace (WS) and head rev (HR) version of a file.
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
//		- Name of file in depot to diff - p4 will automatically determine workspace path
//		- File in workspace
//  Return:
//		- Name of file head rev
//		- Number of line file head rev
//		- Name file in workspace
//		- Number of line file file in workspace
//		- Added, deleted and modified number of lines
//		- Err code, nil if okay

func (p *Perforce) customDiffHRvsWS(aFileInDepot string, aFileInWS *os.File, r *T_DiffRes) (err error) {
	p.log(fmt.Sprintf("customDiffHRvsWS(%s)\n", aFileInDepot))

	// Get head revision file
	tempHR, fileHR, err := p.GetFile(aFileInDepot, 0)
	p.log(fmt.Sprintf("	Head Rev=%s\n", fileHR))
	if err != nil {
		return errors.New(fmt.Sprintf("Error getting head rev: %s", fileHR))
	}
	//tempName := tempHR.Name()
	tempf, err := os.Open(tempHR)
	if err != nil {
		return errors.New(fmt.Sprintf("Error getting head rev: %s", tempHR))
	}
	defer tempf.Close()

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
		r.nbLinesHR++
	}
	p.log(fmt.Sprintf("	Head rev file - nb lines read %d)\n", r.nbLinesHR))
	if err := scanner.Err(); err != nil {
		return errors.New(fmt.Sprintf("Error parsing head rev file: %s", tempHR))
	}

	//	Read workspace and compare
	scanner = bufio.NewScanner(aFileInWS)
	for scanner.Scan() {
		line := scanner.Text()
		if p.diffignorespace {
			line = strings.Trim(line, " \t\r\n")
		}
		if nb, ok := m_lines[line]; ok { // if line found
			if nb <= 0 {
				r.addedLines++ // There are more occurrences of this line in new file
			} else {
				m_lines[line]--
			}
		} else { // if line not found
			r.addedLines++ // This line didn't exist in old file
		}
		nbLinesWS++
	}
	p.log(fmt.Sprintf("	Workspace file - nb lines read %d)\n", nbLinesWS))

	// Check what's left in the map
	for _, v := range m_lines {
		r.removedLines += v // Accrue here number of modified or deleted lines from headrev
	}

	// Delete temp head rev file
	tempf.Close()
	err = os.Remove(tempHR)
	if err != nil {
		p.log(fmt.Sprintf("Error deleting temp file %s %s)\n", tempHR, err))
	} // Non fatal error

	return nil
}

// Count the number of lines of a text file
// and returns utf16crlf true if utf16LE or utf16BE with cr/lf line ending
// in order to inform caller (p4 diff doesn't deal correctly with this encoding).
// Detection is very basic. We assume that the file encoding and line ending is consistent
func lineCounter(r io.Reader) (count int, utf16crlf bool, err error) {

	buf := make([]byte, 64*1024)
	lineSep := []byte{'\n'}
	utf16LESep = []byte{'\r', 0x00, '\n', 0x00}
	utf16BESep = []byte{0x00, '\r', 0x00, '\n'}

	for {
		c, err := r.Read(buf)

		if !utf16crlf {
			// If utf16 and line endings are cr/lf inform caller.
			if (bytes.Count(buf[:c], utf16LESep) >= 1) || (bytes.Count(buf[:c], utf16BESep) >= 1) {
				utf16crlf = true
			}
		}

		count += bytes.Count(buf[:c], lineSep)

		switch {
		case err == io.EOF:
			return count, nil

		case err != nil:
			return count, err
		}
	}
}
