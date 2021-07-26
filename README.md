# etrace
[![Actions Status](https://github.com/anonymouse64/etrace/workflows/Go/badge.svg)](https://github.com/anonymouse64/etrace/actions)
[![Snap Status](https://github.com/canonical/etrace/actions/workflows/snap.yml/badge.svg)](https://github.com/canonical/etrace/actions/workflows/snap.yml)
[![etrace](https://snapcraft.io//etrace/badge.svg)](https://snapcraft.io/etrace)
[![Go Report Card](https://goreportcard.com/badge/github.com/canonical/etrace)](https://goreportcard.com/report/github.com/canonical/etrace)

_etrace_ is a utility like strace or ltrace which uses ptrace to follow programs executed by a main program for performance and debugging analysis. It also supports limited tracing of file accesses for analyzing what files a program accesses during it's execution.

## Basics

Simple, quick (relatively speaking) and dirty way to get numbers on how fast/slow a graphical snap like [`gnome-calculator`](https://snapcraft.io/gnome-calculator) is:

Worst case performance (like when you first turn on and log into your machine):

```bash
$ etrace exec -t --silent --cold gnome-calculator
Total startup time: 4.169633359
```

Best case performance (after everything is cached and ready to go):

```bash
$ etrace exec -t --silent --hot gnome-calculator > /dev/null # to warm up all caches
$ etrace exec -t --silent --hot gnome-calculator # actual measurement
Total startup time: 1.054272336
```

## Installation

The easiest way to get etrace is via a snap:

```bash
$ snap install etrace
```

It is currently under active development, and thus it is suggested to install
from the candidate channel with the `--candidate` option.

Alternatively, this project uses go modules, so you can also `git clone` it and then `go install` it.

```bash
$ git clone https://github.com/anonymouse64/etrace 
$ cd etrace && go install ./...
```

## Usage

_etrace_ has three subcommands, `exec`, `file`, and `analyze-snap`.

### `exec` subcommand

`exec` is used for tracing the programs that are executed with the `execve` family of syscalls. It can also have tracing turned off to just measure graphical timing information.

_etrace exec_ usage:

```
Usage:
  etrace [OPTIONS] exec [exec-OPTIONS] Cmd...

Application Options:
  -e, --errors                    Show errors as they happen
  -w, --window-name=              Window name to wait for
  -p, --prepare-script=           Script to run to prepare a run
      --prepare-script-args=      Args to provide to the prepare script
  -r, --restore-script=           Script to run to restore after a run
      --restore-script-args=      Args to provide to the restore script
  -v, --keep-vm-caches            Don't free VM caches before executing
  -c, --class-name=               Window class to use with xdotool instead of the the first Command
      --window-class-name=        Window class name to use with xdotool
  -s, --use-snap-run              Run command through snap run
  -f, --use-flatpak-run           Run command through flatpak run
  -d, --discard-snap-ns           Discard the snap namespace before running the snap
      --cmd-stdout=               Log file for run command's stdout
      --silent                    Silence all program output
      --cmd-stderr=               Log file for run command's stderr
  -j, --json                      Output results in JSON
  -o, --output-file=              A file to output the results (empty string means stdout)
      --no-window-wait            Don't wait for the window to appear, just run until the program exits
      --window-timeout=           Global timeout for waiting for windows to appear. Set to empty string to use no timeout (default: 60s)

Help Options:
  -h, --help                      Show this help message

[exec command options]
      -t, --no-trace              Don't trace the process, just time the total execution
          --clean-snap-user-data  Delete snap user data before executing and restore after execution
          --reinstall-snap        Reinstall the snap before executing, restoring any existing interface connections for the snap
      -n, --repeat=               Number of times to repeat each task
          --cold                  Use set of options for worst case, cold cache, etc performance
          --hot                   Use set of options for best case, hot cache, etc performance

[exec command arguments]
  Cmd:                            Command to run
```

### `file` subcommand

The `file` subcommand will track all syscalls that a program executes which access files. This is useful for measuring the total set of files that a program attempts to access during its execution.

_etrace file_ usage:

```
Usage:
  etrace [OPTIONS] file [file-OPTIONS] Cmd...

Application Options:
  -e, --errors                      Show errors as they happen
  -w, --window-name=                Window name to wait for
  -p, --prepare-script=             Script to run to prepare a run
      --prepare-script-args=        Args to provide to the prepare script
  -r, --restore-script=             Script to run to restore after a run
      --restore-script-args=        Args to provide to the restore script
  -v, --keep-vm-caches              Don't free VM caches before executing
  -c, --class-name=                 Window class to use with xdotool instead of the the first Command
      --window-class-name=          Window class name to use with xdotool
  -s, --use-snap-run                Run command through snap run
  -f, --use-flatpak-run             Run command through flatpak run
  -d, --discard-snap-ns             Discard the snap namespace before running the snap
      --cmd-stdout=                 Log file for run command's stdout
      --silent                      Silence all program output
      --cmd-stderr=                 Log file for run command's stderr
  -j, --json                        Output results in JSON
  -o, --output-file=                A file to output the results (empty string means stdout)
      --no-window-wait              Don't wait for the window to appear, just run until the program exits
      --window-timeout=             Global timeout for waiting for windows to appear. Set to empty string to use no timeout (default: 60s)

Help Options:
  -h, --help                        Show this help message

[file command options]
          --file-regex=             Regular expression of files to return, if empty all files are returned
          --parent-dirs=            List of parent directories matching files must be underneath to match
          --program-regex=          Regular expression of programs whose file accesses should be returned
          --include-snapd-programs  Include snapd programs whose file accesses match in the list of files accessed
          --show-programs           Show programs that accessed the files

[file command arguments]
  Cmd:                              Command to run
```

### `analyze-snap` subcommand

The `analyze-snap` subcommand will run a few different tests of the specified snap, mainly heuristics around guesses of what might be relevant to why a graphical snap is performing poorly. It takes a snap name, and will install that snap from the store (with an optional channel specification) if it is not already installed. It will make a backup of all the snap user data for that snap before executing tests, but this is not 100% foolproof, so it is suggested that you manually backup any sensitive data for the snap. The snap will also be removed and reinstalled multiple times, but any revisions of the snap that are inactive (i.e. old revisions) that exist at the time of running the command will be lost due to garbage collection by snapd when removing and reinstalling the snap.

Right now, the main assumption it makes is that most snaps that are XZ are slow, and that they would benefit from switching to LZO, so if the specified snap is using XZ, it will analyze the results of switching to XZ. Eventually, it would be great to extend it to do other sorts of performance/execution related analysis. If the snap is already LZO, then it skips the comparison and just displays the cold/hot statistics.

The subcomand currently doesn't obey all of the global options that etrace uses, but that should be fixed up soon.

Finally, note that since it executes the specified snap 40 times, with 20 of those executions "slow" worst case performance scenarios, it can take a decent chunk of time to finish, but just give it time and it will spit out results. A progress bar would be a great addition that I haven't added yet.

_etrace analyze-snap_ usage:

```
Usage:
  etrace [OPTIONS] analyze-snap [analyze-snap-OPTIONS] Snap

Application Options:
  -e, --errors               Show errors as they happen
  -w, --window-name=         Window name to wait for
  -p, --prepare-script=      Script to run to prepare a run
      --prepare-script-args= Args to provide to the prepare script
  -r, --restore-script=      Script to run to restore after a run
      --restore-script-args= Args to provide to the restore script
  -v, --keep-vm-caches       Don't free VM caches before executing
  -c, --class-name=          Window class to use with xdotool instead of the the first Command
      --window-class-name=   Window class name to use with xdotool
  -s, --use-snap-run         Run command through snap run
  -f, --use-flatpak-run      Run command through flatpak run
  -d, --discard-snap-ns      Discard the snap namespace before running the snap
      --cmd-stdout=          Log file for run command's stdout
      --silent               Silence all program output
      --cmd-stderr=          Log file for run command's stderr
  -j, --json                 Output results in JSON
  -o, --output-file=         A file to output the results (empty string means stdout)
      --no-window-wait       Don't wait for the window to appear, just run until the program exits
      --window-timeout=      Global timeout for waiting for windows to appear. Set to empty string to use no timeout (default: 60s)

Help Options:
  -h, --help                 Show this help message

[analyze-snap command options]
          --channel=         Channel to install the snap from if not already installed

[analyze-snap command arguments]
  Snap:                      Snap to analyze
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

Analyzing a snap (note that this example took about 15 minutes to execute on a decently powerful Ubuntu desktop):

```
$ etrace analyze-snap 1password
original snap size: 82.45 MiB
original compression format is xz
content snap slot dependencies: [gnome-3-28-1804 gtk-common-themes]
worst case performance:
        average time to display: 14.021772885s
        standard deviation for time to display: 598.600444ms
best case performance:
        average time to display: 1.096632064s
        standard deviation for time to display: 45.572462ms
worst case performance with LZO compression:
        average time to display: 4.692383384s
        standard deviation for time to display: 51.066194ms
        average time to display percent change: -66.54%
best case performance with LZO compression:
        average time to display: 1.110948092s
        standard deviation for time to display: 19.30841ms
        average time to display percent change: +1.31%
lzo snap size: 105.62 MiB (change of +28.10%)
```

## Current Limitations

Currently, the `file` subcommand has a few limitations. 

1. We currently only measure files that are under $SNAP, ideally this should be controllable by regex specified with an option
1. Due to 1, we only support measuring files accessed by snap apps, but this limitation should go away when we can specify what files to measure with an option.

The `analyze-snap` subcommand has the following limitations:

1. None of the global options that are shared with `exec` and `file` are not yet understood/passed along.
1. The output for `analyze-snap` isn't stable and will probably be adjusted
1. The `analyze-snap` command can take a long time but doesn't convey at all how far along it is in the execution.

## License
This project is licensed under the GPLv3. See LICENSE file for full license. Copyright 2019-2021 Canonical Ltd.
