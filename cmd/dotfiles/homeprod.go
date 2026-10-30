package main

func (m model) homeProdItems() []item {
	if m.screen != screenMain {
		return nil
	}
	return []item{
		{
			title: "Preparação inicial",
			desc:  "Atualizar Ubuntu, criar alsgy e instalar CasaOS; execute como root",
			job: func() job {
				return homeProdBootstrapJob(m.setupDir)
			},
		},
		{title: "Instalar CasaOS", desc: "Executar somente o instalador CasaOS com log; como root", job: func() job {
			return job{title: "Instalar CasaOS", steps: []step{terminalStep("Instalar CasaOS com verificação dos downloads", "bash", configPath(m.setupDir, "casaos", "install.sh"))}}
		}},
		{title: "Configurar SSH", desc: "Colar chave pública de alsgy no terminal; executar como root", job: func() job {
			return job{title: "SSH HomeProd", steps: []step{terminalStep("Autorizar chave e restringir SSH após teste de acesso", "bash", configPath(m.setupDir, "ssh", "configure.sh"))}}
		}},
		{title: "Configurar Swap", desc: "Ativar 8 GiB de swap persistente; executar como root", job: func() job {
			return job{title: "Swap HomeProd", steps: []step{terminalStep("Configurar swap de 8 GiB", "bash", configPath(m.setupDir, "swap", "configure.sh"), "8")}}
		}},
		{title: "HomeProd Tunnel (etapa 1)", desc: "CasaOS via Cloudflare Access; ainda não fecha portas públicas (sudo)", job: func() job {
			return job{title: "HomeProd Tunnel", steps: []step{terminalStep("Instalar conector oficial e configurar Tunnel do HomeProd", "sudo", "bash", configPath(m.setupDir, "tunnel", "configure.sh"))}}
		}},
		{title: "Firewall HomeProd: testar (etapa 2)", desc: "Bloquear entradas públicas por 10 minutos; preservar SSH (sudo)", job: func() job {
			return homeProdFirewallJob(m.setupDir, "apply")
		}},
		{title: "Firewall HomeProd: confirmar", desc: "Após testar NOVA sessão SSH e Access; tornar persistente (sudo)", job: func() job {
			return homeProdFirewallJob(m.setupDir, "confirm")
		}},
		{title: "Firewall HomeProd: reverter", desc: "Reverter teste pendente ao estado anterior (sudo)", job: func() job {
			return homeProdFirewallJob(m.setupDir, "rollback")
		}},
		{title: "Firewall HomeProd: status", desc: "Inspecionar regras e prazo de rollback (sudo)", job: func() job {
			return homeProdFirewallJob(m.setupDir, "status")
		}},
		{title: "Sair", desc: "Até a próxima!", quit: true},
	}
}

func homeProdFirewallJob(setupDir, action string) job {
	return job{
		title: "Firewall HomeProd: " + action,
		steps: []step{terminalStep("Firewall HomeProd: "+action, "sudo", "bash", configPath(setupDir, "firewall", "configure.sh"), action)},
	}
}

func homeProdBootstrapJob(setupDir string) job {
	return job{
		title: "Preparar HomeProd",
		steps: []step{terminalStep(
			"Atualizar Ubuntu, criar administrador alsgy e instalar CasaOS",
			"bash", configPath(setupDir, "bootstrap.sh"), "alsgy",
		)},
		result: func() string {
			return "Ubuntu atualizado; administrador alsgy preparado e CasaOS instalado. Mantenha a sessão root aberta até configurar e testar o SSH do novo usuário."
		},
	}
}
