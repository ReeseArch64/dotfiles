#!/bin/sh
set -eu

main() {
    : "${HOME:?HOME precisa estar definido}"
    case "$(uname -m)" in
        x86_64|amd64) archive=dotfiles-linux-amd64.tar.gz ;;
        *) printf 'Arquitetura sem pacote publicado.\n' >&2; exit 1 ;;
    esac

    release=https://github.com/ReeseArch64/dotfiles/releases/latest/download
    dotfiles_dir=$HOME/.dotfiles
    bin_dir=$HOME/.local/bin
    work_dir=$(mktemp -d "$HOME/.dotfiles-install.XXXXXXXX")
    staged_bin=
    trap 'if [ -n "$staged_bin" ]; then rm -f -- "$staged_bin"; fi; rm -rf -- "$work_dir"' EXIT
    trap 'exit 1' HUP INT TERM

    cd "$work_dir"
    curl -fsSL "$release/$archive" -o "$archive"
    curl -fsSL "$release/$archive.sha256" -o "$archive.sha256"
    sha256sum --check --strict "$archive.sha256"

    mkdir package
    tar -xzf "$archive" -C package
    mkdir -p "$bin_dir"
    staged_bin=$(mktemp "$bin_dir/.dotfiles.XXXXXXXX")
    install -m 0755 package/bin/dotfiles "$staged_bin"

    if [ -d "$dotfiles_dir" ]; then
        printf 'Mantendo os arquivos existentes em %s.\n' "$dotfiles_dir"
    elif [ -e "$dotfiles_dir" ] || [ -L "$dotfiles_dir" ]; then
        printf 'O caminho %s precisa ser um diretório.\n' "$dotfiles_dir" >&2
        exit 1
    else
        mkdir data
        for environment in cachyos ubuntu; do
            if [ -d "package/$environment" ]; then
                mv "package/$environment" data/
            else
                mkdir "data/$environment"
            fi
        done
        mv -T --no-clobber data "$dotfiles_dir"
    fi

    mv -fT -- "$staged_bin" "$bin_dir/dotfiles"
    printf 'Instalado em %s/dotfiles.\n' "$bin_dir"
    case ":${PATH-}:" in
        *":$bin_dir:"*) printf 'Execute dotfiles para abrir o menu.\n' ;;
        *) printf 'Para abrir o menu, execute "%s/dotfiles".\n' "$bin_dir" ;;
    esac
}

main "$@"
