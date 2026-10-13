package main

func idesJob() job {
	steps := ensureShellyPkgs("aur", "visual-studio-code-bin", "android-studio")
	steps = append(steps, ensureShellyPkgs("standard", "zed")...)
	return job{
		title: "Instalar IDEs",
		steps: steps,
	}
}
