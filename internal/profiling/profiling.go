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

package profiling

import (
	"log"
	"os"
	"os/exec"
	"path/filepath"
)

// FreeCaches will drop caches in the kernel for the most accurate measurements
func FreeCaches() error {
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
		// err := ioutil.WriteFile(path.Join("/proc/sys", "vm/drop_caches"), []byte(strconv.Itoa(i)), 0640)
	}
	return nil
}

// RunScript will run the specified script with args, trying both a script on
// $PATH, as well as from the current working directory for easy
// scripting/measurement from the command line without large paths as arguments
func RunScript(fname string, args []string) error {
	path, err := exec.LookPath(fname)
	if err != nil {
		// try the current directory
		cwd, err := os.Getwd()
		if err != nil {
			return err
		}
		path = filepath.Join(cwd, fname)
	}
	// path is either the path found with LookPath, or cwd/fname
	out, err := exec.Command(path, args...).CombinedOutput()
	if err != nil {
		log.Println(string(out))
		log.Printf("failed to run prepare script (%s): %v", fname, err)
	}
	return err
}
