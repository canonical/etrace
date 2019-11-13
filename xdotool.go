package main

import (
	"log"
	"os/exec"
	"strconv"
	"strings"
)

type xdotool struct{}

type window struct {
	class string
	name  string
}

type xtooler interface {
	waitForWindow(w window) ([]string, error)
	closeWindowID(wid string) error
	pidForWindowID(wid string) (int, error)
}

func makeXDoTool() xtooler {
	return &xdotool{}
}

func (x *xdotool) waitForWindow(w window) ([]string, error) {
	if w.class != "" {
		return x.waitForWindowArgs([]string{"--class", w.class})
	} else if w.name != "" {
		return x.waitForWindowArgs([]string{"--name", w.name})
	} else {

	}
	windowids := []string{}
	var err error
	out := []byte{}
	for i := 0; i < 10; i++ {
		out, err = exec.Command("xdotool", "search", "--sync", "--onlyvisible", "--class", w.class).CombinedOutput()
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
