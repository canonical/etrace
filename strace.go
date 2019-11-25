// -*- Mode: Go; indent-tabs-mode: t -*-

/*
 * Copyright (C) 2018 Canonical Ltd
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

package main

import (
	"bufio"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"os/user"
	"regexp"
	"sort"
	"strconv"
	"time"
)

// ExeRuntime is the runtime of an individual executable
type ExeRuntime struct {
	Start    time.Time
	Exe      string
	TotalSec time.Duration
}

// ExecveTiming measures the execve calls timings under strace. This is
// useful for performance analysis. It keeps the N slowest samples.
type ExecveTiming struct {
	TotalTime   time.Duration
	ExeRuntimes []ExeRuntime
	indent      string

	pidChildren *pidChildTracker

	nSlowestSamples int
}

func unixFloatSecondsToTime(t float64) time.Time {
	if t > math.MaxInt64 || t < math.MinInt64 {
		panic(fmt.Sprintf("time %f is outside of int64 range", t))
	}
	startUnixSeconds := math.Floor(t)
	startUnixNanoseconds := (t - startUnixSeconds) * float64(time.Second/time.Nanosecond)
	return time.Unix(int64(startUnixSeconds), int64(startUnixNanoseconds))
}

// NewExecveTiming returns a new ExecveTiming struct that keeps
// the given amount of the slowest exec samples.
// if nSlowestSamples is equal to 0, all exec samples are kept
func NewExecveTiming(nSlowestSamples int) *ExecveTiming {
	return &ExecveTiming{nSlowestSamples: nSlowestSamples}
}

func (stt *ExecveTiming) addExeRuntime(start float64, exe string, totalSec float64) {
	stt.ExeRuntimes = append(stt.ExeRuntimes, ExeRuntime{
		Start:    unixFloatSecondsToTime(start),
		Exe:      exe,
		TotalSec: time.Duration(totalSec * float64(time.Second)),
	})
	if stt.nSlowestSamples > 0 {
		stt.prune()
	}
}

// prune() ensures the number of ExeRuntimes stays with the nSlowestSamples
// limit
func (stt *ExecveTiming) prune() {
	for len(stt.ExeRuntimes) > stt.nSlowestSamples {
		fastest := 0
		for idx, rt := range stt.ExeRuntimes {
			if rt.TotalSec < stt.ExeRuntimes[fastest].TotalSec {
				fastest = idx
			}
		}
		// delete fastest element
		stt.ExeRuntimes = append(stt.ExeRuntimes[:fastest], stt.ExeRuntimes[fastest+1:]...)
	}
}

// Display shows the final exec timing output
func (stt *ExecveTiming) Display(w io.Writer) {
	if len(stt.ExeRuntimes) == 0 {
		return
	}

	fmt.Fprintf(w, "%d exec calls during snap run:\n", len(stt.ExeRuntimes))
	fmt.Fprintf(w, "\tStart\tStop\tElapsed\tExec\n")

	sort.Slice(stt.ExeRuntimes, func(i, j int) bool {
		return stt.ExeRuntimes[i].Start.Before(stt.ExeRuntimes[j].Start)
	})

	// TODO: this shows processes linearly, when really I think we want a
	// tree/forest style output showing forked processes indented underneath the
	// parent, with exec'd processes lined up with their previous executable
	// but note that doing so in the most generic case isn't neat since you can
	// have processes that are forked much later than others and will be aligned
	// with previous executables much earlier in the output
	for _, rt := range stt.ExeRuntimes {
		relativeStart := rt.Start.Sub(stt.ExeRuntimes[0].Start)
		fmt.Fprintf(w,
			"\t%d\t%d\t%v\t%s\n",
			int64(relativeStart/time.Microsecond),
			int64((relativeStart+rt.TotalSec)/time.Microsecond),
			rt.TotalSec,
			rt.Exe,
		)
	}

	fmt.Fprintln(w, "Total time: ", stt.TotalTime)
}

type childPidStart struct {
	start float64
	pid   string
}

type pidChildTracker struct {
	pidToChildrenPIDs map[string][]childPidStart
}

func newPidChildTracker() *pidChildTracker {
	return &pidChildTracker{
		pidToChildrenPIDs: make(map[string][]childPidStart),
	}
}

func (pct *pidChildTracker) Add(pid string, child string, start float64) {
	if _, ok := pct.pidToChildrenPIDs[pid]; !ok {
		pct.pidToChildrenPIDs[pid] = []childPidStart{}
	}
	pct.pidToChildrenPIDs[pid] = append(pct.pidToChildrenPIDs[pid], childPidStart{start: start, pid: child})
}

type exeStart struct {
	start float64
	exe   string
}

type pidTracker struct {
	pidToExeStart map[string]exeStart
}

func newPidTracker() *pidTracker {
	return &pidTracker{
		pidToExeStart: make(map[string]exeStart),
	}
}

func (pt *pidTracker) Get(pid string) (startTime float64, exe string) {
	if exeStart, ok := pt.pidToExeStart[pid]; ok {
		return exeStart.start, exeStart.exe
	}
	return 0, ""
}

func (pt *pidTracker) Add(pid string, startTime float64, exe string) {
	pt.pidToExeStart[pid] = exeStart{start: startTime, exe: exe}
}

func (pt *pidTracker) Del(pid string) {
	delete(pt.pidToExeStart, pid)
}

// TODO: can execve calls be "interrupted" like clone() below?
// lines look like:
// PID   TIME              SYSCALL
// 17363 1542815326.700248 execve("/snap/brave/44/usr/bin/update-mime-database", ["update-mime-database", "/home/egon/snap/brave/44/.local/"...], 0x1566008 /* 69 vars */) = 0
var execveRE = regexp.MustCompile(`([0-9]+)\ +([0-9.]+) execve\(\"([^"]+)\"`)

// lines look like:
// PID   TIME              SYSCALL
// 14157 1542875582.816782 execveat(3, "", ["snap-update-ns", "--from-snap-confine", "test-snapd-tools"], 0x7ffce7dd6160 /* 0 vars */, AT_EMPTY_PATH) = 0
var execveatRE = regexp.MustCompile(`([0-9]+)\ +([0-9.]+) execveat\(.*\["([^"]+)"`)

// lines look like (both SIGTERM and SIGCHLD need to be handled):
// PID   TIME                  SIGNAL
// 17559 1542815330.242750 --- SIGCHLD {si_signo=SIGCHLD, si_code=CLD_EXITED, si_pid=17643, si_uid=1000, si_status=0, si_utime=0, si_stime=0} ---
var sigChldTermRE = regexp.MustCompile(`[0-9]+\ +([0-9.]+).*SIG(CHLD|TERM)\ {.*si_pid=([0-9]+),`)

// lines look like
// PID   TIME                            SIGNAL
// 20882 1573257274.988650 +++ killed by SIGKILL +++
var sigkillRE = regexp.MustCompile(`([0-9]+)\ +([0-9.]+) \+\+\+ killed by SIGKILL \+\+\+`)

func handleExecMatch(trace *ExecveTiming, pt *pidTracker, match []string) error {
	if len(match) == 0 {
		return nil
	}
	// the pid of the process that does the execve{,at}()
	pid := match[1]
	execStart, err := strconv.ParseFloat(match[2], 64)
	if err != nil {
		return err
	}
	exe := match[3]

	// deal with subsequent execve()
	if start, exe := pt.Get(pid); exe != "" {
		trace.addExeRuntime(start, exe, execStart-start)
	}
	pt.Add(pid, execStart, exe)
	return nil
}

func handleSignalMatch(trace *ExecveTiming, pt *pidTracker, match []string) error {
	if len(match) == 0 {
		return nil
	}
	sigTime, err := strconv.ParseFloat(match[1], 64)
	if err != nil {
		return err
	}
	sigPid := match[3]

	if start, exe := pt.Get(sigPid); exe != "" {
		trace.addExeRuntime(start, exe, sigTime-start)
		pt.Del(sigPid)
	}
	return nil
}

func handleSigkillMatch(trace *ExecveTiming, pt *pidTracker, match []string) error {
	if len(match) == 0 {
		return nil
	}
	pid := match[1]
	sigTime, err := strconv.ParseFloat(match[2], 64)
	if err != nil {
		return err
	}

	if start, exe := pt.Get(pid); exe != "" {
		trace.addExeRuntime(start, exe, sigTime-start)
		pt.Del(pid)
	}
	return nil
}

func handleCloneMatch(trace *ExecveTiming, pct *pidChildTracker, match []string) error {
	if len(match) == 0 {
		return nil
	}
	// the pid of the parent process clone()ing a new child
	ppid := match[1]

	// the time the child was created
	execStart, err := strconv.ParseFloat(match[2], 64)
	if err != nil {
		return err
	}

	// the pid of the new child
	pid := match[3]
	pct.Add(ppid, pid, execStart)
	return nil
}

// TraceExecveTimings will read an strace log and produce a timing report of the
// n slowest exec's
func TraceExecveTimings(straceLog string, nSlowest int) (*ExecveTiming, error) {
	slog, err := os.Open(straceLog)
	if err != nil {
		return nil, err
	}
	defer slog.Close()

	// pidTracker maps the "pid" string to the executable
	pidTracker := newPidTracker()

	// pidChildTracker := newPidChildTracker()

	var line string
	var start, end float64
	var startPID, endPID int
	trace := NewExecveTiming(nSlowest)
	r := bufio.NewScanner(slog)
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
		if err := handleExecMatch(trace, pidTracker, match); err != nil {
			return nil, err
		}
		match = execveatRE.FindStringSubmatch(line)
		if err := handleExecMatch(trace, pidTracker, match); err != nil {
			return nil, err
		}
		// handleSignalMatch looks for SIG{CHLD,TERM} signals and
		// maps them via the pidTracker to the execve{,at}() calls
		// of the terminating PID to calculate the total time of
		// an execve{,at}() call.
		match = sigChldTermRE.FindStringSubmatch(line)
		if err := handleSignalMatch(trace, pidTracker, match); err != nil {
			return nil, err
		}

		// handleSignalMatch looks for SIGKILL signals for processes and uses
		// the time that SIGKILL happens to calculate the total time of an
		// execve{,at}() call.
		match = sigkillRE.FindStringSubmatch(line)
		if err := handleSigkillMatch(trace, pidTracker, match); err != nil {
			return nil, err
		}
	}
	if _, err := fmt.Sscanf(line, "%v %f", &endPID, &end); err != nil {
		return nil, fmt.Errorf("cannot parse end of exec profile: %s", err)
	}

	// handle processes which don't execute anything
	if startPID == endPID {
		pidString := strconv.Itoa(startPID)
		if start, exe := pidTracker.Get(pidString); exe != "" {
			trace.addExeRuntime(start, exe, end-start)
			pidTracker.Del(pidString)
		}
	}
	trace.TotalTime = unixFloatSecondsToTime(end).Sub(unixFloatSecondsToTime(start))
	// trace.pidChildren = pidChildTracker

	if r.Err() != nil {
		return nil, r.Err()
	}

	return trace, nil
}

// These syscalls are excluded because they make strace hang on all or
// some architectures (gettimeofday on arm64).
var excludedSyscalls = "!select,pselect6,_newselect,clock_gettime,sigaltstack,gettid,gettimeofday,nanosleep"

// Command returns how to run strace in the users context with the
// right set of excluded system calls.
func straceCommand(extraStraceOpts []string, traceeCmd ...string) (*exec.Cmd, error) {
	current, err := user.Current()
	if err != nil {
		return nil, err
	}
	sudoPath, err := exec.LookPath("sudo")
	if err != nil {
		return nil, fmt.Errorf("cannot use strace without sudo: %s", err)
	}

	// Try strace from the snap first, we use new syscalls like
	// "_newselect" that are known to not work with the strace of e.g.
	// ubuntu 14.04.
	//
	// TODO: some architectures do not have some syscalls (e.g.
	// s390x does not have _newselect). In
	// https://github.com/strace/strace/issues/57 options are
	// discussed.  We could use "-e trace=?syscall" but that is
	// only available since strace 4.17 which is not even in
	// ubutnu 17.10.
	stracePath, err := exec.LookPath("strace")
	if err != nil {
		return nil, fmt.Errorf("cannot find an installed strace, please try 'snap install strace-static'")
	}

	args := []string{
		sudoPath,
		"-E",
		stracePath,
		"-u", current.Username,
		"-f",
		"-e", excludedSyscalls,
	}
	args = append(args, extraStraceOpts...)
	args = append(args, traceeCmd...)

	return &exec.Cmd{
		Path: sudoPath,
		Args: args,
	}, nil
}

// TraceExecCommand returns an exec.Cmd suitable for tracking timings of
// execve{,at}() calls
func TraceExecCommand(straceLogPath string, origCmd ...string) (*exec.Cmd, error) {
	extraStraceOpts := []string{"-ttt", "-e", "trace=execve,execveat", "-o", fmt.Sprintf("%s", straceLogPath)}

	return straceCommand(extraStraceOpts, origCmd...)
}
