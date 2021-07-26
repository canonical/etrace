/*
 * Copyright (C) 2021 Canonical Ltd
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

package main_test

import (
	"testing"
	"time"

	main "github.com/anonymouse64/etrace/cmd/etrace"

	. "gopkg.in/check.v1"
)

func Test(t *testing.T) { TestingT(t) }

type analyzeSnapTestSuite struct{}

var _ = Suite(&analyzeSnapTestSuite{})

func (p *analyzeSnapTestSuite) TestMeanAndStdDevForRuns(c *C) {
	tt := []struct {
		vals      []int64
		expMean   int64
		expStdDev int64
		expErr    string
	}{
		{
			vals: []int64{
				10000,
				20000,
				30000,
				40000,
				50000,
			},
			expMean:   30000,
			expStdDev: 14142,
		},
	}

	for _, t := range tt {
		exec := main.ExecOutputResult{}
		exec.Runs = make([]main.Execution, len(t.vals))
		for i, val := range t.vals {
			exec.Runs[i].TimeToDisplay = time.Duration(val)
		}

		mean, stdDev, err := main.MeanAndStdDevForRuns(exec)
		if t.expErr != "" {
			c.Check(err, ErrorMatches, t.expErr)
			continue
		}
		c.Assert(mean, Equals, time.Duration(t.expMean))
		c.Assert(stdDev, Equals, time.Duration(t.expStdDev))
	}

}
