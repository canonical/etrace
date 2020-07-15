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
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"time"

	"github.com/anonymouse64/etrace/internal/commands"

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
	CleanSnapUserData bool     `long:"clean-snap-user-data" description:"Delete snap user data before executing and restore after execution"`
	ReinstallSnap     bool     `long:"reinstall-snap" description:"Reinstall the snap before executing, restoring any existing interface connections for the snap"`

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
	max := uint(1)
	if currentCmd.Repeat > 0 {
		max = currentCmd.Repeat
	}

	// TODO: ensure the snap is installed if the option --use-snap-run is set

	// first if we are operating on a snap, then use snap save to save the data
	// into a snapshot before running anything
	snapName := x.Args.Cmd[0]

	if x.CleanSnapUserData {
		saveCmd := exec.Command("snap", "save", snapName)
		err := commands.AddSudoIfNeeded(saveCmd)
		if err != nil {
			return fmt.Errorf("failed to add sudo to command: %v", err)
		}
		saveOut, err := saveCmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("failed to save snapshot of snap user data for snap %s before deleting it: %v (%s)", snapName, err, string(saveOut))
		}

		// get the snapshot ID from the output
		s := bufio.NewScanner(bytes.NewReader(saveOut))
		for s.Scan() {
			line := s.Text()
			if strings.Contains(line, snapName) {
				fields := strings.Fields(line)
				snapshotID := fields[0]

				// defer a restore of the snapshot ID for this snap
				defer func() {
					restoreCmd := exec.Command("snap", "restore", snapshotID, snapName)
					err := commands.AddSudoIfNeeded(restoreCmd)
					if err != nil {
						fmt.Fprintf(os.Stderr, "failed to restore snapshot %s for snap %s: %v", snapshotID, snapName, err)
					}
					restoreOut, err := restoreCmd.CombinedOutput()
					if err != nil {
						fmt.Fprintf(os.Stderr, "failed to restore snapshot %s for snap %s: %v (%s)", snapshotID, snapName, err, string(restoreOut))
					}
				}()

				break
			}
		}

		// now delete all the /home/*/snap/$SNAP_NAME/ directories, these are
		// normally not deleted when the snap is removed but the user asked us
		// to do this explicitly
		homeSnapUserDataPattern := filepath.Join("/home/*/snap/", snapName)
		snapUserDataDirs, err := filepath.Glob(homeSnapUserDataPattern)
		if err != nil {
			return fmt.Errorf("poorgramming error: glob pattern wrong: %v", err)
		}
		// get root's snap user data too if it's there
		rootSnapUserDataDir := filepath.Join("/root/snap/", snapName)
		_, err = os.Stat(rootSnapUserDataDir)
		if err == nil {
			snapUserDataDirs = append(snapUserDataDirs, rootSnapUserDataDir)
		}

		for _, dir := range snapUserDataDirs {
			rmCmd := exec.Command("rm", "-rf", dir)
			err := commands.AddSudoIfNeeded(rmCmd)
			if err != nil {
				return fmt.Errorf("failed to add sudo to command: %v", err)
			}
			rmOut, err := rmCmd.CombinedOutput()
			if err != nil {
				return fmt.Errorf("failed to delete snap user data directory %s: %v (%s)", dir, err, string(rmOut))
			}
		}
	}

	for i := uint(0); i < max; i++ {
		// if we were supposed to reinstall the snap before the test, do that
		// first
		if x.ReinstallSnap {
			var isClassic, isDevmode, isJailmode, isUnaliased bool
			snapName := x.Args.Cmd[0]

			// save interface connections
			conns, err := snaps.CurrentConnections(snapName)
			if err != nil {
				return err
			}

			// get the current snap file for the installed snap
			rev, err := snaps.Revision(snapName)
			if err != nil {
				return err
			}

			snapFileName := fmt.Sprintf("%s_%s.snap", snapName, rev)
			tmpSnap := filepath.Join("/tmp/", snapFileName)
			snapFileSrc := filepath.Join("/var/lib/snapd/snaps", snapFileName)

			cpCmd := exec.Command("cp", snapFileSrc, tmpSnap)
			err = commands.AddSudoIfNeeded(cpCmd)
			if err != nil {
				return fmt.Errorf("failed to add sudo to command: %v", err)
			}
			cpOut, err := cpCmd.CombinedOutput()
			if err != nil {
				return fmt.Errorf("failed to copy snap %s: %v (%s)", snapFileSrc, err, string(cpOut))
			}

			// get the install options for the snap
			infoOut, err := exec.Command("snap", "info", snapName).CombinedOutput()
			if err != nil {
				return fmt.Errorf("failed to get snap info for snap %s: %v (%s)", snapName, err, string(infoOut))
			}

			s := bufio.NewScanner(bytes.NewReader(infoOut))

			for s.Scan() {
				line := s.Text()
				if strings.HasPrefix(line, "installed:") {
					fields := strings.Fields(line)
					if len(fields) != 5 {
						return fmt.Errorf("unexpected snap info output: snap info installed line does not have 5 fields")
					}

					// we only care about the last field, the options which will
					// be comma delimited
					for _, opt := range strings.Split(fields[4], ",") {
						switch opt {
						case "try":
							return fmt.Errorf("snap %s is installed as a try snap, etrace does not yet support reinstalling try snaps", snapName)
						case "classic":
							isClassic = true
						case "devmode":
							isDevmode = true
						case "jailmode":
							isJailmode = true
						case "isUnaliased":
							isUnaliased = true
						case "disabled":
							return fmt.Errorf("snap %s is disabled, refusing to remove and reinstall, please enable first with snap enable", snapName)
						case "blocked":
							// TODO: what should one do about a blocked snap?
							// return fmt.Errorf("snap %s is blocked, please see warnings from snap info to proceed", snapName)
						case "broken":
							return fmt.Errorf("snap %s is broken, please fix before continuing", snapName)
						}
					}
				}
			}

			// now remove the snap
			removeOut, err := exec.Command("snap", "remove", snapName).CombinedOutput()
			if err != nil {
				return fmt.Errorf("failed to remove snap %s: %v (%s)", snapName, err, string(removeOut))
			}

			// now reinstall the snap
			installCmd := exec.Command("snap", "install", tmpSnap)
			if isClassic {
				installCmd.Args = append(installCmd.Args, "--classic")
			}
			if isJailmode {
				installCmd.Args = append(installCmd.Args, "--jailmode")
			}
			if isDevmode {
				installCmd.Args = append(installCmd.Args, "--devmode")
			}
			if isUnaliased {
				installCmd.Args = append(installCmd.Args, "--unaliased")
			}

			// if the snap revision number doesn't consist of just numbers, it
			// is a dangerous unasserted revision and needs --dangerous
			if !regexp.MustCompile("^[0-9]+$").Match([]byte(rev)) {
				installCmd.Args = append(installCmd.Args, "--dangerous")
			}

			err = commands.AddSudoIfNeeded(installCmd)
			if err != nil {
				return fmt.Errorf("failed to add sudo if needed: %v", err)
			}
			_, err = installCmd.CombinedOutput()
			if err != nil {
				return fmt.Errorf("failed to install snap using command %v: %v", installCmd.Args, err)
			}

			// restore the interface connections
			for _, conn := range conns {
				err := snaps.ApplyConnection(conn)
				if err != nil {
					return fmt.Errorf("failed to restore connections for snap %s: %v", snapName, err)
				}
			}
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
