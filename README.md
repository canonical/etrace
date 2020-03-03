# etrace
[![Actions Status](https://github.com/anonymouse64/etrace/workflows/Go/badge.svg)](https://github.com/anonymouse64/etrace/actions)

_etrace_ is a utility like strace or ltrace which uses ptrace to follow programs executed by a main program for performance and debugging analysis. It also supports limited tracing of file accesses for analyzing what files a program accesses during it's execution.

## Usage

_etrace_ has two subcommands, `exec` and `file`.

### `exec` subcommand

`exec` is used for tracing the programs that are executed with the `execve` family of syscalls. It can also have tracing turned off to just measure graphical timing information.

_etrace exec_ usage:

```
Usage:
  etrace [OPTIONS] exec [exec-OPTIONS] Cmd...

Application Options:
  -e, --errors                   Show errors as they happen
  -n, --additional-iterations=   Number of additional iterations to run (1 iteration is always run)

Help Options:
  -h, --help                     Show this help message

[exec command options]
      -w, --window-name=         Window name to wait for
      -p, --prepare-script=      Script to run to prepare a run
          --prepare-script-args= Args to provide to the prepare script
      -r, --restore-script=      Script to run to restore after a run
          --restore-script-args= Args to provide to the restore script
      -c, --class-name=          Window class to use with xdotool instead of the the first Command
      -t, --no-trace             Don't trace the process, just time the total execution
      -s, --use-snap-run         Run command through snap run
      -d, --discard-snap-ns      Discard the snap namespace before running the snap
          --cmd-stdout=          Log file for run command's stdout
          --cmd-stderr=          Log file for run command's stderr
      -j, --json                 Output results in JSON
      -o, --output-file=         A file to output the results (empty string means stdout)
          --no-window-wait       Don't wait for the window to appear, just run until the program exits

[exec command arguments]
  Cmd:                           Command to run

```

### `file` subcommand

The `file` subcommand will track all syscalls that a program executes which access files. This is useful for measuring the total set of files that a program attempts to access during its execution.

_etrace file_ usage:

```
Usage:
  etrace [OPTIONS] file [file-OPTIONS] Cmd...

Application Options:
  -e, --errors                   Show errors as they happen
  -n, --additional-iterations=   Number of additional iterations to run (1 iteration is always run)

Help Options:
  -h, --help                     Show this help message

[file command options]
      -w, --window-name=         Window name to wait for
      -p, --prepare-script=      Script to run to prepare a run
          --prepare-script-args= Args to provide to the prepare script
      -r, --restore-script=      Script to run to restore after a run
          --restore-script-args= Args to provide to the restore script
      -c, --class-name=          Window class to use with xdotool instead of the the first Command
      -s, --use-snap-run         Run command through snap run
      -d, --discard-snap-ns      Discard the snap namespace before running the snap
          --cmd-stdout=          Log file for run command's stdout
          --cmd-stderr=          Log file for run command's stderr
      -j, --json                 Output results in JSON
      -o, --output-file=         A file to output the results (empty string means stdout)
          --no-window-wait       Don't wait for the window to appear, just run until the program exits

[file command arguments]
  Cmd:                           Command to run

```


## Examples

Example output measuring the time it takes for gnome-calculator snap to display a window :

```
$ ./etrace run -s -d gnome-calculator
Gdk-Message: 19:24:54.686: gnome-calculator: Fatal IO error 11 (Resource temporarily unavailable) on X server :0.

33 exec calls during snap run:
     Start   Stop     Elapsed       Exec
     0       637604   637.604951ms  /usr/bin/snap
     10861   16162    5.300998ms    /sbin/apparmor_parser
     27100   32632    5.532026ms    /sbin/apparmor_parser
     37952   43672    5.7199ms      /snap/core/8442/usr/lib/snapd/snap-seccomp
     199319  624943   425.623178ms  snap-update-ns
     630534  635629   5.095005ms    snap-update-ns
     637604  643633   6.02889ms     /usr/lib/snapd/snap-exec
     643633  859514   215.881109ms  /snap/gnome-calculator/544/snap/command-chain/desktop-launch
     648627  651546   2.918958ms    /bin/date
     653303  658548   5.24497ms     /usr/bin/getent
     659688  664492   4.803895ms    /bin/mkdir
     665293  667890   2.596855ms    /bin/chmod
     669271  672246   2.974987ms    /usr/bin/md5sum
     673502  676505   3.002882ms    /bin/cat
     677770  680341   2.571105ms    /usr/bin/md5sum
     681612  684237   2.625942ms    /bin/cat
     685184  688700   3.516912ms    /bin/grep
     691617  703045   11.428833ms   /bin/mkdir
     703604  714510   10.905981ms   /bin/mkdir
     714978  726362   11.38401ms    /bin/mkdir
     729024  734812   5.788087ms    /snap/gnome-calculator/544/gnome-platform/usr/bin/xdg-user-dirs-update
     736660  746938   10.27894ms    /usr/bin/realpath
     748053  758188   10.13422ms    /usr/bin/realpath
     759251  768904   9.653091ms    /usr/bin/realpath
     769857  779485   9.628057ms    /usr/bin/realpath
     780469  790699   10.22911ms    /usr/bin/realpath
     791952  802161   10.209083ms   /usr/bin/realpath
     803193  813010   9.817123ms    /usr/bin/realpath
     813982  823803   9.820938ms    /usr/bin/realpath
     826356  837656   11.299133ms   /bin/mkdir
     838415  848245   9.829998ms    /bin/rm
     848959  859074   10.114908ms   /bin/ln
     859514  2063222  1.203707933s  /snap/gnome-calculator/544/usr/bin/gnome-calculator
Total time:  2.063222886s
Total startup time: 2.027564926s
```

You can also disable usage of strace within strace to just get the time it took to display a window:

```
$ ./etrace run -s -d -t gnome-calculator
Gdk-Message: 19:26:11.684: gnome-calculator: Fatal IO error 11 (Resource temporarily unavailable) on X server :0.

Total startup time: 1.017437604s
```

Tracing what files are accessed by the `jq` snap:

```
$ ./etrace file --no-window-wait --cmd-stderr=/dev/null jq
3 files accessed during snap run:
     Filename                                              Size (bytes)
     /snap/jq/6/command-jq.wrapper                         322
     /snap/jq/6/meta/snap.yaml                             182
     /snap/jq/6/usr/lib/x86_64-linux-gnu/libonig.so.2.0.1  426840

Total startup time: 125.04791ms
```

## Current Limitations

Currently, the `file` subcommand has a few limitations. 

1. We currently only measure files that are under $SNAP, ideally this should be controllable by regex specified with an option
1. Due to 1, we only support measuring files accessed by snap apps, but this limitation should go away when we can specify what files to measure with an option.

## License
This project is licensed under the GPLv3. See LICENSE file for full license. Copyright 2019 Canonical Ltd.
