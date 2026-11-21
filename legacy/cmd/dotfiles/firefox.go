package main

func firefoxJobFor(packageSteps []step) job {
	steps := append([]step{}, packageSteps...)
	steps = append(steps, terminalStep("Definir Mozilla Firefox como padrão", "xdg-settings", "set", "default-web-browser", "firefox.desktop"))
	return job{title: "Configurar Mozilla Firefox", steps: steps}
}

func firefoxJob() job {
	return firefoxJobFor(ensurePkgs("firefox", "xdg-utils"))
}
