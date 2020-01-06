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

package files_test

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/anonymouse64/etrace/internal/files"

	"gopkg.in/check.v1"
)

func Test(t *testing.T) { check.TestingT(t) }

type filesTestSuite struct {
}

var _ = check.Suite(&filesTestSuite{})

func (p *filesTestSuite) SetUpTest(c *check.C) {
}

func (p *filesTestSuite) TestEnsureFileIsDeleted(c *check.C) {
	tt := []struct {
		fContentBefore     string
		expectedErrPattern string
		comment            string
	}{
		{fContentBefore: "something", comment: "file exists"},
		{comment: "normal"},
	}

	dir := c.MkDir()

	for _, t := range tt {
		// do the test
		fName := "not-a-real-file"
		// create the file if it should exist before
		if t.fContentBefore != "" {
			// create a file
			f, err := ioutil.TempFile(dir, "")
			c.Assert(err, check.IsNil)
			fName = f.Name()
			_, err = f.WriteString(t.fContentBefore)
			c.Assert(err, check.IsNil)
			err = f.Close()
			c.Assert(err, check.IsNil)
		}

		err := files.EnsureFileIsDeleted(fName)
		if t.expectedErrPattern != "" {
			c.Assert(err, check.ErrorMatches, t.expectedErrPattern)
			continue
		} else {
			c.Assert(err, check.IsNil)

			// check that the file doesn't exist
			_, err = os.Stat(fName)
			c.Assert(os.IsNotExist(err), check.Equals, true, check.Commentf(t.comment))
		}
	}
}

func (p *filesTestSuite) TestEnsureExistAndOpenExists(c *check.C) {
	tt := []struct {
		fContentBefore     string
		fIsDir             bool
		fShouldDelete      bool
		fContentAfter      string
		expectedErrPattern string
	}{
		{
			fContentBefore: "something",
			fShouldDelete:  true,
		},
		{
			fContentBefore: "something",
			fShouldDelete:  true,
		},
		{
			fContentBefore: "something",
			fShouldDelete:  false,
			fContentAfter:  "something",
		},
		{
			fShouldDelete: false,
			fContentAfter: "",
		},
		{
			fShouldDelete: true,
			fContentAfter: "",
		},
		{
			fIsDir:             true,
			expectedErrPattern: "open .* is a directory",
		},
	}

	dir := c.MkDir()

	for _, t := range tt {
		// do the test
		fName := "the-file"
		// create the file if it should exist before
		switch {
		case t.fContentBefore != "":
			// create a file
			f, err := ioutil.TempFile(dir, "")
			c.Assert(err, check.IsNil)
			fName = f.Name()
			_, err = f.WriteString(t.fContentBefore)
			c.Assert(err, check.IsNil)
			err = f.Close()
			c.Assert(err, check.IsNil)
		case t.fIsDir:
			d, err := ioutil.TempDir(dir, "")
			c.Assert(err, check.IsNil)
			fName = d
		default:
			fName = filepath.Join(dir, fName)
		}

		f, err := files.EnsureExistsAndOpen(fName, t.fShouldDelete)
		if t.expectedErrPattern != "" {
			c.Assert(err, check.ErrorMatches, t.expectedErrPattern)
			continue
		} else {
			c.Assert(err, check.IsNil)

			c.Assert(f, check.Not(check.IsNil))
			// read the whole file from the start and compare with
			// the expected string
			err = f.Close()
			c.Assert(err, check.IsNil)
			fileContent, err := ioutil.ReadFile(fName)
			c.Assert(err, check.IsNil)
			c.Assert(string(fileContent), check.Equals, t.fContentAfter)
		}
	}
}
