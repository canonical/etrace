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
	"fmt"
	"os/exec"
	"os/user"
)

// These syscalls are excluded because they make strace hang on all or
// some architectures (gettimeofday on arm64).
var excludedSyscalls = "!select,pselect6,_newselect,clock_gettime,sigaltstack,gettid,gettimeofday,nanosleep"

// Command returns how to run strace in the users context with the
// right set of excluded system calls.
func straceCommand(extraStraceOpts []string, traceeCmd ...string) (*exec.Cmd, error) {
	args := []string{}

	current, err := user.Current()
	if err != nil {
		return nil, err
	}
	if current.Uid != "0" {
		sudoPath, err := exec.LookPath("sudo")
		if err != nil {
			return nil, fmt.Errorf("cannot use strace without sudo: %s", err)
		}
		args = append(args,
			sudoPath,
			"-E",
		)
	}

	stracePath, err := exec.LookPath("strace")
	if err != nil {
		return nil, fmt.Errorf("cannot find an installed strace, please try 'snap install strace-static'")
	}

	args = append(args,
		stracePath,
		"-u", current.Username,
		"-f",
		"-e", excludedSyscalls,
	)
	args = append(args, extraStraceOpts...)
	args = append(args, traceeCmd...)

	return &exec.Cmd{
		Path: args[0],
		Args: args,
	}, nil
}

// TraceExecCommand returns an exec.Cmd suitable for tracking timings of
// execve{,at}() calls
func TraceExecCommand(straceLogPath string, origCmd ...string) (*exec.Cmd, error) {
	extraStraceOpts := []string{"-ttt", "-e", "trace=execve,execveat", "-o", fmt.Sprintf("%s", straceLogPath)}

	return straceCommand(extraStraceOpts, origCmd...)
}

// TraceFilesCommand returns an exec.Cmd suitable for tracking files opened/used
// during execution
func TraceFilesCommand(straceLogPattern string, origCmd ...string) (*exec.Cmd, error) {
	extraStraceOpts := []string{
		// we don't need timing info here, but we need to re-merge the
		// logs, with strace-log-merge, and to work across day changes, this is
		// recommended
		"-ttt",
		// this is to make parsing easier since we don't care about time
		// performance, splitting the output up by process ensures that we will
		// never get output that has a syscall interrupted which is hard to
		// match and parse properly
		"-ff",
		// we don't care about the file contents, we also specifically don't
		// want to get confused if a given read() or write() has the filepath we
		// care about in the content being written, so just don't show any at
		//all
		"-s0",
		// we also want to capture things accessing file descriptors too, so
		// this makes the strace output append </path/to/file/or/dir> wherever
		// a file descriptor shows up
		"-y",
		// this is not a filename, it's a pattern that strace will use, the
		// actual filenames will have their pid appended to this filename in a
		// way that strace-log-merge can understand easily
		"-o", fmt.Sprintf("%s", straceLogPattern),
	}

	return straceCommand(extraStraceOpts, origCmd...)
}
