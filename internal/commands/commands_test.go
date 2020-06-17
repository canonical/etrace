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

package commands_test

import (
	"io/ioutil"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"testing"

	"github.com/anonymouse64/etrace/internal/commands"
	. "gopkg.in/check.v1"
)

func Test(t *testing.T) { TestingT(t) }

type commandsTestSuite struct {
	tmpDir string
}

var _ = Suite(&commandsTestSuite{})

func (s *commandsTestSuite) SetUpTest(c *C) {
}

func (s *commandsTestSuite) TestAddSudoIfNeededCaches(c *C) {
	n := 0

	restore := commands.MockUserCurrent(func() (*user.User, error) {
		n++
		return &user.User{
			Uid: "0",
		}, nil
	})
	defer restore()

	cmd := exec.Command("hello", "world")
	err := commands.AddSudoIfNeeded(cmd)
	c.Assert(err, IsNil)

	// only called once so far
	c.Assert(n, Equals, 1)

	// not called again
	err = commands.AddSudoIfNeeded(cmd)
	c.Assert(err, IsNil)

	// only called once so far
	c.Assert(n, Equals, 1)
}

func (s *commandsTestSuite) TestAddSudoIfNeeded(c *C) {
	// set PATH to a tmp dir to mock exec.LookPath
	tmpDir := c.MkDir()
	oldPath := os.Getenv("PATH")
	os.Setenv("PATH", tmpDir)
	defer func() {
		os.Setenv("PATH", oldPath)
	}()

	sudoPath := filepath.Join(tmpDir, "sudo")

	tt := []struct {
		sudoExists         bool
		uid                string
		expectedErrPattern string
		cmd                *exec.Cmd
		expectedCmd        *exec.Cmd
		sudoArgs           []string
		comment            string
	}{
		{
			sudoExists:  true,
			uid:         "0",
			cmd:         &exec.Cmd{Args: []string{"foo"}},
			expectedCmd: &exec.Cmd{Args: []string{"foo"}},
			comment:     "running as root, sudo exists",
		},
		{
			uid:         "0",
			cmd:         &exec.Cmd{Args: []string{"foo"}},
			expectedCmd: &exec.Cmd{Args: []string{"foo"}},
			comment:     "running as root, sudo does not exist",
		},
		{
			sudoExists:  true,
			uid:         "1000",
			cmd:         &exec.Cmd{Args: []string{"foo"}},
			expectedCmd: &exec.Cmd{Path: sudoPath, Args: []string{sudoPath, "foo"}},
			comment:     "running as user, sudo exists",
		},
		{
			uid:                "1000",
			cmd:                &exec.Cmd{Args: []string{"foo"}},
			expectedErrPattern: `cannot use strace without running as root or without sudo: exec: "sudo": executable file not found in \$PATH`,
			comment:            "running as user, sudo does not exists",
		},
	}

	var restore func()

	for _, t := range tt {
		// mock sudo executable
		if t.sudoExists {
			err := ioutil.WriteFile(sudoPath, []byte{}, 0755)
			c.Assert(err, IsNil, Commentf(t.comment))
		}

		// mock the current user
		if t.uid != "" {
			restore = commands.MockUserCurrent(func() (*user.User, error) {
				return &user.User{
					Uid: t.uid,
				}, nil
			})
		}

		// do the test
		err := commands.AddSudoIfNeeded(t.cmd, t.sudoArgs...)
		if t.expectedErrPattern != "" {
			// check the error
			c.Assert(err, ErrorMatches, t.expectedErrPattern, Commentf(t.comment))
		} else {
			c.Assert(err, IsNil)
			// check the cmd
			c.Assert(t.cmd, DeepEquals, t.expectedCmd, Commentf(t.comment))
		}

		// un-mock the current user
		if t.uid != "" {
			restore()
		}

		// un-mock sudo
		if t.sudoExists {
			c.Assert(os.Remove(sudoPath), IsNil, Commentf(t.comment))
		}

		// reset the caching
		commands.ResetInitialized()
	}
}
