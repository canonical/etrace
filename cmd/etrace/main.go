/*
 * Copyright (C) 2019-2021 Canonical Ltd
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
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"text/tabwriter"

	flags "github.com/jessevdk/go-flags"
)

// Command is the command for the runner
type Command struct {
	File                    cmdFile        `command:"file" description:"Trace files accessed from a program"`
	Exec                    cmdExec        `command:"exec" description:"Trace the program executions from a program"`
	AnalyzeSnap             cmdAnalyzeSnap `command:"analyze-snap" description:"Analyze a snap for performance data"`
	ShowErrors              bool           `short:"e" long:"errors" description:"Show errors as they happen"`
	WindowName              string         `short:"w" long:"window-name" description:"Window name to wait for"`
	PrepareScript           string         `short:"p" long:"prepare-script" description:"Script to run to prepare a run"`
	PrepareScriptArgs       []string       `long:"prepare-script-args" description:"Args to provide to the prepare script"`
	RestoreScript           string         `short:"r" long:"restore-script" description:"Script to run to restore after a run"`
	RestoreScriptArgs       []string       `long:"restore-script-args" description:"Args to provide to the restore script"`
	KeepVMCaches            bool           `short:"v" long:"keep-vm-caches" description:"Don't free VM caches before executing"`
	WindowClass             string         `short:"c" long:"class-name" description:"Window class to use with xdotool instead of the the first Command"`
	WindowClassName         string         `long:"window-class-name" description:"Window class name to use with xdotool"`
	RunThroughSnap          bool           `short:"s" long:"use-snap-run" description:"Run command through snap run"`
	RunThroughFlatpak       bool           `short:"f" long:"use-flatpak-run" description:"Run command through flatpak run"`
	DiscardSnapNs           bool           `short:"d" long:"discard-snap-ns" description:"Discard the snap namespace before running the snap"`
	ProgramStdoutLog        string         `long:"cmd-stdout" description:"Log file for run command's stdout"`
	ProgramStderrLog        string         `long:"cmd-stderr" description:"Log file for run command's stderr"`
	SilentProgram           bool           `long:"silent" description:"Silence all program output"`
	JSONOutput              bool           `short:"j" long:"json" description:"Output results in JSON"`
	OutputFile              string         `short:"o" long:"output-file" description:"A file to output the results (empty string means stdout)"`
	NoWindowWait            bool           `long:"no-window-wait" description:"Don't wait for the window to appear, just run until the program exits"`
	WindowWaitGlobalTimeout string         `long:"window-timeout" default:"60s" description:"Global timeout for waiting for windows to appear. Set to empty string to use no timeout"`
}

// The current input command
var currentCmd Command
var parser = flags.NewParser(&currentCmd, flags.Default)

func main() {
	// first check if we are under an apparmor profile, in which case we need
	// to drop that because it affects tracing and leads to denials
	// unfortunately

	// TODO: move this to it's own pkg?

	label, err := ioutil.ReadFile("/proc/self/attr/apparmor/current")
	if err != nil && !os.IsNotExist(err) {
		// failed to read apparmor label
		log.Fatalf("cannot read apparmor label")
	}

	if os.IsNotExist(err) {
		// try the legacy apparmor path
		label, err = ioutil.ReadFile("/proc/self/attr/current")
		if err != nil && !os.IsNotExist(err) {
			log.Fatalf("cannot read apparmor label")
		}
	}

	// if we read the file successfully, this system has apparmor enabled and we
	// can read our own label
	if err == nil {
		// if the label is anything other than unconfined, we should try to
		// re-exec and drop our apparmor label for the most accurate testing
		if strings.TrimSpace(string(label)) != "unconfined" {
			// write "exec unconfined" to the apparmor label for us and then
			// re-exec
			f, err := os.OpenFile("/proc/self/attr/exec", os.O_WRONLY, 0)
			if err != nil {
				log.Fatalf("could not open process exec attr to transition apparmor profile: %v", err)
			}
			defer f.Close()

			// TODO: should we be extra safe like runc and verify that we are
			//       writing to something in procfs? see https://github.com/opencontainers/runc/commit/d463f6485b809b5ea738f84e05ff5b456058a184

			_, err = fmt.Fprintf(f, "%s", "exec unconfined")
			if err != nil {
				log.Fatalf("could not set process exec attr to unconfined: %v", err)
			}

			// now we are ready to re-exec ourselves before we re-wreck
			// ourselves
			if err := syscall.Exec("/proc/self/exe", os.Args, os.Environ()); err != nil {
				log.Fatalf("failed to re-exec: %v", err)
			}

			// should be impossible to reach here
		}
	}

	_, err = exec.LookPath("sudo")
	if err != nil {
		log.Fatalf("cannot find sudo: %s", err)
	}

	log.SetFlags(log.LstdFlags | log.Lshortfile)
	_, err = parser.Parse()
	if err != nil {
		os.Exit(1)
	}
}

// TODO: move this somewhere else
func tabWriterGeneric(w io.Writer) *tabwriter.Writer {
	return tabwriter.NewWriter(w, 5, 3, 2, ' ', 0)
}

var errs []string

func resetErrors() {
	errs = nil
}

func logError(err error) {
	errs = append(errs, err.Error())
	if currentCmd.ShowErrors {
		log.Println(err)
	}
}
