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

package snaps

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/anonymouse64/etrace/internal/commands"
)

var snapRoot = "/snap"

// DiscardSnapNs runs snap-discard-ns on a snap to get an accurate startup time
// of setting up that snap's namespace
func DiscardSnapNs(snap string) error {
	cmd := exec.Command("/usr/lib/snapd/snap-discard-ns", snap)
	err := commands.AddSudoIfNeeded(cmd)
	if err != nil {
		return err
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to run snap-discard-ns: %v (output: %s)", err, string(out))
	}
	return nil
}

// Revision returns the revision of the snap
func Revision(snap string) (string, error) {
	snapDir := filepath.Join(snapRoot, snap)
	// make sure the snap dir for this snap exists
	if _, err := os.Stat(snapDir); os.IsNotExist(err) {
		return "", fmt.Errorf("no such snap: %s", snap)
	}
	// get the revision by reading the "current" link in /snap/$SNAP_NAME
	return os.Readlink(filepath.Join(snapDir, "current"))
}
