package main

func protonVPNJob() job {
	return job{title: "Instalar Proton VPN CLI", steps: ensureYayPkgs("proton-vpn-cli")}
}
