#!/bin/sh -ex

# remove any previous chromium snaps
sudo snap remove "$1"

# delete $SNAP_USER_DATA entirely
rm -rf "$HOME/snap/$1"

# if the snap is chromium, also delete any chromium profile as well
if [ "$1" = "chromium" ]; then
    rm -rf ~/.config/chromium
fi

# install our local copy of the snap rev to insulate against upgrades to the
# stable channel
sudo snap install --dangerous "$2"

# connect all interfaces
for iface in $(snap interfaces "$1" 2>/dev/null | grep -P "^-" | awk '{print $2}'); 
    do sudo snap connect "$iface" >&2
done
