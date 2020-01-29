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

package commands

import (
	"fmt"
	"os/exec"
	"os/user"
)

var userCurrent = user.Current

// AddSudoIfNeeded will prefix the given exec.Cmd with sudo if the current user
// is not root.
func AddSudoIfNeeded(cmd *exec.Cmd, sudoArgs ...string) error {
	current, err := userCurrent()
	if err != nil {
		return err
	}
	if current.Uid != "0" {
		sudoPath, err := exec.LookPath("sudo")
		if err != nil {
			return fmt.Errorf("cannot use strace without running as root or without sudo: %s", err)
		}

		// prepend the command with sudo and any sudo args
		cmd.Args = append(
			append([]string{sudoPath}, sudoArgs...),
			cmd.Args...,
		)
	}
	return nil
}

// MockUID is only used for tests. We need to mock the uid for
// consistent tests in other packages.
func MockUID(uid string) (restore func()) {
	old := userCurrent
	userCurrent = func() (*user.User, error) {
		return &user.User{
			Uid: uid,
		}, nil
	}
	return func() {
		userCurrent = old
	}
}
