package main

const androidCLIInstallCommand = "curl -fsSL https://dl.google.com/android/cli/latest/linux_x86_64/install.sh | bash"

func androidCLIJob() job {
	return job{
		title: "Instalar Android CLI",
		steps: []step{
			terminalStep("Instalar Android CLI via curl", "bash", "-o", "pipefail", "-c", androidCLIInstallCommand),
		},
	}
}
