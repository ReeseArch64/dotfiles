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
        for environment in cachyos ubuntu; do
            if [ -d "package/$environment" ]; then
                mkdir -p "$dotfiles_dir/$environment"
                cp -Rn -- "package/$environment/." "$dotfiles_dir/$environment/"
            fi
        done
        if [ -d package/cachyos ]; then
            fish_template=$dotfiles_dir/cachyos/fish/config.fish
            # The 1.7.x template predates the dependency checks in fish_greeting.
            if [ -f package/cachyos/fish/config.fish ] && [ -f "$fish_template" ] &&
                [ ! -L "$fish_template" ] && [ ! -L "$dotfiles_dir/cachyos/fish" ] &&
                [ "$(sha256sum < "$fish_template")" = '5ebc1cc6f7fb2aa2faeec92a5587b0bb923a6800554c28efbfa709641fb5cac2  -' ]
            then
                install -m 0644 package/cachyos/fish/config.fish "$fish_template"
            fi
            mise_template=$dotfiles_dir/cachyos/mise/mise.toml
            # Upgrade the original templates from releases 1.10 through 1.21.
            if [ -f package/cachyos/mise/mise.toml ] && [ -f "$mise_template" ] &&
                [ ! -L "$mise_template" ] && [ ! -L "$dotfiles_dir/cachyos/mise" ]
            then
                case "$(sha256sum < "$mise_template")" in
                    'b86df137b10200eaf638dd256657ce0de50ffc8759617a9c7b45e80fe7ee9bb7  -'|\
                    '8f4e2ec67693106769359d70eaa92095d907707abfbb0d75bafd3bd36e3ed343  -'|\
                    'b633ce4567fbfa9554e2dc24eae48cad30f1e31234757b763daa82b25c52d691  -'|\
                    '8e99e7cb3e6672716fe5141a405acf8882896451e140c83dfd5defe35d376fc4  -'|\
                    '516d0cac0c0bf89cd8a1f91cd66211c1e721fea7204f2dfdd61bd0b8fe407d22  -'|\
                    '8d65d7ac5a2ebf11c97915742833500920c74b685fd0281178da82b3995ebf1f  -')
                        install -m 0644 package/cachyos/mise/mise.toml "$mise_template"
                        ;;
                esac
            fi
        fi
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
