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
	"log"
	"os/exec"
	"strconv"
	"strings"
)

type xdotool struct{}

// Window represents a X11 window
type Window struct {
	Class string
	Name  string
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
	if w.Class != "" {
		return x.waitForWindowArgs([]string{"--class", w.Class})
	} else if w.Name != "" {
		return x.waitForWindowArgs([]string{"--name", w.Name})
	} else {
		// what was I thinking here again?
	}
	windowids := []string{}
	var err error
	out := []byte{}
	for i := 0; i < 10; i++ {
		out, err = exec.Command("xdotool", "search", "--sync", "--onlyvisible", "--class", w.Class).CombinedOutput()
		if err != nil {
			continue
		}
		windowids = strings.Split(strings.TrimSpace(string(out)), "\n")
		return windowids, nil
	}
	log.Println(string(out))
	return nil, err
}

func (x *xdotool) waitForWindowArgs(searchArgs []string) ([]string, error) {
	windowids := []string{}
	var err error
	out := []byte{}
	for i := 0; i < 10; i++ {
		out, err = exec.Command("xdotool", append([]string{"search", "--sync", "--onlyvisible"}, searchArgs...)...).CombinedOutput()
		if err != nil {
			continue
		}
		windowids = strings.Split(strings.TrimSpace(string(out)), "\n")
		return windowids, nil
	}
	log.Println(string(out))
	return nil, err
}

func (x *xdotool) CloseWindowID(wid string) error {
	out, err := exec.Command("xdotool", "windowkill", wid).CombinedOutput()
	if err != nil {
		log.Println(string(out))
		return err
	}
	return nil
}

func (x *xdotool) PidForWindowID(wid string) (int, error) {
	out, err := exec.Command("xdotool", "getwindowpid", wid).CombinedOutput()
	if err != nil {
		log.Println(string(out))
		return 0, err
	}
	return strconv.Atoi(strings.TrimSpace(string(out)))
}
