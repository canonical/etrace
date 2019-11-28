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
	current, err := user.Current()
	if err != nil {
		return nil, err
	}
	sudoPath, err := exec.LookPath("sudo")
	if err != nil {
		return nil, fmt.Errorf("cannot use strace without sudo: %s", err)
	}

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
