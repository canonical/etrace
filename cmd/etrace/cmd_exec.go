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
	"strings"
	"syscall"
	"time"

	"github.com/anonymouse64/etrace/internal/files"
	"github.com/anonymouse64/etrace/internal/profiling"
	"github.com/anonymouse64/etrace/internal/snaps"
	"github.com/anonymouse64/etrace/internal/strace"
	"github.com/anonymouse64/etrace/internal/xdotool"
)

// ExecOutputResult is the result of running a command with various information
// encoded in it
type ExecOutputResult struct {
	Runs []Execution
}

// Execution represents a single run
type Execution struct {
	ExecveTiming  *strace.ExecveTiming
	TimeToDisplay time.Duration
	TimeToRun     time.Duration
	Errors        []error
}

type cmdExec struct {
	WindowName        string   `short:"w" long:"window-name" description:"Window name to wait for"`
	PrepareScript     string   `short:"p" long:"prepare-script" description:"Script to run to prepare a run"`
	PrepareScriptArgs []string `long:"prepare-script-args" description:"Args to provide to the prepare script"`
	RestoreScript     string   `short:"r" long:"restore-script" description:"Script to run to restore after a run"`
	RestoreScriptArgs []string `long:"restore-script-args" description:"Args to provide to the restore script"`
	WindowClass       string   `short:"c" long:"class-name" description:"Window class to use with xdotool instead of the the first Command"`
	NoTrace           bool     `short:"t" long:"no-trace" description:"Don't trace the process, just time the total execution"`
	RunThroughSnap    bool     `short:"s" long:"use-snap-run" description:"Run command through snap run"`
	DiscardSnapNs     bool     `short:"d" long:"discard-snap-ns" description:"Discard the snap namespace before running the snap"`
	ProgramStdoutLog  string   `long:"cmd-stdout" description:"Log file for run command's stdout"`
	ProgramStderrLog  string   `long:"cmd-stderr" description:"Log file for run command's stderr"`
	JSONOutput        bool     `short:"j" long:"json" description:"Output results in JSON"`
	OutputFile        string   `short:"o" long:"output-file" description:"A file to output the results (empty string means stdout)"`
	NoWindowWait      bool     `long:"no-window-wait" description:"Don't wait for the window to appear, just run until the program exits"`

	Args struct {
		Cmd []string `description:"Command to run" required:"yes"`
	} `positional-args:"yes" required:"yes"`
}

type straceResult struct {
	timings *strace.ExecveTiming
	err     error
}

func (x *cmdExec) Execute(args []string) error {
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

	outRes := ExecOutputResult{}
	i := uint(0)
	for i = 0; i < 1+currentCmd.AdditionalIterations; i++ {
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

		doneCh := make(chan straceResult, 1)
		var slg *strace.ExecveTiming
		var cmd *exec.Cmd
		var fw *os.File
		if !x.NoTrace {
			// setup private tmp dir with strace fifo
			straceTmp, err := ioutil.TempDir("", "exec-trace")
			if err != nil {
				return err
			}
			defer os.RemoveAll(straceTmp)
			straceLog := filepath.Join(straceTmp, "strace.fifo")
			if err := syscall.Mkfifo(straceLog, 0640); err != nil {
				return err
			}
			// ensure we have one writer on the fifo so that if strace fails
			// nothing blocks
			fw, err = os.OpenFile(straceLog, os.O_RDWR, 0640)
			if err != nil {
				return err
			}
			defer fw.Close()

			// read strace data from fifo async
			go func() {
				timing, err := strace.TraceExecveTimings(straceLog, -1)
				doneCh <- straceResult{timings: timing, err: err}
				close(doneCh)
			}()

			cmd, err = strace.TraceExecCommand(straceLog, targetCmd...)
			if err != nil {
				return err
			}
		} else {
			// Don't setup tracing, so just use exec.Command directly
			// x.Args.Cmd (and thus targetCmd) is guaranteed to be at least one
			// element given that it is a required argument
			prog := targetCmd[0]
			var args []string
			// setup args if there's more than 1
			if len(targetCmd) > 1 {
				args = targetCmd[1:]
			}
			cmd = exec.Command(prog, args...)
		}

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

		xtool := xdotool.MakeXDoTool()

		tryXToolClose := true
		tryWmctrl := false
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
		err := profiling.FreeCaches()
		if err != nil {
			return err
		}

		// start running the command
		start := time.Now()
		err = cmd.Start()

		if !x.NoWindowWait {
			// now wait until the window appears
			wids, err = xtool.WaitForWindow(windowspec)
			if err != nil {
				logError(fmt.Errorf("waiting for window appearance: %w", err))
				// if we don't get the wid properly then we can't try closing
				tryXToolClose = false
			}
		}

		if x.NoWindowWait || len(wids) == 0 {
			// if we aren't waiting on the window class, then just wait for the
			// command to return
			if err := cmd.Wait(); err != nil {
				logError(fmt.Errorf("waiting for command: %w", err))
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
					tryWmctrl = true
					break
				}
				pids[i] = pid
			}

			// close the windows
			for _, wid := range wids {
				err = xtool.CloseWindowID(wid)
				if err != nil {
					logError(fmt.Errorf("closing window: %w", err))
					tryWmctrl = true
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
						tryWmctrl = true
					}
				}
			}
		}

		if tryWmctrl {
			err = wmctrlCloseWindow(x.WindowName)
			if err != nil {
				logError(fmt.Errorf("closing window with wmctrl: %w", err))
			}
		}

		if !x.NoTrace {
			// ensure we close the fifo here so that the strace.TraceExecCommand()
			// helper gets a EOF from the fifo (i.e. all writers must be closed
			// for this)
			fw.Close()

			// wait for strace reader
			straceRes := <-doneCh
			if straceRes.err == nil {
				slg = straceRes.timings
				// make a new tabwriter to stderr
				if !x.JSONOutput {
					wtab := tabWriterGeneric(w)
					slg.Display(wtab)
				}
			} else {
				logError(fmt.Errorf("cannot extract runtime data: %w", straceRes.err))
				return straceRes.err
			}
		}

		if x.RestoreScript != "" {
			err := profiling.RunScript(x.RestoreScript, x.RestoreScriptArgs)
			if err != nil {
				logError(fmt.Errorf("running restore script: %w", err))
			}
		}

		run := Execution{
			ExecveTiming:  slg,
			TimeToDisplay: startup,
			Errors:        errs,
		}

		// if we're not tracing then just use startup time as time to run
		if x.NoTrace {
			run.TimeToRun = startup
		} else {
			run.TimeToRun = slg.TotalTime
		}

		// add the run to our result
		outRes.Runs = append(outRes.Runs, run)

		if !x.JSONOutput {
			fmt.Fprintln(w, "Total startup time:", startup)
		}

		resetErrors()
	}

	if x.JSONOutput {
		json.NewEncoder(w).Encode(outRes)
	}

	return nil
}
