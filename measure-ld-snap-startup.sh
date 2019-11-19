#!/bin/bash -ex

run=$RANDOM

go build ./...

for opt in trace notrace; do
    for the_snap in gnome-calculator chromium; do

        vanillafile=$the_snap-$opt-snap-$run.txt
        ldfile=$the_snap-ld-snap-$run.txt

        touch $vanillafile
        touch $ldfile

        for i in $(seq 1 10); do
            if [ $opt = trace ]; then
                out=$(./etrace run -p "$(pwd)/prepare-$the_snap.sh" -c "$the_snap" snap run "$the_snap" 2>&1)
            else 
                out=$(./etrace run -t -p "$(pwd)/prepare-$the_snap.sh" -c "$the_snap" snap run "$the_snap" 2>&1)
            fi
            start=$(echo "$out" | grep "Total startup time:" | awk '{print $4}')
            echo "$start" >> "$vanillafile"
        done

        for j in $(seq 1 10); do
            if [ $opt = trace ]; then
                out=$(./etrace run -p "$(pwd)/prepare-ld-cached-$the_snap.sh" -c "$the_snap" snap run "$the_snap" 2>&1)
            else 
                out=$(./etrace run -t -p "$(pwd)/prepare-ld-cached-$the_snap.sh" -c "$the_snap" snap run "$the_snap" 2>&1)
            fi
            
            ldstart=$(echo "$out" | grep "Total startup time:" | awk '{print $4}')
            echo "$ldstart" >> "$ldfile"
        done
    done
done