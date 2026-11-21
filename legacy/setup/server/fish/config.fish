# Configuração portátil para Homelab e HomeProd (Ubuntu).
# Recursos de histórico do CachyOS; sem dependências de notificações gráficas.
if not status is-interactive
    return
end

fish_add_path ~/.local/bin ~/.cargo/bin

function fish_greeting
    if command -q fastfetch
        if command -q chafa; and test -r "$HOME/.face"
            fastfetch --config "$HOME/.config/fastfetch/config.json"
        else
            fastfetch --config "$HOME/.config/fastfetch/config.json" --logo-type builtin --logo ubuntu
        end
    end
end

if command -q mise
    mise activate fish | source
end

set -gx MANROFFOPT '-c'
if command -q col
    if command -q bat
        set -gx MANPAGER "sh -c 'col -bx | bat -l man -p'"
    else if command -q batcat
        set -gx MANPAGER "sh -c 'col -bx | batcat -l man -p'"
    end
end

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

if test "$fish_key_bindings" = fish_vi_key_bindings
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

function backup --argument-names filename
    if test (count $argv) -ne 1
        echo 'Uso: backup arquivo' >&2
        return 2
    end
    cp -- "$filename" "$filename.bak"
end

# Ubuntu: use as ferramentas disponíveis, sem depender do pacote CachyOS.
if command -q eza
    alias ls='eza -al --color=always --group-directories-first --icons=always'
    alias la='eza -a --color=always --group-directories-first --icons=always'
    alias ll='eza -l --color=always --group-directories-first --icons=always'
    alias lt='eza -aT --color=always --group-directories-first --icons=always'
else
    alias ls='ls --color=auto --group-directories-first'
    alias la='ls -A'
    alias ll='ls -alh'
    alias lt='ls -alht'
end

alias install='sudo apt-get install'
alias update='sudo apt-get update'
alias upgrade='sudo apt-get update && sudo apt-get upgrade'
alias tarnow='tar -acf'
alias untar='tar -zxvf'
alias wget='wget -c'
alias enablenow='sudo systemctl enable --now'
alias enable='sudo systemctl enable'
alias start='sudo systemctl start'
alias restart='sudo systemctl restart'
alias stop='sudo systemctl stop'
