package main

func idesJob() job {
	steps := ensureShellyPkgs("aur", "visual-studio-code-bin")
	steps = append(steps, ensureShellyPkgs("standard", "zed")...)
	return job{
		title: "Instalar IDEs",
		steps: steps,
	}
}
