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
	"bufio"
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

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

// Connection represents an interface connection between two snaps.
type Connection struct {
	Plug      string
	PlugSnap  string
	Slot      string
	SlotSnap  string
	Interface string
}

// ApplyConnection runs snap connect to make the specified connection.
func ApplyConnection(conn Connection) error {
	plug := conn.PlugSnap + ":" + conn.Plug
	slot := conn.SlotSnap + ":" + conn.Slot
	connectCmd := exec.Command("snap", "connect", plug, slot)
	err := commands.AddSudoIfNeeded(connectCmd)
	if err != nil {
		return fmt.Errorf("failed to add sudo to command: %v", err)
	}
	connectOut, err := connectCmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to apply connection from %s to %s: %v (%s)", plug, slot, err, string(connectOut))
	}
	return nil
}

// CurrentConnections returns the connections of the snap.
func CurrentConnections(snapName string) ([]Connection, error) {
	// save interface connections
	ifacesOut, err := exec.Command("snap", "connections", snapName).CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("failed to save snap connections output: %v (%s)", err, string(ifacesOut))
	}

	s := bufio.NewScanner(bytes.NewReader(ifacesOut))

	var conns []Connection

	// discard the first line as that's the column headers
	// but also if a snap has no connections then the output here will
	// be empty, so we just continue as normal here
	if s.Scan() {
		// empty output somehow
		for s.Scan() {
			// split each connection by whitespace
			fields := strings.Fields(s.Text())

			if len(fields) != 4 {
				return nil, fmt.Errorf("error saving interface state: unexpected number of rows from snap connections output")
			}

			// ignore disconnected plugs and slots which are indicated
			// by "-" in the snap connections output
			if fields[1] == "-" || fields[2] == "-" {
				continue
			}
			// the first column is the interface type which we don't care
			// about
			// the second column is the plug, which we do care about
			// the third column is the slot, which we also care about

			plugFields := strings.SplitN(fields[1], ":", 2)
			if len(plugFields) != 2 {
				return nil, fmt.Errorf("unexpected snap connections output format, expected plug to have \":\" in it")
			}
			if plugFields[0] == "" {
				plugFields[0] = "system"
			}
			slotFields := strings.SplitN(fields[2], ":", 2)
			if len(slotFields) != 2 {
				return nil, fmt.Errorf("unexpected snap connections output format, expected plug to have \":\" in it")
			}
			if slotFields[0] == "" {
				slotFields[0] = "system"
			}
			conn := Connection{
				Interface: fields[0],
				PlugSnap:  plugFields[0],
				Plug:      plugFields[1],
				SlotSnap:  slotFields[0],
				Slot:      slotFields[1],
			}
			conns = append(conns, conn)
		}
	}

	return conns, nil
}
