package main

const thunderbirdPackage = "thunderbird"

func thunderbirdJobFor(packageSteps []step) job {
	return job{
		title: "Instalar Thunderbird",
		steps: packageSteps,
	}
}

func thunderbirdJob() job {
	return thunderbirdJobFor(ensurePkgs(thunderbirdPackage))
}
