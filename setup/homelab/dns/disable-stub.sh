#!/usr/bin/env bash
set -euo pipefail

config=$1
resolver=$2
stub=$3
upstream=$4
shift 4

# Drop-ins take precedence over resolved.conf. Refuse conflicting settings rather
# than claiming the port was freed while another configuration enables the stub.
if [[ $# -eq 0 ]]; then
    set -- /etc/systemd/resolved.conf.d /run/systemd/resolved.conf.d /usr/local/lib/systemd/resolved.conf.d /usr/lib/systemd/resolved.conf.d
fi
for directory in "$@"; do
    for dropin in "$directory"/*.conf; do
        [[ -f $dropin ]] || continue
        if ! awk '
            /^[[:space:]]*\[/ {
                resolve = ($0 ~ /^[[:space:]]*\[Resolve\][[:space:]]*$/)
                next
            }
            resolve && /^[[:space:]]*DNSStubListener[[:space:]]*=/ {
                value = $0
                sub(/^[^=]*=[[:space:]]*/, "", value)
                sub(/[[:space:]]*$/, "", value)
                if (value !~ /^(no|false|off|0)$/) conflict = 1
            }
            END { exit conflict }
        ' "$dropin"; then
            printf 'Drop-in DNSStubListener conflitante: %s; ajuste-o antes de continuar. Nenhuma alteração realizada.\n' "$dropin" >&2
            exit 1
        fi
    done
done

if [[ ! -f $config || -L $config ]]; then
    printf 'resolved.conf deve ser um arquivo regular: %s\n' "$config" >&2
    exit 1
fi
switch_resolver=false
if [[ -L $resolver && $(readlink -m -- "$resolver") == $(readlink -m -- "$stub") ]]; then
    if [[ ! -r $upstream ]] || ! grep -Eq '^nameserver[[:space:]]+' "$upstream" || grep -Eq '^nameserver[[:space:]]+127\.0\.0\.53([[:space:]]|$)' "$upstream"; then
        printf 'Resolver upstream indisponível: %s; nenhuma alteração realizada.\n' "$upstream" >&2
        exit 1
    fi
    switch_resolver=true
elif [[ ! -r $resolver ]] || grep -Eq '^nameserver[[:space:]]+127\.0\.0\.53([[:space:]]|$)' "$resolver"; then
    printf 'resolv.conf externo depende do stub ou está indisponível; ajuste seu gerenciador DNS antes de continuar.\n' >&2
    exit 1
fi

staging=$(mktemp -d "$(dirname "$config")/.dotfiles-dns.XXXXXX")
trap 'rm -rf -- "$staging"' EXIT
awk '
    /^[[:space:]]*\[/ {
        resolve = ($0 ~ /^[[:space:]]*\[Resolve\][[:space:]]*$/)
        print
        if (resolve) {
            print "DNSStubListener=no"
            found = 1
        }
        next
    }
    resolve && /^[[:space:]]*#?[[:space:]]*DNSStubListener[[:space:]]*=/ { next }
    { print }
    END { if (!found) print "\n[Resolve]\nDNSStubListener=no" }
' "$config" > "$staging/resolved.conf"

if ! cmp -s -- "$config" "$staging/resolved.conf"; then
    if [[ ! -e $config.dotfiles-backup ]]; then
        cp -p -- "$config" "$config.dotfiles-backup"
    fi
fi
if $switch_resolver; then
    if [[ ! -e $resolver.dotfiles-backup && ! -L $resolver.dotfiles-backup ]]; then
        cp -P -- "$resolver" "$resolver.dotfiles-backup"
    fi
    ln -s -- "$upstream" "$staging/resolv.conf"
    mv -Tf -- "$staging/resolv.conf" "$resolver"
fi
if ! cmp -s -- "$config" "$staging/resolved.conf"; then
    # Preserve ownership/mode (and any ACLs) of the system configuration.
    cp --attributes-only --preserve=all -- "$config" "$staging/resolved.conf"
    mv -f -- "$staging/resolved.conf" "$config"
fi
systemctl restart systemd-resolved
