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

package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/anonymouse64/etrace/internal/files"
	"github.com/anonymouse64/etrace/internal/profiling"
	"github.com/anonymouse64/etrace/internal/snaps"
	"github.com/anonymouse64/etrace/internal/strace"
	"github.com/anonymouse64/etrace/internal/xdotool"
)

type cmdFile struct {
	WindowName           string   `short:"w" long:"window-name" description:"Window name to wait for"`
	PrepareScript        string   `short:"p" long:"prepare-script" description:"Script to run to prepare a run"`
	PrepareScriptArgs    []string `long:"prepare-script-args" description:"Args to provide to the prepare script"`
	RestoreScript        string   `short:"r" long:"restore-script" description:"Script to run to restore after a run"`
	RestoreScriptArgs    []string `long:"restore-script-args" description:"Args to provide to the restore script"`
	WindowClass          string   `short:"c" long:"class-name" description:"Window class to use with xdotool instead of the the first Command"`
	RunThroughSnap       bool     `short:"s" long:"use-snap-run" description:"Run command through snap run"`
	DiscardSnapNs        bool     `short:"d" long:"discard-snap-ns" description:"Discard the snap namespace before running the snap"`
	ProgramStdoutLog     string   `long:"cmd-stdout" description:"Log file for run command's stdout"`
	ProgramStderrLog     string   `long:"cmd-stderr" description:"Log file for run command's stderr"`
	JSONOutput           bool     `short:"j" long:"json" description:"Output results in JSON"`
	OutputFile           string   `short:"o" long:"output-file" description:"A file to output the results (empty string means stdout)"`
	NoWindowWait         bool     `long:"no-window-wait" description:"Don't wait for the window to appear, just run until the program exits"`
	FileRegex            string   `long:"file-regex" description:"Regular expression of files to return, if empty all files are returned"`
	ParentDirPaths       []string `long:"parent-dirs" description:"List of parent directories matching files must be underneath to match"`
	ProgramRegex         string   `long:"program-regex" description:"Regular expression of programs whose file accesses should be returned"`
	IncludeSnapdPrograms bool     `long:"include-snapd-programs" description:"Include snapd programs whose file accesses match in the list of files accessed"`
	ShowPrograms         bool     `long:"show-programs" description:"Show programs that accessed the files"`

	Args struct {
		Cmd []string `description:"Command to run" required:"yes"`
	} `positional-args:"yes" required:"yes"`
}

// FileOutputResult is the result of running a command with various information
// encoded in it
type FileOutputResult struct {
	ExecvePaths   *strace.ExecvePaths
	TimeToDisplay time.Duration
	Errors        []error
}

func (x *cmdFile) Execute(args []string) error {
	if !x.NoWindowWait {
		// check if we are running on X11, if not then bail because we don't
		// support graphical window waiting on wayland yet
		sessionType := os.Getenv("XDG_SESSION_TYPE")
		if strings.TrimSpace(strings.ToLower(sessionType)) != "x11" {
			return fmt.Errorf("error: graphical session type %s is unsupported, only x11 is supported", sessionType)
		}
	}

	// check the output file
	w := os.Stdout
	if x.OutputFile != "" {
		// TODO: add option for appending?
		// if the file already exists, delete it and open a new file
		file, err := files.EnsureExistsAndOpen(x.OutputFile, true)
		if err != nil {
			return err
		}
		w = file
	}

	// run the prepare script if it's available
	if x.PrepareScript != "" {
		err := profiling.RunScript(x.PrepareScript, x.PrepareScriptArgs)
		if err != nil {
			logError(fmt.Errorf("running prepare script: %w", err))
		}
	}

	// handle if the command should be run through `snap run`
	targetCmd := x.Args.Cmd
	if x.RunThroughSnap {
		targetCmd = append([]string{"snap", "run"}, targetCmd...)
	}

	var cmd *exec.Cmd
	// setup private tmp dir to use for strace logs
	straceTmp, err := ioutil.TempDir("", "file-trace")
	if err != nil {
		return err
	}
	defer os.RemoveAll(straceTmp)

	// make sure the file doesn't somehow already exist
	straceLog := filepath.Join(straceTmp, "strace.log")
	err = files.EnsureFileIsDeleted(straceLog)
	if err != nil {
		return err
	}

	cmd, err = strace.TraceFilesCommand(straceLog, targetCmd...)
	if err != nil {
		return err
	}

	// setup cmd's streams
	cmd.Stdin = os.Stdin

	// redirect all output from the child process to the log files if they exist
	// otherwise just to this process's stdout, etc.
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if x.ProgramStdoutLog != "" {
		f, err := files.EnsureExistsAndOpen(x.ProgramStdoutLog, false)
		if err != nil {
			return err
		}
		defer f.Close()
		cmd.Stdout = f
	}
	if x.ProgramStderrLog != "" {
		f, err := files.EnsureExistsAndOpen(x.ProgramStderrLog, false)
		if err != nil {
			return err
		}
		defer f.Close()
		cmd.Stderr = f
	}

	if x.DiscardSnapNs {
		if !x.RunThroughSnap {
			return errors.New("cannot use --discard-snap-ns without --use-snap-run")
		}
		// the name of the snap in this case is the first argument
		err := snaps.DiscardSnapNs(x.Args.Cmd[0])
		if err != nil {
			return err
		}
	}

	// handle the file regex
	var fileRegex *regexp.Regexp
	switch {
	case x.FileRegex != "" && len(x.ParentDirPaths) != 0:
		return errors.New("cannot use --file-regex with --parent-dirs")
	case x.FileRegex != "":
		// check that what the user passed in is a correct regex
		fileRegex, err = regexp.Compile(x.FileRegex)
		if err != nil {
			return fmt.Errorf("invalid setting for --file-regex (%q): %v", x.FileRegex, err)
		}
	case len(x.ParentDirPaths) != 0:
		// build the regex to only match files rooted under the specified paths
		// all of the paths are assumed to be directories

		// the start of the capturing group
		fileRegexStr := "("
		for i, dir := range x.ParentDirPaths {
			// escape the slash character since it is a special regexp char
			s := strings.Replace(filepath.Clean(dir), "/", `\/`, -1)
			// then add conditional ending, so that we both catch any files
			// below this directory, as well as this directory itself
			s += `/.*`
			// add to the regex
			fileRegexStr += s
			// on all dirs except the last one, add a "|" to or the path
			if i != len(x.ParentDirPaths)-1 {
				fileRegexStr += "|"
			}
		}
		fileRegexStr += ")"

		fileRegex, err = regexp.Compile(fileRegexStr)
		if err != nil {
			return fmt.Errorf("internal error compiling regex for --parent-dirs setting (%v): %v", x.ParentDirPaths, err)
		}
	default:
		// default case is to match all files, so use ".*" as the regexp
		fileRegex = regexp.MustCompile(".*")
	}

	// now handle the executable program patterns
	var programRegex *regexp.Regexp

	if x.ProgramRegex != "" {
		programRegex, err = regexp.Compile(x.ProgramRegex)
		if err != nil {
			return fmt.Errorf("invalid setting for --program-regex (%q): %v", x.ProgramRegex, err)
		}
	} else {
		// include all programs
		programRegex = regexp.MustCompile(".*")
	}

	// ideally we would use a negative lookahead regex to implement exclusion listing
	// certain programs in the programRegex, but go doesn't support those and
	// I'm not ready to jump ship to use a non-stdlib regex lib, so for now
	// we will just use globs to match snap binaries from /usr/lib/snapd/
	// /snap/core/*/<snap tool path> and /snap/snapd/*/<snap tool path>
	excludeListProgramPatterns := []string{
		// all installs
		"/usr/bin/snap",
		"/usr/lib/snapd/*",
		"/sbin/apparmor_parser",

		// core snap programs
		"/snap/core/*/usr/bin/snap",
		"/snap/core/*/usr/lib/snapd/*",

		// snapd snap
		"/snap/snapd/*/usr/bin/snap",
		"/snap/snapd/*/usr/lib/snapd/*",
	}
	if x.IncludeSnapdPrograms {
		excludeListProgramPatterns = []string{}
	}

	xtool := xdotool.MakeXDoTool()

	tryXToolClose := true
	var wids []string

	windowspec := xdotool.Window{}
	// check which opts are defined
	if x.WindowClass != "" {
		// prefer window class from option
		windowspec.Class = x.WindowClass
	} else if x.WindowName != "" {
		// then window name
		windowspec.Name = x.WindowName
	} else {
		// finally fall back to base cmd as the class
		// note we use the original command and note the processed targetCmd
		// because for example when measuring a snap, we invoke etrace like so:
		// $ ./etrace run --use-snap chromium
		// where targetCmd becomes []string{"snap","run","chromium"}
		// but we still want to use "chromium" as the windowspec class
		windowspec.Class = filepath.Base(x.Args.Cmd[0])
	}

	// before running the final command, free the caches to get most accurate
	// timing
	err = profiling.FreeCaches()
	if err != nil {
		return err
	}

	// start running the command
	start := time.Now()
	err = cmd.Start()

	if x.NoWindowWait {
		// if we aren't waiting on the window class, then just wait for the
		// command to return
		cmd.Wait()
	} else {
		// now wait until the window appears
		wids, err = xtool.WaitForWindow(windowspec)
		if err != nil {
			logError(fmt.Errorf("waiting for window appearance: %w", err))
			// if we don't get the wid properly then we can't try closing
			tryXToolClose = false
		}
	}

	// save the startup time
	startup := time.Since(start)

	// now get the pids before closing the window so we can gracefully try
	// closing the windows before forcibly killing them later
	if tryXToolClose {
		pids := make([]int, len(wids))
		for i, wid := range wids {
			pid, err := xtool.PidForWindowID(wid)
			if err != nil {
				logError(fmt.Errorf("getting pid for wid %s: %w", wid, err))
				break
			}
			pids[i] = pid
		}

		// close the windows
		for _, wid := range wids {
			err = xtool.CloseWindowID(wid)
			if err != nil {
				logError(fmt.Errorf("closing window: %w", err))
			}
		}

		// kill the app pids in case x fails to close the window
		for _, pid := range pids {
			// FindProcess always succeeds on unix
			proc, _ := os.FindProcess(pid)
			if err := proc.Signal(os.Kill); err != nil {
				// if the process already exited then try wmctrl
				if !strings.Contains(err.Error(), "process already finished") {
					logError(fmt.Errorf("killing window process pid %d: %w", pid, err))
				}
			}
		}
	}

	// parse the strace log
	execFiles, err := strace.TraceExecveWithFiles(
		straceLog,
		fileRegex,
		programRegex,
		excludeListProgramPatterns,
	)
	if err != nil {
		logError(fmt.Errorf("cannot extract runtime data: %w", err))
	}

	if !x.JSONOutput {
		// make a new tabwriter to stderr
		wtab := tabWriterGeneric(w)
		opts := &strace.DisplayOptions{}
		if !x.ShowPrograms {
			opts.NoDisplayPrograms = true
		}
		execFiles.Display(wtab, opts)
	}

	if x.RestoreScript != "" {
		err := profiling.RunScript(x.RestoreScript, x.RestoreScriptArgs)
		if err != nil {
			logError(fmt.Errorf("running restore script: %w", err))
		}
	}

	outRes := FileOutputResult{
		TimeToDisplay: startup,
		Errors:        errs,
		ExecvePaths:   execFiles,
	}

	// if we're not tracing then just use startup time as time to run

	if x.JSONOutput {
		json.NewEncoder(w).Encode(outRes)
	} else {
		fmt.Fprintln(w, "Total startup time:", startup)
	}

	return nil
}
