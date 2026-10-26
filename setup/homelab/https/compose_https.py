#!/usr/bin/env python3
"""Prepare CasaOS Compose JSON for a LAN-only Vaultwarden HTTPS proxy."""

import argparse
import json
import pathlib
import sys


def matching_service(compose, image_prefix):
    matches = [
        (name, service)
        for name, service in compose.get("services", {}).items()
        if service.get("image", "").startswith(image_prefix)
    ]
    if len(matches) != 1:
        raise ValueError(
            f"esperado exatamente um serviço com imagem {image_prefix!r}; encontrados: "
            + ", ".join(name for name, _ in matches)
        )
    return matches[0]


def add_proxy_network(compose, service):
    compose.setdefault("networks", {})["homelab_proxy"] = {
        "external": True,
        "name": "homelab-proxy",
    }
    networks = service.get("networks")
    if networks is None:
        networks = {}
        service["networks"] = networks
    if isinstance(networks, list):
        if "homelab_proxy" not in networks:
            networks.append("homelab_proxy")
    else:
        networks.setdefault("homelab_proxy", None)


def configure_caddy(compose, app_data, server_ip):
    _, service = matching_service(compose, "caddy:")
    add_proxy_network(compose, service)
    service["entrypoint"] = ["/config/bin/caddy"]
    service["command"] = [
        "run",
        "--config",
        "/etc/caddy/caddyfile/Caddyfile",
        "--adapter",
        "caddyfile",
    ]
    environment = service.get("environment")
    if isinstance(environment, dict):
        environment.pop("CF_API_TOKEN", None)
    elif isinstance(environment, list):
        service["environment"] = [entry for entry in environment if not entry.startswith("CF_API_TOKEN=")]
    service["env_file"] = [str(app_data / "config" / "cloudflare.env")]
    service["ports"] = [
        {
            "target": 443,
            "published": "443",
            "host_ip": server_ip,
            "protocol": "tcp",
            "mode": "ingress",
        },
    ]
    metadata = compose.setdefault("x-casaos", {})
    metadata["port_map"] = "443"
    metadata["scheme"] = "https"


def configure_vaultwarden(compose, domain):
    _, service = matching_service(compose, "vaultwarden/server:")
    add_proxy_network(compose, service)
    environment = service.get("environment")
    if environment is None:
        environment = {}
        service["environment"] = environment
    if isinstance(environment, list):
        converted = {}
        for entry in environment:
            key, _, value = entry.partition("=")
            converted[key] = value
        environment = converted
        service["environment"] = environment
    environment["DOMAIN"] = f"https://{domain}"
    # Keep a loopback diagnostic endpoint while preventing LAN clients from
    # bypassing HTTPS. Caddy reaches Vaultwarden through homelab-proxy.
    service["ports"] = [
        {
            "target": 80,
            "published": "10380",
            "host_ip": "127.0.0.1",
            "protocol": "tcp",
            "mode": "ingress",
        }
    ]


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("kind", choices=("caddy", "vaultwarden"))
    parser.add_argument("source", type=pathlib.Path)
    parser.add_argument("destination", type=pathlib.Path)
    parser.add_argument("--domain", default="homelab.reesearch64.com")
    parser.add_argument("--app-data", type=pathlib.Path, default=pathlib.Path("/DATA/AppData/caddy"))
    parser.add_argument("--server-ip")
    args = parser.parse_args()

    with args.source.open(encoding="utf-8") as stream:
        compose = json.load(stream)
    if args.kind == "caddy":
        if not args.server_ip:
            raise ValueError("--server-ip é obrigatório para o Caddy")
        configure_caddy(compose, args.app_data, args.server_ip)
    else:
        configure_vaultwarden(compose, args.domain)
    with args.destination.open("w", encoding="utf-8") as stream:
        json.dump(compose, stream, indent=2, sort_keys=True)
        stream.write("\n")


if __name__ == "__main__":
    try:
        main()
    except (OSError, ValueError, json.JSONDecodeError) as error:
        print(f"erro: {error}", file=sys.stderr)
        raise SystemExit(1)
