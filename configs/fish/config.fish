# CachyOS Config
source /usr/share/cachyos-fish-config/cachyos-config.fish

# Mise
mise activate fish | source

# Alias
alias reload="niri msg action load-config-file"
alias install="sudo pacman -S --noconfirm"
alias update="sudo pacman -Syy"
alias upgrade="sudo pacman -Syyuu --noconfirm"
