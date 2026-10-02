package main

var driverPackages = []string{"base-devel", "vulkan-tools", "mesa-utils", "linux-headers"}

func driversJob() job {
	return job{
		title: "Instalar drivers e ferramentas gráficas",
		steps: ensurePkgs(driverPackages...),
	}
}
