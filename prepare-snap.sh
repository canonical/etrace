#!/bin/sh -ex

# remove any previous chromium snaps
sudo snap remove "$1"

# delete $SNAP_USER_DATA entirely
rm -rf "$HOME/snap/$1"

# if the snap is chromium, also delete any chromium profile as well
if [ "$1" = "chromium" ]; then
    rm -rf ~/.config/chromium
fi

# if the snap is using the gnome content snaps, remove those and reinstall them
# so that we don't end up caching those snaps as well
if [ "$1" = "libreoffice" ]; then
    snap remove gnome-3-28-1804 gtk-common-themes

    # for snap-repository versions
    snap install --dangerous /home/ijohnson/git/etrace/snap-repository/no-compression/gnome-3-28-1804.snap
    snap install --dangerous /home/ijohnson/git/etrace/snap-repository/no-compression/gtk-common-themes.snap

    # for vanilla versions
    # snap install gnome-3-28-1804 gtk-common-themes
fi

# install our local copy of the snap rev to insulate against upgrades to the
# stable channel
sudo snap install --dangerous "$2"

# try connecting finnicky interfaces which require a slot to be specified if 
# they are plugged by the snap
for iface in icon-themes gtk-3-themes sound-themes; do
    if snap connections "$1" | grep -q $iface; then
        snap connect "$1:$iface" "gtk-common-themes:$iface"
    fi
done

if snap connections "$1" | grep -q "gnome-3-28-1804"; then
    snap connect "$1:gnome-3-28-1804" gnome-3-28-1804:gnome-3-28-1804
fi

# connect all interfaces
# TODO: update to use `snap connections` instead
for iface in $(snap interfaces "$1" 2>/dev/null | grep -P "^-" | awk '{print $2}'); 
    do sudo snap connect "$iface" >&2
done
