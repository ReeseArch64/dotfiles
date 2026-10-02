package main

var terminalToolPackages = []string{"vim", "neovim", "wget", "curl", "bat", "eza", "tree"}

func terminalToolsJob() job {
	return job{
		title: "Instalar ferramentas de terminal",
		steps: ensureShellyPkgs("standard", terminalToolPackages...),
	}
}
