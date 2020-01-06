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
package profiling_test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/anonymouse64/etrace/internal/files"
	"github.com/anonymouse64/etrace/internal/profiling"
	"gopkg.in/check.v1"
)

func Test(t *testing.T) { check.TestingT(t) }

type profilingTestSuite struct {
	tmpDir string
	script string
}

const (
	testScriptName = "test-script-cwd.sh"
)

func MockCWD(c *check.C, new string) func() {
	old, err := os.Getwd()
	c.Assert(err, check.IsNil)
	err = os.Chdir(new)
	c.Assert(err, check.IsNil)
	return func() {
		c.Assert(os.Chdir(old), check.IsNil)
	}
}

var _ = check.Suite(&profilingTestSuite{})

func (p *profilingTestSuite) SetUpTest(c *check.C) {
	// put a test script in a tmp dir
	p.tmpDir = c.MkDir()
	p.script = filepath.Join(p.tmpDir, testScriptName)
	f, err := files.EnsureExistsAndOpen(p.script, true)
	c.Assert(err, check.IsNil)
	c.Assert(f, check.Not(check.IsNil))
	// the file just needs to exist, so we can close it
	f.Close()

	// make the file executable
	os.Chmod(p.script, os.FileMode(755))
}

func (p *profilingTestSuite) TestRunScriptFromPathEnv(c *check.C) {
	// add the tmpdir to path for this test
	oldPath := os.Getenv("PATH")
	os.Setenv("PATH", fmt.Sprintf("%s:%s", p.tmpDir, oldPath))
	defer func() {
		os.Setenv("PATH", oldPath)
	}()

	r := profiling.MockExecCommand(func(exec string, args ...string) ([]byte, error) {
		c.Assert(exec, check.Equals, p.script)
		c.Assert(args, check.DeepEquals, []string{"arg1", "arg2"})
		return nil, nil
	})
	defer r()

	err := profiling.RunScript(testScriptName, []string{"arg1", "arg2"})
	c.Assert(err, check.IsNil)
}

func (p *profilingTestSuite) TestRunScriptFromCWD(c *check.C) {
	// set cwd to the tmpdir
	r := MockCWD(c, p.tmpDir)
	defer r()

	r = profiling.MockExecCommand(func(exec string, args ...string) ([]byte, error) {
		c.Assert(exec, check.Equals, p.script)
		c.Assert(args, check.DeepEquals, []string{"arg1", "arg2"})
		return nil, nil
	})
	defer r()

	err := profiling.RunScript(testScriptName, []string{"arg1", "arg2"})
	c.Assert(err, check.IsNil)
}

func (p *profilingTestSuite) TestRunScriptInvalid(c *check.C) {
	err := profiling.RunScript(testScriptName, []string{"arg1", "arg2"})
	c.Assert(err, check.ErrorMatches, ".*no such file or directory")
}

func (p *profilingTestSuite) TestFreeCachesSudoNotFound(c *check.C) {
	// unset path so that sudo is not found
	oldPath := os.Getenv("PATH")
	os.Setenv("PATH", "")
	defer func() {
		os.Setenv("PATH", oldPath)
	}()

	err := profiling.FreeCaches()
	c.Assert(err, check.ErrorMatches, `exec: "sudo": executable file not found in \$PATH`)
}

func (p *profilingTestSuite) TestFreeCaches(c *check.C) {
	runs := 0
	r := profiling.MockExecCommand(func(exec string, args ...string) ([]byte, error) {
		c.Assert(exec, check.Equals, "sudo")
		switch runs {
		case 0:
			c.Assert(args, check.DeepEquals, []string{"sysctl", "-q", "vm.drop_caches=\x01"})
		case 1:
			c.Assert(args, check.DeepEquals, []string{"sysctl", "-q", "vm.drop_caches=\x02"})
		case 2:
			c.Assert(args, check.DeepEquals, []string{"sysctl", "-q", "vm.drop_caches=\x03"})
		default:
			c.Fatalf(
				"unexpected exec call of %v (on %d calls)",
				append([]string{exec}, args...),
				runs,
			)
		}

		runs++
		return nil, nil
	})
	defer r()

	err := profiling.FreeCaches()
	c.Assert(err, check.IsNil)
}
