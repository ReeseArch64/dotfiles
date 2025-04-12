#!/bin/sh

#######################################################################
#                                                                    #
#  ██████  ▄▄▄       ▄████▄   ██ ▄█▀ ██▓███   ██ ▄█▀ ██▓███  ▒█████   #
# ▒██    ▒ ▒████▄    ▒██▀ ▀█   ██▄█▒ ▓██░  ██▒██▄█▒ ▓██░  ██▒██▒  ██▒ #
# ░ ▓██▄   ▒██  ▀█▄  ▒▓█    ▄ ▓███▄░ ▓██░ ██▓▓███▄░ ▓██░ ██▓▒██░  ██▒ #
#   ▒   ██▒░██▄▄▄▄██ ▒▓▓▄ ▄██▒▓██ █▄ ▒██▄█▓▒ ▓██ █▄ ▒██▄█▓▒ ▒██   ██░ #
# ▒██████▒▒ ▓█   ▓██▒▒ ▓███▀ ░▒██▒ █▄▒██▒ ░  ▒██▒ █▄▒██▒ ░  ░ ████▓▒░ #
# ▒ ▒▓▒ ▒ ░ ▒▒   ▓▒█░░ ░▒ ▒  ░▒ ▒▒ ▓▒▒▓▒░ ░  ▒ ▒▒ ▓▒▒▓▒░ ░  ░ ▒░▒░▒░  #
# ░ ░▒  ░ ░  ▒   ▒▒ ░  ░  ▒   ░ ░▒ ▒░░▒ ░    ░ ░▒ ▒░░▒ ░      ░ ▒ ▒░  #
# ░  ░  ░    ░   ▒   ░        ░ ░░ ░ ░░        ░░ ░ ░░      ░ ░ ░ ▒   #
#       ░        ░  ░░ ░      ░  ░               ░            ░ ░   #
#                    ░                                             #
#           Automatic sync with rclone – BACKUP SCRIPT             #
#######################################################################

rclone sync $HOME/Documents gdrive:Documents --progress --include ".*"
rclone sync $HOME/Music gdrive:Music --progress --include ".*"

rclone sync $HOME/Templates gdrive:Templates --progress --include ".*"
rclone sync $HOME/Public gdrive:Public --progress --include ".*"

rclone sync $HOME/Videos gphotos:Videos --progress --include ".*"
rclone sync $HOME/Photos/Wallpapers gphotos:Wallpapers --progress --include ".*"
