#!/bin/sh 

# remove any previous chromium snaps
snap remove chromium

# install our local version of the snap
snap install --dangerous /home/ijohnson/git/etrace/chromium_78.0.3904.70_amd64.snap
# snap install --dangerous /home/ijohnson/git/etrace/chromium_937.snap

# connect all interfaces
for iface in $(snap interfaces chromium 2>/dev/null | grep -P "^-" | awk '{print $2}'); 
    do snap connect "$iface" >&2
done

# delete any profiles 
rm -rf ~/.config/chromium ~/snap/chromium/current/* ~/snap/chromium/common/*
rm -rf ~/snap/chromium/current/.* ~/snap/chromium/common/.*

