package main

func zedJob() job {
	return job{
		title: "Instalar Zed",
		steps: ensureYayPkgs("zed"),
	}
}
