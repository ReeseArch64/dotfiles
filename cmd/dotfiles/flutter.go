package main

func flutterInstallStep(dotfiles string) step {
	return terminalStep(
		"Instalar Flutter via mise",
		"mise", "--cd", dotfiles, "install", "flutter",
	)
}

func flutterJob(dotfiles string) job {
	steps := miseSetupSteps(dotfiles)
	steps = append(steps, flutterInstallStep(dotfiles))
	return job{
		title: "Instalar ambiente Flutter",
		steps: steps,
	}
}
