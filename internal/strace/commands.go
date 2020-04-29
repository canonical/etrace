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

	"github.com/anonymouse64/etrace/internal/commands"
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

	stracePath, err := exec.LookPath("strace")
	if err != nil {
		return nil, fmt.Errorf("cannot find an installed strace, please try 'snap install strace-static'")
	}

	args := []string{
		stracePath,
		"-u", current.Username,
		"-f",
		"-e", excludedSyscalls,
	}
	args = append(args, extraStraceOpts...)
	args = append(args, traceeCmd...)

	cmd := &exec.Cmd{
		Path: args[0],
		Args: args,
	}

	err = commands.AddSudoIfNeeded(cmd, "-E")
	if err != nil {
		return nil, err
	}
	return cmd, nil
}

// TraceExecCommand returns an exec.Cmd suitable for tracking timings of
// execve{,at}() calls
func TraceExecCommand(straceLogPath string, origCmd ...string) (*exec.Cmd, error) {
	extraStraceOpts := []string{
		// we want maximum timing accuracy for measuring exec's
		"-ttt",
		// only trace the execve syscalls
		"-e", "trace=execve,execveat",
		// the output file to use (this is usually a fifo for best performance)
		"-o", straceLogPath,
	}

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
		// care about in the content being written or read, so just don't show
		// any strings
		"-s0",
		// we also want to capture things accessing file descriptors too, so
		// this makes the strace output append </path/to/file/or/dir> wherever
		// a file descriptor shows up
		"-y",
		// don't output any verbose structures as they may have strings in them
		// that aren't files, such as:
		// recvfrom(7<socket:[624422]>, ""..., 2048, 0, {sa_family=AF_INET, sin_port=htons(53), sin_addr=inet_addr("127.0.0.53")}, [28->16])
		// we don't want to match 127.0.0.53, and instead with this option set
		// we will get the much less ambiguous:
		// recvfrom(6<socket:[644672]>, ""..., 65536, 0, 0x7f895afcc0e0, 0x7f895afcc0c0) = 68
		"-everbose=none",
		// since we also specify the "ff" option, this is not a verbatim
		// filename that strace outputs to, it's now used as a pattern that
		// strace will use to create the actual filenames, with the pid for each
		// appended to the pattern - this is consistent with how
		// strace-log-merge expects the files to be named
		"-o", straceLogPattern,
	}

	return straceCommand(extraStraceOpts, origCmd...)
}
