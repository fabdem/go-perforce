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


// Implementation of diff (based on "p4 diff")
// Get the workspace files
// Counts its number of lines and report if encoding is utf16 and line endings are cr/lf
// If it's the case the number of added and removed lines will have to be divided by 2.
//	Input params:
//		- depot file path and name
//	Output params:
//    - structure with results
//		- flag indicating that the file is utf16 encoded and line endings are cr/lf
//		- err
//
type T_p4DiffRes struct {
  utf16crlf       bool
  addedLines      int
  removedLines    int
  changedLines    int
  depotFileLineCt int
}
func (p *Perforce) P4Diff(depotFile string)(res T_p4DiffRes, err error){
  p.log(fmt.Sprintf("P4Diff(%s)", depotFile))

	// Get workspace file
	wsFile,err := p.GetP4Where(depotFile)
	if err != nil {
		return res, err
	}

  // Get its number of lines
  f, err := os.Open(wsFile)
  if err != nil {
		return res, err
	}
  defer f.Close()

  p.log(fmt.Sprintf("Get workspace file line count (%s)",wsFile))
  totalLineCountWS, res.utf16crlf, err := lineCounter(f)
  if err != nil {
		return res, err
  }

  // Diff workspace file from head revision
  _, _, res.addedLines, res.removedLines, res.changedLines, err := p.DiffHeadnWorkspace(depotFile)
  if err != nil {
		return res, err
	}

  if res.utf16crlf { // Divide by 2 added and removed # of lines if encoding utf16 and line ending cr/lf
    addedLines << 1
    removedLines << 1
  }

  // Calculate total number of lines of the depot files because this is the one
  // we want to base the percentages on
  res.depotFileLineCt = totalLineCountWS - addedLines + removedLines

  return res, nil
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
			if (bytes.Count(buf[:c], utf16LESep) >=1) || (bytes.Count(buf[:c], utf16BESep) >=1) {
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
