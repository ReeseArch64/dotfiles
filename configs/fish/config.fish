# CachyOS Config
source /usr/share/cachyos-fish-config/conf.d/done.fish

# Fastfetch
fastfetch --config $HOME/.config/fastfetch/config.json --logo-type chafa --logo "$HOME/.face"

# Mise
mise activate fish | source

# Format man pages
set -x MANROFFOPT "-c"
set -x MANPAGER "sh -c 'col -bx | bat -l man -p'"

# Set settings for https://github.com/franciscolourenco/done
set -U __done_min_cmd_duration 10000
set -U __done_notification_urgency_level low

# Append common directories for executable files to $PATH
fish_add_path ~/.local/bin ~/.cargo/bin ~/Applications/depot_tools

## Functions
# Functions needed for !! and !$ https://github.com/oh-my-fish/plugin-bang-bang
function __history_previous_command
  switch (commandline -t)
  case "!"
    commandline -t $history[1]; commandline -f repaint
  case "*"
    commandline -i !
  end
end

function __history_previous_command_arguments
  switch (commandline -t)
  case "!"
    commandline -t ""
    commandline -f history-token-search-backward
  case "*"
    commandline -i '$'
  end
end

if [ "$fish_key_bindings" = fish_vi_key_bindings ];
  bind -Minsert ! __history_previous_command
  bind -Minsert '$' __history_previous_command_arguments
else
  bind ! __history_previous_command
  bind '$' __history_previous_command_arguments
end

# Fish command history
function history
    builtin history --show-time='%F %T ' $argv
end

function backup --argument filename
    cp $filename $filename.bak
end

# Copy DIR1 DIR2
function copy
    set count (count $argv | tr -d \n)
    if test "$count" = 2; and test -d "$argv[1]"
        set from (echo $argv[1] | trim-right /)
        set to (echo $argv[2])
        command cp -r $from $to
    else
        command cp $argv
    end
end

# Alias
alias reload="niri msg action load-config-file"

alias install="sudo pacman -S --noconfirm"
alias update="sudo pacman -Syy"

alias upgrade='sudo cachyos-rate-mirrors && sudo pacman -Syyuu --noconfirm'

alias fixpacman="sudo rm /var/lib/pacman/db.lck"

alias grubup="sudo grub-mkconfig -o /boot/grub/grub.cfg"

alias ls='eza -al --color=always --group-directories-first --icons=always'
alias la='eza -a --color=always --group-directories-first --icons=always'
alias ll='eza -l --color=always --group-directories-first --icons=always'
alias lt='eza -aT --color=always --group-directories-first --icons=always'

alias tarnow='tar -acf '
alias untar='tar -zxvf '

alias wget='wget -c '

alias mirror="sudo cachyos-rate-mirrors"

alias enablenow="sudo systemctl enable --now"
alias enable="sudo systemctl enable"
alias start="sudo systemctl start"
alias stop="sudo systemctl stop"

alias pu="pi update --extensions"
