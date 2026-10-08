package main

const linearCLIInstallCommand = "curl --proto '=https' --tlsv1.2 -LsSf https://github.com/schpet/linear-cli/releases/latest/download/linear-installer.sh | sh"

func linearCLIJob() job {
	return job{
		title: "Instalar Linear CLI",
		steps: []step{
			terminalStep("Instalar Linear CLI via curl", "sh", "-c", linearCLIInstallCommand),
		},
	}
}
