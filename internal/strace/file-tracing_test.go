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
package strace_test

import (
	"testing"

	. "gopkg.in/check.v1"

	"github.com/anonymouse64/etrace/internal/strace"
)

func Test(t *testing.T) { TestingT(t) }

type regexpMatchSuite struct{}

var _ = Suite(&regexpMatchSuite{})

type regexSyscallTestCase struct {
	line       string
	expmatches []string
	comment    string
}

func (p *regexpMatchSuite) TestFdAndPathRE(c *C) {
	tt := []regexSyscallTestCase{
		{
			`122166 1574886795.484115 newfstatat(3</proc/122166/fd>, "9", {st_mode=S_IFREG|0644, st_size=1377694, ...}, 0) = 0`,
			[]string{
				"122166",
				"1574886795.484115",
				"newfstatat",
				"/proc/122166/fd",
				"9",
			},
			"second arg number",
		},
		{
			`121041 1574886786.247289 openat(9</snap/chromium/958>, "data-dir", O_RDONLY|O_NOFOLLOW|O_CLOEXEC|O_DIRECTORY) = 10</snap/chromium/958/data-dir>`,
			[]string{
				"121041",
				"1574886786.247289",
				"openat",
				"/snap/chromium/958",
				"data-dir",
			},
			"second arg name",
		},
		{
			`121041 1574886786.247289 openat(9</snap/chromium/958>, "data-dir/some-sub-dir", O_RDONLY|O_NOFOLLOW|O_CLOEXEC|O_DIRECTORY) = 10</snap/chromium/958/data-dir>`,
			[]string{
				"121041",
				"1574886786.247289",
				"openat",
				"/snap/chromium/958",
				"data-dir/some-sub-dir",
			},
			"second arg path with sub-dir",
		},
	}

	for _, t := range tt {
		matches := strace.FdAndPathRE.FindStringSubmatch(t.line)
		c.Check(
			matches,
			DeepEquals,
			// the first argument should be the whole line itself
			append([]string{t.line}, t.expmatches...),
			Commentf(t.comment),
		)
	}
}

func (p *regexpMatchSuite) TestAbsPathWithCWDRE(c *C) {
	tt := []regexSyscallTestCase{
		{
			`121188 1574886788.027891 openat(AT_FDCWD, "/snap/chromium/current/usr/lib/locale/en_US.UTF-8/LC_COLLATE", O_RDONLY|O_CLOEXEC) = 4</some/where>`,
			[]string{
				"121188",
				"1574886788.027891",
				"openat",
				"/snap/chromium/current/usr/lib/locale/en_US.UTF-8/LC_COLLATE",
			},
			"syscall with returned fd path testcase 1",
		},
		{
			`121188 1574886788.027966 openat(AT_FDCWD, "/snap/chromium/958/usr/lib/locale/en_US.utf8/LC_COLLATE", O_RDONLY|O_CLOEXEC) = 3</snap/chromium/958/usr/lib/locale/aa_DJ.utf8/LC_COLLATE>`,
			[]string{
				"121188",
				"1574886788.027966",
				"openat",
				"/snap/chromium/958/usr/lib/locale/en_US.utf8/LC_COLLATE",
			},
			"syscall with returned fd path testcase 2",
		},
		{
			`120994 1574886785.937456 readlinkat(AT_FDCWD, "/snap/chromium/current", ""..., 128) = 3`,
			[]string{
				"120994",
				"1574886785.937456",
				"readlinkat",
				"/snap/chromium/current",
			},
			"syscall without returned fd path",
		},
	}

	for _, t := range tt {
		matches := strace.AbsPathWithCWDRE.FindStringSubmatch(t.line)
		c.Check(
			matches,
			DeepEquals,
			// the first argument should be the whole line itself
			append([]string{t.line}, t.expmatches...),
			Commentf(t.comment),
		)
	}
}

func (p *regexpMatchSuite) TestAbsPathRE(c *C) {
	tt := []regexSyscallTestCase{
		{
			`25251 1588799883.286400 newfstatat(-1, "/sys/kernel/security/apparmor/features", 0x7ffe17b21970, 0) = 0`,
			[]string{
				"25251",
				"1588799883.286400",
				"newfstatat",
				"/sys/kernel/security/apparmor/features",
			},
			"newfstatat syscall",
		},
		{
			`26004 1588121137.500643 recvfrom(7<socket:[624422]>, ""..., 2048, 0, {sa_family=AF_INET, sin_port=htons(53), sin_addr=inet_addr("127.0.0.53")}, [28->16]) = 84`,
			[]string{},
			"not matching recvfrom",
		},
	}

	for _, t := range tt {
		matches := strace.AbsPathRE.FindStringSubmatch(t.line)
		// the first argument should be the whole line itself for positive
		// matches
		var exp []string
		if len(t.expmatches) != 0 {
			exp = append([]string{t.line}, t.expmatches...)
		}
		c.Check(
			matches,
			DeepEquals,
			exp,
			Commentf(t.comment),
		)
	}
}

func (p *regexpMatchSuite) TestAbsPathFirstRE(c *C) {
	tt := []regexSyscallTestCase{
		// TODO: fix this case
		// {
		// 	`121372 1574886788.833540 symlinkat("/snap/chromium/958/usr/lib/x86_64-linux-gnu/gtk-3.0/3.0.0/immodules/im-am-et.so", AT_FDCWD, "/home/ijohnson/snap/chromium/common/.cache/immodules/im-am-et.so") = 0`,
		// 	[]string{
		// 		"121372",
		// 		"1574886788.833540",
		// 		"symlinkat",
		// 		"AT_FDCWD",
		// 		"/home/ijohnson/snap/chromium/common/.cache/immodules/im-am-et.so",
		// 	},
		// 	"symlinkat",
		// },
		{
			`121185 1574886787.979943 execve("/snap/chromium/958/usr/sbin/update-icon-caches", [...], 0x561bce4ee880 /* 105 vars */) = 0`,
			[]string{
				"121185",
				"1574886787.979943",
				"execve",
				"/snap/chromium/958/usr/sbin/update-icon-caches",
			},
			"execve syscall",
		},
		{
			`120990 1574886792.229066 readlink("/snap/chromium/958/etc/fonts/conf.d/65-nonlatin.conf", ""..., 4095) = 30`,
			[]string{
				"120990",
				"1574886792.229066",
				"readlink",
				"/snap/chromium/958/etc/fonts/conf.d/65-nonlatin.conf",
			},
			"readlink syscall",
		},
		{
			`15546 1588797314.955495 readlink("/proc/self/fd/3", ""..., 4096) = 25`,
			[]string{
				"15546",
				"1588797314.955495",
				"readlink",
				"/proc/self/fd/3",
			},
			"another readlink syscall",
		},
	}

	for _, t := range tt {
		matches := strace.AbsPathFirstRE.FindStringSubmatch(t.line)
		// the first argument should be the whole line itself for positive
		// matches
		var exp []string
		if len(t.expmatches) != 0 {
			exp = append([]string{t.line}, t.expmatches...)
		}
		c.Check(
			matches,
			DeepEquals,
			exp,
			Commentf(t.comment),
		)
	}
}

func (p *regexpMatchSuite) TestfdRE(c *C) {
	tt := []regexSyscallTestCase{
		{
			`121041 1574886786.247289 openat(9</snap/chromium/958>, "data-dir", O_RDONLY|O_NOFOLLOW|O_CLOEXEC|O_DIRECTORY) = 10</snap/chromium/958/data-dir>`,
			[]string{
				"121041",
				"1574886786.247289",
				"openat",
				"/snap/chromium/958",
			},
			"",
		},
		{
			`121188 1574886788.028095 close(3</snap/chromium/958/usr/lib/locale/aa_DJ.utf8/LC_COLLATE>) = 0`,
			[]string{
				"121188",
				"1574886788.028095",
				"close",
				"/snap/chromium/958/usr/lib/locale/aa_DJ.utf8/LC_COLLATE",
			},
			"",
		},
		{
			`121188 1574886788.028052 mmap(NULL, 1244054, PROT_READ, MAP_PRIVATE, 3</snap/chromium/958/usr/lib/locale/aa_DJ.utf8/LC_COLLATE>, 0) = 0x7f8d780a7000`,
			[]string{
				"121188",
				"1574886788.028052",
				"mmap",
				"/snap/chromium/958/usr/lib/locale/aa_DJ.utf8/LC_COLLATE",
			},
			"",
		},
		{
			`120990 1574886796.125850 lseek(156</snap/chromium/958/data-dir/icons/Yaru/cursors/text>, 6144, SEEK_SET) = 6144`,
			[]string{
				"120990",
				"1574886796.125850",
				"lseek",
				"/snap/chromium/958/data-dir/icons/Yaru/cursors/text",
			},
			"",
		},
		{
			`120990 1574886796.126170 read(156</snap/chromium/958/data-dir/icons/Yaru/cursors/text>, ""..., 1024) = 1024`,
			[]string{
				"120990",
				"1574886796.126170",
				"read",
				"/snap/chromium/958/data-dir/icons/Yaru/cursors/text",
			},
			"read",
		},
		{
			`20721 1592353878.163963 ftruncate(26</tmp/.glDNftWu (deleted)>, 8192) = 0`,
			[]string{
				"20721",
				"1592353878.163963",
				"ftruncate",
				"/tmp/.glDNftWu (deleted)",
			},
			"ftruncate with deleted fd",
		},
		// negative cases we expect not to match
		{
			`27652 1587946984.879501 write(9<pipe:[200089]>, ""..., 4) = 4`,
			[]string{},
			"",
		},
		{
			`25251 1588799883.286429 openat(-1, "/sys/kernel/security/apparmor/features", O_RDONLY|O_CLOEXEC|O_DIRECTORY) = 3</sys/kernel/security/apparmor/features>`,
			[]string{},
			"openat is handled by absPathRE",
		},
	}

	for _, t := range tt {
		matches := strace.FdRE.FindStringSubmatch(t.line)
		// the first argument should be the whole line itself for positive
		// matches
		var exp []string
		if len(t.expmatches) != 0 {
			exp = append([]string{t.line}, t.expmatches...)
		}
		c.Check(matches, DeepEquals, exp, Commentf(t.comment))
	}
}
