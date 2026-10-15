package main

func idesJob() job {
	steps := ensureYayPkgs("visual-studio-code-bin", "android-studio")
	steps = append(steps, ensureYayPkgs("zed")...)
	return job{
		title: "Instalar IDEs",
		steps: steps,
	}
}
