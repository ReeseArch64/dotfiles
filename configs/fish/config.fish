# CachyOS Config
source /usr/share/cachyos-fish-config/cachyos-config.fish

# Added by Antigravity CLI installer
set -gx PATH "/home/alsgy/.local/bin" $PATH

# Hermes Agent
fish_add_path "$HOME/.local/bin"

# Mise
mise activate fish | source

# Alias
alias reload = "niri msg action load-config-file"
alias install = "sudo pacman -S --noconfirm"
alias update = "sudo pacman -Syy"
alias upgrade = "sudo pacman -Syyuu --noconfirm"
