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
	"os"
	"path/filepath"
	"testing"

	. "gopkg.in/check.v1"
)

func Test(t *testing.T) { TestingT(t) }

type snapsTestSuite struct{}

var _ = Suite(&snapsTestSuite{})

func (s *snapsTestSuite) SetUpTest(c *C) {}

func (s *snapsTestSuite) TestRevision(c *C) {
	tmpDir := c.MkDir()
	r := MockSnapRoot(tmpDir)
	defer r()
	tt := []struct {
		snap               string
		nomkdir            bool
		revision           string
		expected           string
		expectedErrPattern string
	}{
		{
			snap:     "test-snap",
			revision: "294",
			expected: "294",
		},
		{
			snap:               "test-not-a-snap",
			nomkdir:            true,
			expectedErrPattern: "no such snap: test-not-a-snap",
		},
		{
			snap:               "test-not-a-snap",
			expectedErrPattern: "readlink .*: no such file or directory",
		},
	}

	for _, t := range tt {
		snapDir := filepath.Join(tmpDir, t.snap)
		currentSymlink := filepath.Join(snapDir, "current")
		// mock the snap dir
		if t.snap != "" && !t.nomkdir {
			c.Assert(os.MkdirAll(snapDir, 0777), IsNil)
		}

		// mock the symlink
		if t.revision != "" {
			// make the symlink
			c.Assert(os.Symlink(t.revision, currentSymlink), IsNil)
		}

		rev, err := Revision(t.snap)
		if t.expectedErrPattern != "" {
			c.Assert(err, ErrorMatches, t.expectedErrPattern)
		} else {
			c.Assert(err, IsNil)
			c.Assert(rev, Equals, t.expected)
		}

		// un-mock the symlink
		if t.revision != "" {
			c.Assert(os.Remove(currentSymlink), IsNil)
		}
	}
}
