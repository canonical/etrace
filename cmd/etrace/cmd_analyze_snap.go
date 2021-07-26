/*
 * Copyright (C) 2021 Canonical Ltd
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
	"fmt"
	"io/ioutil"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/anonymouse64/etrace/internal/commands"
	"github.com/anonymouse64/etrace/internal/snaps"

	// TODO: eliminate this dependency
	"github.com/snapcore/snapd/gadget/quantity"
)

type cmdAnalyzeSnap struct {
	InstallChannel string `long:"channel" description:"Channel to install the snap from if not already installed"`
	Args           struct {
		Snap string `description:"Snap to analyze" required:"yes"`
	} `positional-args:"yes" required:"yes"`
}

func (x *cmdAnalyzeSnap) Execute(args []string) error {

	snapName := x.Args.Snap

	// analyze looks at a few aspects of the snap:
	// 1. The size of the snap as-is from installed
	// 2. What compression format the snap is using
	// 3. What content interface dependency snaps does this snap have
	// 4. Worst case performance launch data
	// 5. Best case performance launch data
	// 6. (optional) if XZ compressed, worst case performance launch if it was switched to LZO
	// 7. (optional) if XZ compressed, best case performance launch if it was switched to LZO
	// 8. (optional) if XZ compressed, file size increase if it was switched to LZO

	tmpWorkDir, err := ioutil.TempDir("", fmt.Sprintf("etrace-analyze-%s", snapName))
	if err != nil {
		return err
	}

	// first ensure the snap is installed, if it is not then download it
	if !snaps.IsInstalled(snapName) {
		// then install it
		if err := exec.Command("snap", "install", snapName, "--channel="+x.InstallChannel).Run(); err != nil {
			return fmt.Errorf("unable to install snap %s and analyze: %w", snapName, err)
		}

	}

	// now make a copy of what is currently installed as the original version to
	// analyze and compare with possibly alternative compression formats
	rev, err := snaps.Revision(snapName)
	if err != nil {
		return err
	}

	originalSnapFile := filepath.Join(tmpWorkDir, snapName+".snap")
	// TODO: need to use cp manually here
	cpCmd := exec.Command("cp", filepath.Join("/var/lib/snapd/snaps/", snapName+"_"+rev+".snap"), originalSnapFile)
	commands.AddSudoIfNeeded(cpCmd)
	if err := cpCmd.Run(); err != nil {
		return err
	}

	// 1. get the original size
	st, err := os.Stat(originalSnapFile)
	if err != nil {
		// very unexpected as we just copied the file
		return err
	}
	origSz := quantity.Size(st.Size())
	fmt.Printf("original snap size: %s\n", origSz.IECString())

	// 2. get what compression format this snap is using
	unsquashfsCmd := exec.Command("unsquashfs", "-s", originalSnapFile)
	if err := commands.AddSudoIfNeeded(unsquashfsCmd); err != nil {
		return err
	}

	unsquashfsOut, err := unsquashfsCmd.CombinedOutput()
	if err != nil {
		return err
	}

	// look for the compression format line
	s := bufio.NewScanner(bytes.NewReader(unsquashfsOut))
	compressionLineRegexp := regexp.MustCompile(`^Compression ([a-zA-Z0-9]+)$`)
	compressionFormat := ""
	for s.Scan() {
		line := s.Text()
		matches := compressionLineRegexp.FindStringSubmatch(line)
		if len(matches) == 0 {
			continue
		}
		compressionFormat = matches[1]
		break
	}

	if compressionFormat == "" {
		// TODO: what about test snaps with actually no compression in the squashfs?
		return fmt.Errorf("error: snap has no compression or unsquashfs output is corrupted")
	}

	fmt.Printf("original compression format is %s\n", compressionFormat)

	// 3. get what content interface dependency snaps this snap has by looking
	// at the slots for all connections, excluding system snap provided slots
	// and slots this snap provides
	conns, err := snaps.CurrentConnections(snapName)
	if err != nil {
		return err
	}
	contentInterfaceDependencySnapsMap := map[string]bool{}
	for _, conn := range conns {
		switch conn.SlotSnap {
		case "system", snapName:
			continue
		default:
			contentInterfaceDependencySnapsMap[conn.SlotSnap] = true
		}
	}

	contentInterfaceDependencySnaps := make([]string, 0, len(contentInterfaceDependencySnapsMap))
	for snap := range contentInterfaceDependencySnapsMap {
		contentInterfaceDependencySnaps = append(contentInterfaceDependencySnaps, snap)
	}

	fmt.Printf("content snap slot dependencies: %+v\n", contentInterfaceDependencySnaps)

	// 4. Get the worst case performance data using etrace
	meanWorst, stdDevWorst, err := performanceData("--cold", snapName)
	if err != nil {
		return err
	}

	fmt.Printf("worst case performance:\n")
	fmt.Printf("\taverage time to display: %s\n", meanWorst)
	fmt.Printf("\tstandard deviation for time to display: %s\n", stdDevWorst)

	// 5. Get the best case performance data using etrace
	meanBest, stdDevBest, err := performanceData("--hot", snapName)
	if err != nil {
		return err
	}

	fmt.Printf("best case performance:\n")
	fmt.Printf("\taverage time to display: %s\n", meanBest)
	fmt.Printf("\tstandard deviation for time to display: %s\n", stdDevBest)

	if strings.ToLower(compressionFormat) != "xz" {
		// nothing left to check
		return nil
	}

	// otherwise it was xz, we should check LZO

	// first unpack the snap and repack it with LZO
	lzoSnapFile := filepath.Join(tmpWorkDir, snapName+"_lzo.snap")
	unpackDir := filepath.Join(tmpWorkDir, "unpacked-snap")
	unsquashfsCmd = exec.Command("unsquashfs", "-d", unpackDir, originalSnapFile)
	commands.AddSudoIfNeeded(unsquashfsCmd)
	if err := unsquashfsCmd.Run(); err != nil {
		return err
	}

	// now re-pack as lzo
	packCmd := exec.Command("snap",
		"pack",
		"--filename="+lzoSnapFile,
		"--compression=lzo",
		unpackDir,
	)
	commands.AddSudoIfNeeded(packCmd)
	if err := packCmd.Run(); err != nil {
		return err
	}

	// now install the lzo version
	// TODO: handle devmode or classic snap options, etc. with the logic from
	// exec cmd
	installCmd := exec.Command("snap", "install", "--dangerous", lzoSnapFile)
	commands.AddSudoIfNeeded(installCmd)
	if err := installCmd.Run(); err != nil {
		return err
	}

	// defer a revert command to the original revision we had installed
	defer func() {
		revertCmd := exec.Command("snap", "install", originalSnapFile)
		commands.AddSudoIfNeeded(revertCmd)
		if err := revertCmd.Run(); err != nil {
			fmt.Printf("error reverting to previous version of %s\n: %v", snapName, err)
		}
	}()

	// now we should have the LZO version installed, get data for this one

	// 6. Get the worst case performance data using etrace
	meanWorstLZO, stdDevWorseLZO, err := performanceData("--cold", snapName)
	if err != nil {
		return err
	}

	fmt.Printf("worst case performance with LZO compression:\n")
	fmt.Printf("\taverage time to display: %s\n", meanWorstLZO)
	fmt.Printf("\tstandard deviation for time to display: %s\n", stdDevWorseLZO)
	fmt.Printf("\taverage time to display percent change: %s\n", percentDiffDuration(meanWorst, meanWorstLZO))

	// 7. Get the best case performance data using etrace
	meanBestLZO, stdDevBestLZO, err := performanceData("--hot", snapName)
	if err != nil {
		return err
	}

	fmt.Printf("best case performance with LZO compression:\n")
	fmt.Printf("\taverage time to display: %s\n", meanBestLZO)
	fmt.Printf("\tstandard deviation for time to display: %s\n", stdDevBestLZO)
	fmt.Printf("\taverage time to display percent change: %s\n", percentDiffDuration(meanBest, meanBestLZO))

	// 8. Calculate the percent change in filesize between LZO and XZ
	st, err = os.Stat(lzoSnapFile)
	if err != nil {
		return err
	}
	lzoSz := quantity.Size(st.Size())
	fmt.Printf("lzo snap size: %s (change of %s)\n", lzoSz.IECString(), percentDiffSz(origSz, lzoSz))

	return nil
}

func percentDiffDuration(d1, d2 time.Duration) string {
	sign := ""
	if d1 < d2 {
		sign = "+"
	} else {
		sign = "-"
	}
	return fmt.Sprintf("%s%.2f%%", sign, math.Abs(100*float64(d2-d1)/float64(d1)))
}

func percentDiffSz(sz1, sz2 quantity.Size) string {
	sign := ""
	if sz1 < sz2 {
		sign = "+"
	} else {
		sign = "-"
	}
	return fmt.Sprintf("%s%.2f%%", sign, math.Abs(100*float64(sz2-sz1)/float64(sz1)))
}

func meanAndStdDevForRuns(runs ExecOutputResult) (time.Duration, time.Duration, error) {
	// analyze the TimeToDisplay field for all the runs
	count := float64(len(runs.Runs))
	var mean float64
	for _, run := range runs.Runs {
		if run.TimeToDisplay == 0 {
			// this is unexpected
			return 0, 0, fmt.Errorf("error: run produced time of exactly 0")
		}

		mean += float64(run.TimeToDisplay)
	}
	mean = mean / count

	sumDiffSq := float64(0)
	for _, run := range runs.Runs {
		diff := float64(run.TimeToDisplay) - mean
		sumDiffSq += (diff * diff)
	}
	stdDev := time.Duration(math.Sqrt(sumDiffSq / count))

	return time.Duration(mean), stdDev, nil
}

func performanceData(mode, snapName string) (man, stdDev time.Duration, err error) {
	runs := "10"
	if mode == "--hot" {
		runs = "11"
	}

	// TODO: just call the right functions from this same process, this is a bit
	// unfortunate to call ourself externally like this
	cmd := exec.Command("etrace",
		"exec",
		"--json",                 // we want machine readable output
		"--repeat="+runs,         // we want statistically significant results
		"--use-snap-run",         // we are running a snap
		mode,                     // for whatever mode was specified
		"--cmd-stderr=/dev/null", // we don't want any stderr output
		"--cmd-stdout=/dev/null", // we don't want any stdout output
		"--no-trace",             // we don't want to trace for best performance
		snapName,
	)

	out, err := cmd.CombinedOutput()
	if err != nil {
		return 0, 0, err
	}

	// parse the output as json
	var execOutputJSON ExecOutputResult
	if err := json.Unmarshal(out, &execOutputJSON); err != nil {
		return 0, 0, err
	}

	if mode == "--hot" {
		// discard the first run as it may have been a "cold" one
		execOutputJSON.Runs = execOutputJSON.Runs[1:]
	}

	return meanAndStdDevForRuns(execOutputJSON)
}
