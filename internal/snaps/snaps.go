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
	"os/exec"
	"strings"

	"github.com/anonymouse64/etrace/internal/commands"
)

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
	out, err := exec.Command("snap", "run", "--shell", snap, "-c", "echo $SNAP_REVISION").CombinedOutput()
	if err != nil {
		return "", nil
	}
	return strings.TrimSpace(string(out)), err
}
