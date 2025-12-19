package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("205")).
			MarginBottom(1)

	selectedStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("229")).
			Background(lipgloss.Color("57")).
			Bold(true)

	normalStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("252"))

	dimStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("241"))

	statusStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("42"))

	errorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("196"))

	helpStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("241")).
			MarginTop(1)

	boundStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("42"))

	unboundStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("241"))
)

// View renders the UI
func (m Model) View() string {
	var b strings.Builder

	switch m.state {
	case StateEnvironmentSelect:
		b.WriteString(m.viewEnvironmentSelect())
	case StateAuth:
		b.WriteString(m.viewAuth())
	case StateList:
		b.WriteString(m.viewList())
	case StateBinding:
		b.WriteString(m.viewBinding())
	case StateFilePicker:
		b.WriteString(m.viewFilePicker())
	}

	// Status bar
	if m.status != "" {
		b.WriteString("\n")
		if m.statusIsError {
			b.WriteString(errorStyle.Render(m.status))
		} else {
			b.WriteString(statusStyle.Render(m.status))
		}
	}

	return b.String()
}

func (m Model) viewEnvironmentSelect() string {
	var b strings.Builder

	b.WriteString(titleStyle.Render("D365 Web Resource Publisher"))
	b.WriteString("\n\n")

	// Input mode
	if m.inputMode != InputNone {
		switch m.inputMode {
		case InputEnvironmentName:
			b.WriteString("Environment Name:\n")
		case InputEnvironmentURL:
			b.WriteString("Environment URL:\n")
		case InputDeleteConfirm:
			if m.envSelected < len(m.config.Environments) {
				b.WriteString(fmt.Sprintf("Delete '%s'? (y/n):\n", m.config.Environments[m.envSelected].Name))
			}
		}
		b.WriteString(m.textInput.View())
		b.WriteString("\n")
		b.WriteString(helpStyle.Render("enter: confirm • esc: cancel"))
		return b.String()
	}

	// Environment list
	if len(m.config.Environments) == 0 {
		b.WriteString(dimStyle.Render("No environments configured"))
		b.WriteString("\n")
		b.WriteString(dimStyle.Render("Press 'a' to add an environment"))
	} else {
		b.WriteString("Select an environment:\n\n")
		for i, env := range m.config.Environments {
			line := fmt.Sprintf("  %s\n  %s", env.Name, dimStyle.Render(env.URL))
			if i == m.envSelected {
				b.WriteString(selectedStyle.Render("> " + line))
			} else {
				b.WriteString(normalStyle.Render("  " + line))
			}
			b.WriteString("\n\n")
		}
	}

	b.WriteString(helpStyle.Render("↑/↓: navigate • enter: select • a: add • e: edit • d: delete • q: quit"))

	return b.String()
}

func (m Model) viewAuth() string {
	var b strings.Builder

	b.WriteString(titleStyle.Render("Authentication Required"))
	b.WriteString("\n\n")

	if m.deviceCode != nil {
		b.WriteString("To sign in, use a web browser to open:\n")
		b.WriteString(lipgloss.NewStyle().Bold(true).Render(m.deviceCode.VerificationURI))
		b.WriteString("\n\n")
		b.WriteString("And enter the code:\n")
		b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("229")).Render(m.deviceCode.UserCode))
		b.WriteString("\n\n")
		b.WriteString(m.spinner.View())
		b.WriteString(" Waiting for authentication...")
	} else {
		b.WriteString(m.spinner.View())
		b.WriteString(" Requesting device code...")
	}

	b.WriteString("\n\n")
	b.WriteString(helpStyle.Render("esc: back • q: quit"))

	return b.String()
}

func (m Model) viewList() string {
	var b strings.Builder

	env := m.config.GetEnvironment(m.config.CurrentEnvironment)
	if env != nil {
		b.WriteString(titleStyle.Render(fmt.Sprintf("Web Resources - %s", env.Name)))
	} else {
		b.WriteString(titleStyle.Render("Web Resources"))
	}
	b.WriteString("\n\n")

	if len(m.displayItems) == 0 {
		b.WriteString(dimStyle.Render("No web resources found"))
		b.WriteString("\n")
	} else {
		// Calculate visible range for scrolling
		visibleLines := m.height - 10
		if visibleLines < 5 {
			visibleLines = 5
		}

		start := 0
		if m.resourceSelected >= visibleLines {
			start = m.resourceSelected - visibleLines + 1
		}

		end := start + visibleLines
		if end > len(m.displayItems) {
			end = len(m.displayItems)
		}

		for i := start; i < end; i++ {
			item := m.displayItems[i]
			node := item.Node

			// Build indent
			indent := strings.Repeat("  ", node.Depth)

			var line string
			if node.IsFolder {
				// Folder with expand/collapse indicator
				if node.Expanded {
					line = fmt.Sprintf("%s▼ %s/", indent, node.Name)
				} else {
					line = fmt.Sprintf("%s▶ %s/", indent, node.Name)
				}
			} else {
				// File with binding status
				res := node.Resource
				binding := m.config.GetBinding(m.config.CurrentEnvironment, res.ID)

				status := unboundStyle.Render("[unbound]")
				if binding != nil {
					if binding.AutoPublish {
						status = boundStyle.Render("[auto]")
					} else {
						status = boundStyle.Render("[bound]")
					}
				}

				line = fmt.Sprintf("%s  %s %s", indent, node.Name, status)
			}

			if i == m.resourceSelected {
				b.WriteString(selectedStyle.Render("> " + line))
			} else {
				b.WriteString(normalStyle.Render("  " + line))
			}
			b.WriteString("\n")
		}

		if len(m.displayItems) > visibleLines {
			b.WriteString(dimStyle.Render(fmt.Sprintf("\n[%d/%d]", m.resourceSelected+1, len(m.displayItems))))
		}
	}

	b.WriteString("\n")
	b.WriteString(helpStyle.Render("↑/↓: navigate • enter: expand/collapse • b: bind • p: publish • a: toggle auto • r: refresh • esc: back • q: quit"))

	return b.String()
}

func (m Model) viewBinding() string {
	var b strings.Builder

	b.WriteString(titleStyle.Render("Bind Web Resource"))
	b.WriteString("\n\n")

	if m.resourceSelected < len(m.resources) {
		res := m.resources[m.resourceSelected]
		b.WriteString(fmt.Sprintf("Resource: %s\n\n", res.Name))
	}

	b.WriteString("Local file path:\n")
	b.WriteString(m.textInput.View())
	b.WriteString("\n\n")
	b.WriteString(helpStyle.Render("enter: confirm • esc: cancel"))

	return b.String()
}

func (m Model) viewFilePicker() string {
	var b strings.Builder

	b.WriteString(titleStyle.Render("Select Local File"))
	b.WriteString("\n\n")

	if m.bindingResource != nil {
		b.WriteString(fmt.Sprintf("Binding: %s\n\n", m.bindingResource.Name))
	}

	b.WriteString(m.filepicker.View())
	b.WriteString("\n")
	b.WriteString(helpStyle.Render("↑/↓: navigate • enter: select • esc: cancel"))

	return b.String()
}
