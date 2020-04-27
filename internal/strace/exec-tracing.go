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
	"math"
	"os"
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
	pid      string
}

// ExecveTiming measures the execve calls timings under strace. This is
// useful for performance analysis. It keeps the N slowest samples.
type ExecveTiming struct {
	TotalTime   time.Duration
	ExeRuntimes []ExeRuntime
	indent      string

	// pidChildren *pidChildTracker

	nSlowestSamples int

	*pidTracker
}

type execveTimingTracer interface {
	addExeRuntime(start float64, exe string, totalSec float64, pid string)

	getPid(pid string) (startTime float64, exe string)
	addPid(pid string, startTime float64, exe string)
	deletePid(pid string)
}

func unixFloatSecondsToTime(t float64) time.Time {
	// check to make sure the time isn't outside of the bounds of an int64
	if t > math.MaxInt64 || t < math.MinInt64 {
		panic(fmt.Sprintf("time %f is outside of int64 range", t))
	}
	startUnixSeconds := math.Floor(t)
	startUnixNanoseconds := (t - startUnixSeconds) * float64(time.Second)
	return time.Unix(int64(startUnixSeconds), int64(startUnixNanoseconds))
}

// newExecveTiming returns a new ExecveTiming struct that keeps
// the given amount of the slowest exec samples.
// if nSlowestSamples is equal to 0, all exec samples are kept
func newExecveTiming(nSlowestSamples int) *ExecveTiming {
	e := &ExecveTiming{nSlowestSamples: nSlowestSamples}
	e.pidTracker = newpidTracker()
	return e
}

func (stt *ExecveTiming) addExeRuntime(start float64, exe string, totalSec float64, pid string) {
	stt.ExeRuntimes = append(stt.ExeRuntimes, ExeRuntime{
		Start:    unixFloatSecondsToTime(start),
		Exe:      exe,
		TotalSec: time.Duration(totalSec * float64(time.Second)),
		pid:      pid,
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
func (stt *ExecveTiming) Display(w io.Writer, opts *DisplayOptions) {
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

// TODO: can execve calls be "interrupted" like clone() below?
// lines look like:
// PID   TIME              SYSCALL
// 17363 1542815326.700248 execve("/snap/brave/44/usr/bin/update-mime-database", ["update-mime-database", "/home/egon/snap/brave/44/.local/"...], 0x1566008 /* 69 vars */) = 0
var execveRE = regexp.MustCompile(`([0-9]+)\ +([0-9.]+) execve\(\"([^"]+)\".*\) = 0`)

// lines look like:
// PID   TIME              SYSCALL
// 14157 1542875582.816782 execveat(3, "", ["snap-update-ns", "--from-snap-confine", "test-snapd-tools"], 0x7ffce7dd6160 /* 0 vars */, AT_EMPTY_PATH) = 0
var execveatRE = regexp.MustCompile(`([0-9]+)\ +([0-9.]+) execveat\(.*\["([^"]+)".*\) = 0`)

// lines look like (both SIGTERM and SIGCHLD need to be handled):
// PID   TIME                  SIGNAL
// 17559 1542815330.242750 --- SIGCHLD {si_signo=SIGCHLD, si_code=CLD_EXITED, si_pid=17643, si_uid=1000, si_status=0, si_utime=0, si_stime=0} ---
var sigChldTermRE = regexp.MustCompile(`[0-9]+\ +([0-9.]+).*SIG(CHLD|TERM)\ {.*si_pid=([0-9]+),`)

// lines look like
// PID   TIME                            SIGNAL
// 20882 1573257274.988650 +++ killed by SIGKILL +++
var sigkillRE = regexp.MustCompile(`([0-9]+)\ +([0-9.]+) \+\+\+ killed by SIGKILL \+\+\+`)

// this is a silly function but de-duplicates the code
func parsePIDAndReturnOthers(match []string) (string, float64, string, error) {
	execStart, err := strconv.ParseFloat(match[2], 64)
	if err != nil {
		return "", 0, "", err
	}
	// for all matches, match[1] is the pid and match[2] is the time
	// for execve matches, match[3] is the exe
	// for file matches, match[3] is the syscall
	return match[1], execStart, match[3], nil
}

func handleExecMatch(trace execveTimingTracer, match []string) error {
	if len(match) == 0 {
		return nil
	}

	pid, execStart, exe, err := parsePIDAndReturnOthers(match)
	if err != nil {
		return err
	}

	// deal with subsequent execve()
	if start, exe := trace.getPid(pid); exe != "" {
		trace.addExeRuntime(start, exe, execStart-start, pid)
	}
	trace.addPid(pid, execStart, exe)
	return nil
}

func handleSignalMatch(trace execveTimingTracer, match []string) error {
	if len(match) == 0 {
		return nil
	}
	sigTime, err := strconv.ParseFloat(match[1], 64)
	if err != nil {
		return err
	}
	sigPid := match[3]

	if start, exe := trace.getPid(sigPid); exe != "" {
		trace.addExeRuntime(start, exe, sigTime-start, sigPid)
		trace.deletePid(sigPid)
	}
	return nil
}

func handleSigkillMatch(trace execveTimingTracer, match []string) error {
	if len(match) == 0 {
		return nil
	}
	pid := match[1]
	sigTime, err := strconv.ParseFloat(match[2], 64)
	if err != nil {
		return err
	}

	if start, exe := trace.getPid(pid); exe != "" {
		trace.addExeRuntime(start, exe, sigTime-start, pid)
		trace.deletePid(pid)
	}
	return nil
}

// func handleCloneMatch(trace *ExecveTiming, pct *pidChildTracker, match []string) error {
// 	if len(match) == 0 {
// 		return nil
// 	}
// 	// the pid of the parent process clone()ing a new child
// 	ppid := match[1]

// 	// the time the child was created
// 	execStart, err := strconv.ParseFloat(match[2], 64)
// 	if err != nil {
// 		return err
// 	}

// 	// the pid of the new child
// 	pid := match[3]
// 	pct.Add(ppid, pid, execStart)
// 	return nil
// }

// TraceExecveTimings will read an strace log and produce a timing report of the
// n slowest exec's
func TraceExecveTimings(straceLog string, nSlowest int) (*ExecveTiming, error) {
	slog, err := os.Open(straceLog)
	if err != nil {
		return nil, err
	}
	defer slog.Close()

	// pidChildTracker := newPidChildTracker()

	var line string
	var start, end float64
	var startPID, endPID int
	trace := newExecveTiming(nSlowest)
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
	}
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

	if r.Err() != nil {
		return nil, r.Err()
	}

	return trace, nil
}
