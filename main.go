package main

import (
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"text/tabwriter"
	"time"

	flags "github.com/jessevdk/go-flags"
)

const sysctlBase = "/proc/sys"

// Command is the command for the runner
type Command struct {
	Run cmdRun `command:"run" description:"Run a command"`
}

// The current input command
var currentCmd Command
var parser = flags.NewParser(&currentCmd, flags.Default)

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	_, err := parser.Parse()
	if err != nil {
		os.Exit(1)
	}
}

func tabWriterGeneric(w io.Writer) *tabwriter.Writer {
	return tabwriter.NewWriter(w, 5, 3, 2, ' ', 0)
}

type cmdRun struct {
	WindowName    string `short:"w" long:"window-name" description:"Window name to wait for"`
	PrepareScript string `short:"p" long:"prepare-script" description:"Script to run to prepare a run"`
	CleanupScript string `short:"r" long:"restore-script" description:"Script to run to restore after a run"`
	Iterations    string `short:"n" long:"number-iterations" description:"Number of iterations to run"`
	WindowClass   string `short:"c" long:"class-name" description:"Window class to use with xdotool instead of the the first Command"`
	Args          struct {
		Cmd []string `description:"Command to run" required:"yes"`
	} `positional-args:"yes" required:"yes"`
}

func freeCaches() error {
	// it would be nice to do this from pure Go, but then we have to become root
	// which is a hassle
	// so just use sudo for now
	for _, i := range []int{1, 2, 3} {
		cmd := exec.Command("sudo", "sysctl", "-q", "vm.drop_caches="+string(i))
		out, err := cmd.CombinedOutput()
		if err != nil {
			log.Println(string(out))
			return err
		}

		// equivalent go code that must be run as root
		// err := ioutil.WriteFile(path.Join(sysctlBase, "vm/drop_caches"), []byte(strconv.Itoa(i)), 0640)
	}
	return nil
}

// waitForWindowStateChangeWmctrl waits for a window to appear or disappear using
// wmctrl
// func waitForWindowStateChangeWmctrl(name string, appearing bool) error {
// 	for {
// 		out, err := exec.Command("wmctrl", "-l").CombinedOutput()
// 		if err != nil {
// 			return err
// 		}
// 		appears := strings.Contains(string(out), name)
// 		if appears == appearing {
// 			return nil
// 		}
// 	}
// }

func wmctrlCloseWindow(name string) error {
	out, err := exec.Command("wmctrl", "-c", name).CombinedOutput()
	if err != nil {
		log.Println(string(out))
		return err
	}
	return nil
}

func (x *cmdRun) Execute(args []string) error {
	// run the prepare script if it's available
	if x.PrepareScript != "" {
		out, err := exec.Command(x.PrepareScript).CombinedOutput()
		if err != nil {
			log.Println(string(out))
			log.Printf("failed to run prepare script (%s): %v", x.PrepareScript, err)
		}
	}

	xtool := makeXDoTool()
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
	fw, err := os.OpenFile(straceLog, os.O_RDWR, 0640)
	if err != nil {
		return err
	}
	defer fw.Close()

	// read strace data from fifo async
	var slg *ExecveTiming
	var straceErr error
	doneCh := make(chan bool, 1)
	go func() {
		// FIXME: make this configurable?
		nSlowest := 1000
		slg, straceErr = TraceExecveTimings(straceLog, nSlowest)
		close(doneCh)
	}()

	cmd, err := TraceExecCommand(straceLog, x.Args.Cmd...)
	if err != nil {
		return err
	}
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	// before running the final command, free the caches to get most accurate
	// timing
	err = freeCaches()
	if err != nil {
		return err
	}

	start := time.Now()

	// start the command running
	err = cmd.Start()

	// now wait until the window appears
	// err = waitForWindowStateChangeWmctrl(x.WindowName, true)
	tryXToolClose := true
	tryWmctrl := false

	windowspec := window{}
	// check which opts are defined
	if x.WindowClass != "" {
		// prefer window class from option
		windowspec.class = x.WindowClass
	} else if x.WindowName != "" {
		// then window name
		windowspec.name = x.WindowName
	} else {
		// finally fall back to base cmd as the class
		windowspec.class = filepath.Base(x.Args.Cmd[0])
	}

	wids, err := xtool.waitForWindow(windowspec)
	if err != nil {
		log.Println("error waiting for window appearance:", err)
		// if we don't get the wid properly then we can't try closing
		tryXToolClose = false
	}

	// save the startup time
	startup := time.Since(start)

	// now get the pids before closing the window so we can gracefully try
	// closing the windows before forcibly killing them later
	if tryXToolClose {
		pids := make([]int, len(wids))
		for i, wid := range wids {
			pid, err := xtool.pidForWindowID(wid)
			if err != nil {
				log.Println("error getting pid for wid", wid, ":", err)
				tryWmctrl = true
				break
			}
			pids[i] = pid
		}

		// close the windows
		for _, wid := range wids {
			err = xtool.closeWindowID(wid)
			if err != nil {
				log.Println("error closing window", err)
				tryWmctrl = true
			}
		}

		// kill the app pids in case x fails to close the window
		for _, pid := range pids {
			// FindProcess always succeeds on unix
			proc, _ := os.FindProcess(pid)
			if err := proc.Signal(os.Kill); err != nil {
				// if the process already exited then don't fail and try wmctrl
				if !strings.Contains(err.Error(), "process already finished") {
					log.Printf("failed to kill window process %d: %v\n", pid, err)
					tryWmctrl = true
				}
			}
		}
	} else {
		log.Println("xdotool failed to get window id so try using wmctrl")
	}

	if tryWmctrl {
		err = wmctrlCloseWindow(x.WindowName)
		if err != nil {
			log.Println("failed trying to close window with wmctrl:", err)
		}
	}

	// finally kill the process we started as sudo to _really_ kill it
	// stracePid := strconv.Itoa(cmd.Process.Pid)
	// out, err := exec.Command("sudo", "kill", "-9", stracePid).CombinedOutput()
	// if err != nil {
	// 	log.Println(out)
	// 	log.Println("error killing initial strace process:", err)
	// }

	// ensure we close the fifo here so that the strace.TraceExecCommand()
	// helper gets a EOF from the fifo (i.e. all writers must be closed
	// for this)
	fw.Close()

	// wait for strace reader
	<-doneCh
	if straceErr == nil {
		// make a new tabwriter to stderr
		w := tabWriterGeneric(os.Stderr)
		slg.Display(w)
	} else {
		log.Printf("cannot extract runtime data: %v\n", straceErr)
	}

	fmt.Println("Total startup time:", startup)

	if x.CleanupScript != "" {
		out, err := exec.Command(x.CleanupScript).CombinedOutput()
		if err != nil {
			log.Println(string(out))
			log.Printf("failed to run cleanup script (%s): %v", x.PrepareScript, err)
		}
	}

	return err
}
