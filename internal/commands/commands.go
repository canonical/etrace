/*
 * Copyright (C) 2020-2021 Canonical Ltd
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

var (
	userCurrent     = user.Current
	userInitialized bool
	current         *user.User
)

// AddSudoIfNeeded will prefix the given exec.Cmd with sudo if the current user
// is not root.
func AddSudoIfNeeded(cmd *exec.Cmd, sudoArgs ...string) error {
	if !userInitialized {
		var err error
		current, err = userCurrent()
		if err != nil {
			return err
		}
		userInitialized = true
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

		cmd.Path = sudoPath
	}
	return nil
}
