#!/usr/bin/env bash

terminate_setup() {
    killall -q polybar
    while pgrep -u $UID -x polybar >/dev/null; do sleep 1; done
}

terminate_setup
polybar example &
