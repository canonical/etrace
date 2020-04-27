/*
 * Copyright (C) 2019 Canonical Ltd
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU General Public License version 3 as
 * published by the Free Software Foundation.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU General Public License for more details.
 *
 * You should have received a copy of the GNU General Public License
 * along with this program.  If not, see <http://www.gnu.org/licenses/>.
 *
 */

package strace

import (
	"bufio"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"time"

	"github.com/anonymouse64/etrace/internal/files"
)

// matches syscalls that have fd as the first arg and a path as the second arg
// note that since it's really hard to match whether the fd + path match the
// desired path, this just matches the fd + path and the code will join them to
// see if the full path matches the desired path or not
// TODO: should we also handle the returned fd too? probably don't need to
// since just because a program gets a fd returned to it doesn't mean it does
// anything to it, so we should catch the returned fd with another syscall if
// the program actually uses it, right?
// TODO: could we reduce false matches here by only match syscalls with "at" at
// the end ???
// lines look like:
// 122166 1574886795.484115 newfstatat(3</proc/122166/fd>, "9", {st_mode=S_IFREG|0644, st_size=1377694, ...}, 0) = 0
// 121041 1574886786.247289 openat(9</snap/chromium/958>, "data-dir", O_RDONLY|O_NOFOLLOW|O_CLOEXEC|O_DIRECTORY) = 10</snap/chromium/958/data-dir>
// 121041 1574886786.247289 openat(9</snap/chromium/958>, "data-dir/some-sub-dir", O_RDONLY|O_NOFOLLOW|O_CLOEXEC|O_DIRECTORY) = 10</snap/chromium/958/data-dir>
var fdAndPathRE = regexp.MustCompile(
	`([0-9]+) ([0-9]+\.[0-9]+) (.*)\([0-9]+<(.*?)>, "([^\/]?.+)".*= [0-9]+(?:\s*$|x[0-9a-f]+$|<.*>$|$)`,
)

// matches syscalls that have AT_FDCWD with an absolute path as the 2nd argument
// lines look like:
// 121188 1574886788.027891 openat(AT_FDCWD, "/snap/chromium/current/usr/lib/locale/en_US.UTF-8/LC_COLLATE", O_RDONLY|O_CLOEXEC) = 4</some/where>
// 121188 1574886788.027966 openat(AT_FDCWD, "/snap/chromium/958/usr/lib/locale/en_US.utf8/LC_COLLATE", O_RDONLY|O_CLOEXEC) = 3</snap/chromium/958/usr/lib/locale/aa_DJ.utf8/LC_COLLATE>
// 120994 1574886785.937456 readlinkat(AT_FDCWD, "/snap/chromium/current", ""..., 128) = 3
var absPathWithCWDRE = regexp.MustCompile(
	`([0-9]+) ([0-9]+\.[0-9]+) ([a-zA-Z0-9_]+)\(AT_FDCWD,\s+\"(.*?)\".*=\s+[0-9]+(?:\s*$|x[0-9a-f]+$|<.*>$|$)`,
)

// matches syscalls that have just a single path as any of the arguments, except
// those with AT_FDCWD as those cases are handled with absPathWithCWDString
// above
// TODO: support syscalls like symlinkat to catch multiple file paths, since
// we currently will only catch the last one
// unfortunately we don't have negative lookarounds in go regex, so instead we
// have an optional group with just AT_FDCWD in it to check if that group is
// present we can tell if this really matched or not
// TODO: investigate combining this pattern with the absPathWithCWDRE one
// lines look like:
// 121372 1574886788.833540 symlinkat("/snap/chromium/958/usr/lib/x86_64-linux-gnu/gtk-3.0/3.0.0/immodules/im-am-et.so", AT_FDCWD, "/home/ijohnson/snap/chromium/common/.cache/immodules/im-am-et.so") = 0
// 121185 1574886787.979943 execve("/snap/chromium/958/usr/sbin/update-icon-caches", [...], 0x561bce4ee880 /* 105 vars */) = 0
// 120990 1574886792.229066 readlink("/snap/chromium/958/etc/fonts/conf.d/65-nonlatin.conf", ""..., 4095) = 30
// 121041 1574886786.249939 mount("tmpfs", "/snap/chromium/958/data-dir/icons", ""..., 0, ""...) = 0
var absPathRE = regexp.MustCompile(
	`^([0-9]+) ([0-9]+\.[0-9]+) ([a-zA-Z0-9_]+)\(.*?\"(.+?)\".*?\) =\s+[0-9]+(?:\s*$|x[0-9a-f]+$|<.*>$)`,
)

// matches syscalls that just have a single fd as any of the arguments,
// INCLUDING those with an additional path argument immediately following the fd
// which is also matched by fdAndPathRE, due to this care has to be taken not to
// double count these accesses
// lines look like:
// 121041 1574886786.247289 openat(9</snap/chromium/958>, "data-dir", O_RDONLY|O_NOFOLLOW|O_CLOEXEC|O_DIRECTORY) = 10</snap/chromium/958/data-dir>
// 121188 1574886788.028095 close(3</snap/chromium/958/usr/lib/locale/aa_DJ.utf8/LC_COLLATE>) = 0
// 121188 1574886788.028052 mmap(NULL, 1244054, PROT_READ, MAP_PRIVATE, 3</snap/chromium/958/usr/lib/locale/aa_DJ.utf8/LC_COLLATE>, 0) = 0x7f8d780a7000
// 120990 1574886796.125850 lseek(156</snap/chromium/958/data-dir/icons/Yaru/cursors/text>, 6144, SEEK_SET) = 6144
// 120990 1574886796.126170 read(156</snap/chromium/958/data-dir/icons/Yaru/cursors/text>, ""..., 1024) = 1024
// DOES NOT match these lines:
// 27652 1587946984.879501 write(9<pipe:[200089]>, ""..., 4) = 4
var fdRE = regexp.MustCompile(
	`([0-9]+)\s+([0-9]+\.[0-9]+)\s+(.*)\(.*[0-9]+<(/.*?)>.*= [0-9]+(?:\s*$|x[0-9a-f]+$|<.*>$|$)`,
)

// PathAccess represents a single syscall accessing a file
type PathAccess struct {
	Time    time.Time
	Path    string
	Syscall string
	pid     string
}

// ProcessRuntime represents a single program and the file accesses over the
// course of it's lifetime
type ProcessRuntime struct {
	Start        time.Time
	Exe          string
	RunDuration  time.Duration
	PathAccesses []PathAccess
	pid          string
}

// FileAndSize contains the path of a file and the size of it
type FileAndSize struct {
	// Path is where the file was measured as
	Path string
	// Size may be -1 if we cannot get the size of the file with os.Stat()
	Size int64
}

// ExecvePaths represents the set of processes and files accessed by those
// processes for a given program execution
type ExecvePaths struct {
	AllFiles  []FileAndSize
	Processes []ProcessRuntime
	TotalTime time.Duration

	*pidTracker

	persistentPidTracker *pidTracker
	allFilesSet          map[string]bool
	pathProcesses        []PathAccess
}

type execvePathsTracer interface {
	execveTimingTracer
	addPathToList(path string)
	addProcessPathAccess(path PathAccess)
}

// NewExecveFiles returns a ExecveFiles suitable for
func newExecveFiles() *ExecvePaths {
	// TODO: merge this with execveTiming in an interface so we can share
	// parsing loop between the implementations
	e := &ExecvePaths{
		allFilesSet: make(map[string]bool),
		pidTracker:  newpidTracker(),
	}
	return e
}

func (e *ExecvePaths) addExeRuntime(start float64, exe string, totalSec float64, pid string) {
	e.Processes = append(e.Processes, ProcessRuntime{
		Start:       unixFloatSecondsToTime(start),
		Exe:         exe,
		RunDuration: time.Duration(totalSec * float64(time.Second)),
		pid:         pid,
	})
}

func (e *ExecvePaths) addPathToList(path string) {
	e.allFilesSet[path] = true
}

func (e *ExecvePaths) addProcessPathAccess(path PathAccess) {
	// add the path to the full list
	e.addPathToList(path.Path)

	// save the path access for later, when we have all the processes finished
	// and we can correlate path accesses to particular processes
	e.pathProcesses = append(e.pathProcesses, path)
}

// Display shows the final exec timing output
func (e *ExecvePaths) Display(w io.Writer, opts *DisplayOptions) {
	if len(e.AllFiles) == 0 {
		return
	}

	fmt.Fprintf(w, "%d files accessed during snap run:\n", len(e.AllFiles))
	fmt.Fprintf(w, "\tFilename\tSize (bytes)\n")
	for _, f := range e.AllFiles {
		fmt.Fprintf(w, "\t%s\t%d\n", f.Path, f.Size)
	}

	fmt.Fprintln(w)
}

func handlePathMatchElem4(trace execvePathsTracer, match []string, fileRegex *regexp.Regexp) error {
	if len(match) == 0 {
		return nil
	}

	pid, execStart, syscall, err := parsePIDAndReturnOthers(match)
	if err != nil {
		return err
	}

	// add this path to the tracer's total list of paths if it matches the
	// regex
	if fileRegex.FindString(match[4]) != "" {
		trace.addProcessPathAccess(
			PathAccess{
				Time:    unixFloatSecondsToTime(execStart),
				Path:    match[4],
				Syscall: syscall,
				pid:     pid,
			},
		)
	}

	return nil
}

func handleFdAndPathMatch(trace execvePathsTracer, match []string, fileRegex *regexp.Regexp) error {
	if len(match) == 0 {
		return nil
	}

	pid, execStart, syscall, err := parsePIDAndReturnOthers(match)
	if err != nil {
		return err
	}

	// for this, we can get any path, not just the one we want, so we need to
	// join the fd + path and see if it matches
	fullPath := filepath.Join(match[4], match[5])
	if fileRegex.FindString(fullPath) != "" {
		trace.addProcessPathAccess(
			PathAccess{
				Time:    unixFloatSecondsToTime(execStart),
				Path:    fullPath,
				Syscall: syscall,
				pid:     pid,
			},
		)
	}

	return nil
}

func handleAbsPathMatch(trace execvePathsTracer, match []string, fileRegex *regexp.Regexp) error {
	if len(match) == 0 {
		return nil
	}

	// if we caught the AT_FDCWD, then return, we are handling that one in the
	// other pattern
	if match[4] != "" {
		return nil
	}

	pid, execStart, syscall, err := parsePIDAndReturnOthers(match)
	if err != nil {
		return err
	}

	if fileRegex.FindString(match[4]) != "" {
		// add this path to the tracer's total list of paths
		trace.addProcessPathAccess(
			PathAccess{
				Time:    unixFloatSecondsToTime(execStart),
				Path:    match[5],
				Syscall: syscall,
				pid:     pid,
			},
		)
	}

	return nil
}

// TraceExecveWithFiles will merge strace logs matching the given pattern and
// produce a file report with all the files matching the specified pattern read
// by every process in the execution
// TODO: we could speed this up if we injected the provided regex into the
// regular expressions we use to match all the strace lines, but that requires
// some really tough regular expression work and may have odd user behavior for
// "simple" cases like `.*`, which probably the user wants to use as `.*?`,
// otherwise they would get filepaths like `/some/file/thing/", "` because the
// filepath really has to stop at the last `"` character
func TraceExecveWithFiles(straceLogPattern string, regex *regexp.Regexp) (*ExecvePaths, error) {
	// first ensure the log file is empty and exists and open it
	mergedFile, err := files.EnsureExistsAndOpen(straceLogPattern, true)
	if err != nil {
		return nil, err
	}
	defer mergedFile.Close()

	// merge the log files
	cmd := exec.Command("strace-log-merge", straceLogPattern)

	// redirect stdout for strace-log-merge to the merged log file
	cmd.Stdout = mergedFile
	cmd.Stderr = os.Stderr

	err = cmd.Run()
	if err != nil {
		// if we failed to run strace-log-merge, check the file we redirected
		// stdout to, since otherwise we don't know how it failed
		mergedFile.Close()
		out, err2 := ioutil.ReadFile(straceLogPattern)
		if err2 != nil {
			log.Println(err2)
		}
		log.Println(string(out))
		return nil, err
	}

	// now we need to go back to the beginning of the file we opened to start
	// parsing it
	_, err = mergedFile.Seek(0, 0)
	if err != nil {
		return nil, err
	}

	// start scanning the file
	var line string
	var start, end float64
	var startPID, endPID int
	trace := newExecveFiles()
	r := bufio.NewScanner(mergedFile)
	for r.Scan() {
		line = r.Text()
		if start == 0.0 {
			if _, err := fmt.Sscanf(line, "%d %f ", &startPID, &start); err != nil {
				return nil, fmt.Errorf("cannot parse start of exec profile: %s", err)
			}
		}
		// handleExecMatch looks for execve{,at}() calls and
		// uses the pidTracker to keep track of execution of
		// things. Because of fork() we may see many pids and
		// within each pid we can see multiple execve{,at}()
		// calls.
		// An example of pids/exec transitions:
		// $ snap run --trace-exec test-snapd-sh -c "/bin/true"
		//    pid 20817 execve("snap-confine")
		//    pid 20817 execve("snap-exec")
		//    pid 20817 execve("/snap/test-snapd-sh/x2/bin/sh")
		//    pid 20817 execve("/bin/sh")
		//    pid 2023  execve("/bin/true")
		match := execveRE.FindStringSubmatch(line)
		if err := handleExecMatch(trace, match); err != nil {
			return nil, err
		}
		match = execveatRE.FindStringSubmatch(line)
		if err := handleExecMatch(trace, match); err != nil {
			return nil, err
		}
		// handleSignalMatch looks for SIG{CHLD,TERM} signals and
		// maps them via the pidTracker to the execve{,at}() calls
		// of the terminating PID to calculate the total time of
		// an execve{,at}() call.
		match = sigChldTermRE.FindStringSubmatch(line)
		if err := handleSignalMatch(trace, match); err != nil {
			return nil, err
		}

		// handleSignalMatch looks for SIGKILL signals for processes and uses
		// the time that SIGKILL happens to calculate the total time of an
		// execve{,at}() call.
		match = sigkillRE.FindStringSubmatch(line)
		if err := handleSigkillMatch(trace, match); err != nil {
			return nil, err
		}

		// now handle any file accesses
		match = absPathWithCWDRE.FindStringSubmatch(line)
		if err := handlePathMatchElem4(trace, match, regex); err != nil {
			return nil, err
		}

		match = absPathRE.FindStringSubmatch(line)
		if err := handleAbsPathMatch(trace, match, regex); err != nil {
			return nil, err
		}

		// for the last 2, the fdAndPath matches will also match fdRE, so
		// if that match is successful then we just skip the last check and
		// continue to the next line
		match = fdAndPathRE.FindStringSubmatch(line)
		if err := handleFdAndPathMatch(trace, match, regex); err != nil {
			return nil, err
		}

		if len(match) != 0 {
			continue
		}

		match = fdRE.FindStringSubmatch(line)
		if err := handlePathMatchElem4(trace, match, regex); err != nil {
			return nil, err
		}
	}

	// check scanning error
	if r.Err() != nil {
		return nil, r.Err()
	}

	// scan the last line to see if it matches the end line to compare with the
	// start
	if _, err := fmt.Sscanf(line, "%v %f", &endPID, &end); err != nil {
		return nil, fmt.Errorf("cannot parse end of exec profile: %s", err)
	}

	// handle processes which don't execve{,at} at all
	if startPID == endPID {
		pidString := strconv.Itoa(startPID)
		if start, exe := trace.getPid(pidString); exe != "" {
			trace.addExeRuntime(start, exe, end-start, pidString)
			trace.deletePid(pidString)
		}
	}
	trace.TotalTime = unixFloatSecondsToTime(end).Sub(unixFloatSecondsToTime(start))

	// put all the path accesses from the trace into their respective processes
	for _, path := range trace.pathProcesses {
		// to add a PathAccess to the process that triggered it, we need to find
		// what process triggered this by pid and time
		// we look first for all matching pids, then filter by pids who's
		// duration contains the time that the path access happened

		for i, proc := range trace.Processes {
			if proc.pid == path.pid {
				start := proc.Start
				end := proc.Start.Add(proc.RunDuration)
				if path.Time.After(start) && path.Time.Before(end) {
					// add this path access
					trace.Processes[i].PathAccesses = append(trace.Processes[i].PathAccesses, path)
					break
				}
			}
		}
	}

	// free up the path process access memory
	trace.pathProcesses = nil

	// put all the files from the map set into the list
	trace.AllFiles = make([]FileAndSize, 0, len(trace.allFilesSet))
	for path := range trace.allFilesSet {
		// get the size of the file to include in the output
		size := int64(-1)
		info, err := os.Stat(path)
		if err == nil {
			size = info.Size()
		}
		trace.AllFiles = append(trace.AllFiles, FileAndSize{
			Path: path,
			Size: size,
		})
	}

	// sort the all files by the path member for nicer formatting
	sort.Slice(trace.AllFiles, func(i, j int) bool {
		return trace.AllFiles[i].Path < trace.AllFiles[j].Path
	})

	// free up path file set memory
	trace.allFilesSet = nil

	return trace, nil
}
