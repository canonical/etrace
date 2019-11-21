package main

import (
	"encoding/json"
	"errors"
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
	Run        cmdRun `command:"run" description:"Run a command"`
	ShowErrors bool   `short:"e" long:"errors" description:"Show errors as they happen"`
}

// OutputResult is the result of running a command with various information
// encoded in it
type OutputResult struct {
	Runs []Execution
}

type Execution struct {
	ExecveTiming  *ExecveTiming
	TimeToDisplay time.Duration
	TimeToRun     time.Duration
	Errors        []error
}

type cmdRun struct {
	WindowName       string `short:"w" long:"window-name" description:"Window name to wait for"`
	PrepareScript    string `short:"p" long:"prepare-script" description:"Script to run to prepare a run"`
	RestoreScript    string `short:"r" long:"restore-script" description:"Script to run to restore after a run"`
	Iterations       string `short:"n" long:"number-iterations" description:"Number of iterations to run"`
	WindowClass      string `short:"c" long:"class-name" description:"Window class to use with xdotool instead of the the first Command"`
	NoTrace          bool   `short:"t" long:"no-trace" description:"Don't trace the process, just time the total execution"`
	RunThroughSnap   bool   `short:"s" long:"use-snap-run" description:"Run command through snap run"`
	DiscardSnapNs    bool   `short:"d" long:"discard-snap-ns" description:"Discard the snap namespace before running the snap"`
	ProgramStdoutLog string `long:"cmd-stdout" description:"Log file for run command's stdout"`
	ProgramStderrLog string `long:"cmd-stderr" description:"Log file for run command's stderr"`
	JSONOutput       bool   `long:"json" description:"Output results in JSON"`

	Args struct {
		Cmd []string `description:"Command to run" required:"yes"`
	} `positional-args:"yes" required:"yes"`
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

// discardSnapNs runs snap-discard-ns on a snap to get an accurate startup time
// of setting up that snap's namespace
func discardSnapNs(snap string) error {
	out, err := exec.Command("sudo", "/usr/lib/snapd/snap-discard-ns", snap).CombinedOutput()
	if err != nil {
		log.Println(string(out))
	}
	return err
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

func runScript(fname string) error {
	out, err := exec.Command(fname).CombinedOutput()
	if err != nil {
		log.Println(string(out))
		log.Printf("failed to run prepare script (%s): %v", fname, err)
	}
	return err
}

func ensureFileExistsAndOpen(fname string) (*os.File, error) {
	// if the file doesn't exist, create it
	if _, err := os.Stat(fname); os.IsNotExist(err) {
		return os.Create(fname)
	}
	// otherwise just try to open it directly
	return os.Open(fname)
}

func wmctrlCloseWindow(name string) error {
	out, err := exec.Command("wmctrl", "-c", name).CombinedOutput()
	if err != nil {
		log.Println(string(out))
		return err
	}
	return nil
}

var errs []error

func logError(err error) {
	errs = append(errs, err)
	if currentCmd.ShowErrors {
		log.Println(err)
	}
}

func (x *cmdRun) Execute(args []string) error {
	// run the prepare script if it's available
	if x.PrepareScript != "" {
		err := runScript(x.PrepareScript)
		if err != nil {
			logError(fmt.Errorf("running prepare script: %w", err))
		}
	}

	// handle if the command should be run through `snap run`
	targetCmd := x.Args.Cmd
	if x.RunThroughSnap {
		targetCmd = append([]string{"snap", "run"}, targetCmd...)
	}

	doneCh := make(chan bool, 1)
	var straceErr error
	var slg *ExecveTiming
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
			slg, straceErr = TraceExecveTimings(straceLog, -1)
			close(doneCh)
		}()

		cmd, err = TraceExecCommand(straceLog, targetCmd...)
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
		f, err := ensureFileExistsAndOpen(x.ProgramStdoutLog)
		if err != nil {
			return err
		}
		defer f.Close()
		cmd.Stdout = f
	}
	if x.ProgramStderrLog != "" {
		f, err := ensureFileExistsAndOpen(x.ProgramStderrLog)
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
		err := discardSnapNs(x.Args.Cmd[0])
		if err != nil {
			return err
		}
	}

	// before running the final command, free the caches to get most accurate
	// timing
	err := freeCaches()
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
		// note we use the original command and note the processed targetCmd
		// because for example when measuring a snap, we invoke etrace like so:
		// $ ./etrace run --use-snap chromium
		// where targetCmd becomes []string{"snap","run","chromium"}
		// but we still want to use "chromium" as the windowspec class
		windowspec.class = filepath.Base(x.Args.Cmd[0])
	}

	xtool := makeXDoTool()
	wids, err := xtool.waitForWindow(windowspec)
	if err != nil {
		logError(fmt.Errorf("waiting for window appearance: %w", err))
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
				logError(fmt.Errorf("getting pid for wid %s: %w", wid, err))
				tryWmctrl = true
				break
			}
			pids[i] = pid
		}

		// close the windows
		for _, wid := range wids {
			err = xtool.closeWindowID(wid)
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
		<-doneCh
		if straceErr == nil {
			// make a new tabwriter to stderr
			if !x.JSONOutput {
				w := tabWriterGeneric(os.Stderr)
				slg.Display(w)
			}
		} else {
			logError(fmt.Errorf("cannot extract runtime data: %w", straceErr))
		}
	}

	if x.RestoreScript != "" {
		err := runScript(x.RestoreScript)
		if err != nil {
			logError(fmt.Errorf("running restore script: %w", err))
		}
	}

	out := OutputResult{
		Runs: []Execution{
			{
				ExecveTiming:  slg,
				TimeToDisplay: startup,
				TimeToRun:     slg.TotalTime,
				Errors:        errs,
			},
		},
	}

	if x.JSONOutput {
		json.NewEncoder(os.Stdout).Encode(out)
	} else {
		fmt.Println("Total startup time:", startup)
	}

	return nil
}
