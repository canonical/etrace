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
	"io"
	"log"
	"os"
	"os/exec"
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
	_, err := exec.LookPath("sudo")
	if err != nil {
		log.Fatalf("cannot find sudo: %s", err)
	}

	log.SetFlags(log.LstdFlags | log.Lshortfile)
	_, err = parser.Parse()
	if err != nil {
		os.Exit(1)
	}
}

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
