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

package strace

// type childPidStart struct {
// 	start float64
// 	pid   string
// }

// type pidChildTracker struct {
// 	pidToChildrenPIDs map[string][]childPidStart
// }

// func newPidChildTracker() *pidChildTracker {
// 	return &pidChildTracker{
// 		pidToChildrenPIDs: make(map[string][]childPidStart),
// 	}
// }

// func (pct *pidChildTracker) Add(pid string, child string, start float64) {
// 	if _, ok := pct.pidToChildrenPIDs[pid]; !ok {
// 		pct.pidToChildrenPIDs[pid] = []childPidStart{}
// 	}
// 	pct.pidToChildrenPIDs[pid] = append(pct.pidToChildrenPIDs[pid], childPidStart{start: start, pid: child})
// }

type exeStart struct {
	start float64
	exe   string
}

type pidTracker struct {
	pidToExeStart map[string]exeStart
}

func newpidTracker() *pidTracker {
	return &pidTracker{
		pidToExeStart: make(map[string]exeStart),
	}
}

func (pt *pidTracker) getPid(pid string) (startTime float64, exe string) {
	if exeStart, ok := pt.pidToExeStart[pid]; ok {
		return exeStart.start, exeStart.exe
	}
	return 0, ""
}

func (pt *pidTracker) addPid(pid string, startTime float64, exe string) {
	pt.pidToExeStart[pid] = exeStart{start: startTime, exe: exe}
}

func (pt *pidTracker) deletePid(pid string) {
	delete(pt.pidToExeStart, pid)
}
