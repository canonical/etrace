#!/bin/bash -ex

# allow resuming a previous run by defining datadir to a previously interrupted
# run
# if [ -z "$datadir" ]; then
# 	datadir=data-testrun-$RANDOM
# fi
datadir=data-testrun-exec-bootchart
mkdir -p $datadir

numAddtIterations=4

for s in gnome-calculator chromium supertuxkart libreoffice; do
    outFile=$datadir/$s-vanilla-startup-trace.json

    if ! test -f "$outFile.done"; then
        ./etrace --additional-iterations=$numAddtIterations run \
            --prepare-script=prepare-snap.sh \
            --prepare-script-args=$s \
            --prepare-script-args=/home/ijohnson/git/etrace/snap-repository/vanilla/$s.snap \
            --output-file=$outFile \
            --use-snap-run \
            --discard-snap-ns \
            --json \
            $s

        touch "$outFile.done"
    fi
done

# silly ones that need to have a window name specified
# for s in mari0 test-snapd-glxgears; do
#     case $s in 
#         mari0)
#             WINDOW_NAME=Mari0;;
#         test-snapd-glxgears)
#             WINDOW_NAME=glxgears;;
#     esac

#     outFile=$datadir/$s-ld-cache-startup-trace.json

#     if ! test -f "$outFile.done"; then
#         ./etrace --additional-iterations=$numAddtIterations run \
#             --prepare-script=prepare-snap.sh \
#             --prepare-script-args=$s \
#             --prepare-script-args=/home/ijohnson/git/etrace/snap-repository/ld-cache/$s.snap \
#             --window-name=$WINDOW_NAME \
#             --output-file=$outFile \
#             --use-snap-run \
#             --discard-snap-ns \
#             --json \
#             $s

#         touch "$outFile.done"
#     fi    
# done


exit 0

# datadir=data-testrun-second-notrace-startup
# mkdir -p $datadir

# numAddtIterations=4

# for s in gnome-calculator chromium supertuxkart libreoffice; do
#     outFile=$datadir/$s-vanilla-second-startup-notrace.json

#     if ! test -f "$outFile.done"; then
#         # run the prepare-snap script manually
#         ./prepare-snap.sh $s /home/ijohnson/git/etrace/snap-repository/vanilla/$s.snap
#         ./etrace --additional-iterations=$numAddtIterations run \
#             --output-file=$outFile \
#             --no-trace \
#             --use-snap-run \
#             --discard-snap-ns \
#             --json \
#             $s

#         touch "$outFile.done"
#     fi
# done

# # silly ones that need to have a window name specified
# for s in mari0 test-snapd-glxgears; do
#     case $s in 
#         mari0)
#             WINDOW_NAME=Mari0;;
#         test-snapd-glxgears)
#             WINDOW_NAME=glxgears;;
#     esac

#     outFile=$datadir/$s-vanilla-second-startup-notrace.json

#     if ! test -f "$outFile.done"; then
#         # run the prepare-snap script manually
#         ./prepare-snap.sh $s /home/ijohnson/git/etrace/snap-repository/vanilla/$s.snap
#         ./etrace --additional-iterations=$numAddtIterations run \
#             --window-name=$WINDOW_NAME \
#             --output-file=$outFile \
#             --no-trace \
#             --use-snap-run \
#             --discard-snap-ns \
#             --json \
#             $s

#         touch "$outFile.done"
#     fi    
# done

datadir=data-testrun-second-exec-bootchart
mkdir -p $datadir

numAddtIterations=4

# for s in gnome-calculator chromium supertuxkart libreoffice; do
for s in supertuxkart; do
    outFile=$datadir/$s-vanilla-second-startup-trace.json

    if ! test -f "$outFile.done"; then
        # run the prepare-snap script manually
        ./prepare-snap.sh $s /home/ijohnson/git/etrace/snap-repository/vanilla/$s.snap
        ./etrace --additional-iterations=$numAddtIterations run \
            --output-file=$outFile \
            --use-snap-run \
            --discard-snap-ns \
            --json \
            $s

        touch "$outFile.done"
    fi
done

# # silly ones that need to have a window name specified
# for s in mari0 test-snapd-glxgears; do
#     case $s in 
#         mari0)
#             WINDOW_NAME=Mari0;;
#         test-snapd-glxgears)
#             WINDOW_NAME=glxgears;;
#     esac

#     outFile=$datadir/$s-vanilla-second-startup-trace.json

#     if ! test -f "$outFile.done"; then
#         # run the prepare-snap script manually
#         ./prepare-snap.sh $s /home/ijohnson/git/etrace/snap-repository/vanilla/$s.snap

#         ./etrace --additional-iterations=$numAddtIterations run \
#             --window-name=$WINDOW_NAME \
#             --output-file=$outFile \
#             --use-snap-run \
#             --discard-snap-ns \
#             --json \
#             $s

#         touch "$outFile.done"
#     fi    
# done

