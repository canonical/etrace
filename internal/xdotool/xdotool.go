// -*- Mode: Go; indent-tabs-mode: t -*-

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

package xdotool

import (
	"bytes"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

type xdotool struct{}

// Window represents a X11 window
type Window struct {
	Class     string
	ClassName string
	Name      string
}

func (w Window) windowSpecErrDescription() string {
	if w.Class != "" {
		return fmt.Sprintf("class %s", w.Class)
	} else if w.Name != "" {
		return fmt.Sprintf("name %s", w.Name)
	} else if w.ClassName != "" {
		return fmt.Sprintf("class name %s", w.ClassName)
	} else {
		return "no specification"
	}
}

func (w Window) searchArgs() []string {
	if w.Class != "" {
		return []string{"--class", w.Class}
	} else if w.Name != "" {
		return []string{"--name", w.Name}
	} else if w.ClassName != "" {
		return []string{"--classname", w.ClassName}
	}
	return nil
}

// Xtooler works with xdotool to perform various operations on X11 windows
type Xtooler interface {
	WaitForWindow(w Window) ([]string, error)
	CloseWindowID(wid string) error
	PidForWindowID(wid string) (int, error)
}

// MakeXDoTool returns a Xtooler that can interact with windows
func MakeXDoTool() Xtooler {
	return &xdotool{}
}

func (x *xdotool) WaitForWindow(w Window) ([]string, error) {
	searchArgs := w.searchArgs()
	if searchArgs == nil {
		return nil, fmt.Errorf("window specification is empty")
	}

	var err error
	out := []byte{}
	for i := 0; i < 10; i++ {
		out, err = exec.Command("xdotool", append([]string{"search", "--sync", "--onlyvisible"}, searchArgs...)...).CombinedOutput()
		if err != nil {
			continue
		}
		return strings.Split(strings.TrimSpace(string(out)), "\n"), nil
	}
	return nil, fmt.Errorf("xdotool failed to find window with %s: %v", w.windowSpecErrDescription(), outputErr(out, err))
}

func (x *xdotool) CloseWindowID(wid string) error {
	out, err := exec.Command("xdotool", "windowkill", wid).CombinedOutput()
	if err != nil {
		return fmt.Errorf("xdotool failed to close window ID %s: %v", wid, outputErr(out, err))
	}
	return nil
}

func (x *xdotool) PidForWindowID(wid string) (int, error) {
	out, err := exec.Command("xdotool", "getwindowpid", wid).CombinedOutput()
	if err != nil {
		return 0, fmt.Errorf("xdotool failed to get pid for window ID %s: %v", wid, outputErr(out, err))
	}
	return strconv.Atoi(strings.TrimSpace(string(out)))
}

// outputErr formats an error based on output if its length is not zero,
// or returns err otherwise.
// copied from osutil package in snapd to avoid having to directly import snapd
func outputErr(output []byte, err error) error {
	output = bytes.TrimSpace(output)
	if len(output) > 0 {
		if bytes.Contains(output, []byte{'\n'}) {
			err = fmt.Errorf("\n-----\n%s\n-----", output)
		} else {
			err = fmt.Errorf("%s", output)
		}
	}
	return err
}
