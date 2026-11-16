package main

// O laptop Omarchy oferece somente firewall, sem preparação de diretórios ou links.
func (m model) laptopItems() []item {
	switch m.screen {
	case screenMain:
		return []item{
			{title: "Firewall", desc: "Configurar UFW sem liberar SSH de entrada", goTo: screenFirewall},
			{title: "Sair", desc: "Até a próxima!", quit: true},
		}
	case screenFirewall:
		return m.fwItems()
	default:
		return nil
	}
}
