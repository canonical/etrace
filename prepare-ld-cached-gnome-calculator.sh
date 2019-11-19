#!/bin/sh 

# remove any previous gnome-calculator snaps
snap remove gnome-calculator

# install our local version of the snap
snap install --dangerous /home/ijohnson/git/etrace/gnome-calculator_3.34.1+git1.d34dc842_amd64.snap
# snap install --dangerous /home/ijohnson/git/etrace/gnome-calculator_937.snap

# connect all interfaces
for iface in $(snap interfaces gnome-calculator 2>/dev/null | grep -P "^-" | awk '{print $2}'); 
    do snap connect "$iface" >&2
done

sudo snap run --hook=install gnome-calculator

# delete any profiles 
rm -rf ~/snap/gnome-calculator/current/* ~/snap/gnome-calculator/common/*
rm -rf ~/snap/gnome-calculator/current/.* ~/snap/gnome-calculator/common/.*

