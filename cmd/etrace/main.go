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
	File       cmdFile `command:"file" description:"Trace files accessed from a program"`
	Exec       cmdExec `command:"exec" description:"Trace the program executions from a program"`
	ShowErrors bool    `short:"e" long:"errors" description:"Show errors as they happen"`
	Repeat     uint    `short:"n" long:"repeat" description:"Number of times to repeat each task"`
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

// TODO: move to an internal pkg, maybe commands or generalize xdotool?
func wmctrlCloseWindow(name string) error {
	out, err := exec.Command("wmctrl", "-c", name).CombinedOutput()
	if err != nil {
		log.Println(string(out))
		return err
	}
	return nil
}

var errs []error

func resetErrors() {
	errs = nil
}

func logError(err error) {
	errs = append(errs, err)
	if currentCmd.ShowErrors {
		log.Println(err)
	}
}
