package main

import (
	"log"
	"os/exec"
	"strconv"
	"strings"
)

type xdotool struct{}

type xtooler interface {
	waitForWindows(prog string) ([]string, error)
	closeWindowID(wid string) error
	pidForWindowID(wid string) (int, error)
}

func makeXDoTool() xtooler {
	return &xdotool{}
}

func (x *xdotool) waitForWindows(name string) ([]string, error) {
	windowids := []string{}
	var err error
	out := []byte{}
	for i := 0; i < 10; i++ {
		out, err = exec.Command("xdotool", "search", "--sync", "--onlyvisible", "--class", name).CombinedOutput()
		if err != nil {
			continue
		}
		windowids = strings.Split(strings.TrimSpace(string(out)), "\n")
		return windowids, nil
	}
	log.Println(string(out))
	return nil, err
}

func (x *xdotool) closeWindowID(wid string) error {
	out, err := exec.Command("xdotool", "windowkill", wid).CombinedOutput()
	if err != nil {
		log.Println(string(out))
		return err
	}
	return nil
}

func (x *xdotool) pidForWindowID(wid string) (int, error) {
	out, err := exec.Command("xdotool", "getwindowpid", wid).CombinedOutput()
	if err != nil {
		log.Println(string(out))
		return 0, err
	}
	return strconv.Atoi(strings.TrimSpace(string(out)))
}
