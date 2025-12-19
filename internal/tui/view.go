package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

var (
	titleStyle = lipgloss.NewStyle().
			Align(lipgloss.Center).
			Bold(true).
			Foreground(lipgloss.Color("205")).
			Background(lipgloss.Color("235")).
			Padding(0, 1)

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
			Padding(0, 1)

	boundStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("42"))

	unboundStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("241"))

	// Border styles
	mainBorderStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("63")).
			Padding(1, 2)

	contentBoxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("240")).
			Padding(0, 1)

	statusBarStyle = lipgloss.NewStyle().
			Border(lipgloss.NormalBorder(), false, false, true, false).
			BorderForeground(lipgloss.Color("240")).
			Padding(0, 1).
			MarginTop(1)
)

// View renders the UI
func (m Model) View() string {
	var content string

	// File picker needs full screen - don't wrap in borders
	if m.state == StateFilePicker {
		return m.viewFilePicker()
	}

	switch m.state {
	case StateEnvironmentSelect:
		content = m.viewEnvironmentSelect()
	case StateAuth:
		content = m.viewAuth()
	case StateList:
		content = m.viewList()
	case StateBinding:
		content = m.viewBinding()
	}

	// Wrap content in main border
	mainContent := mainBorderStyle.Width(m.width - 4).Render(content)

	// Status bar
	var statusBar string
	if m.status != "" {
		if m.statusIsError {
			statusBar = statusBarStyle.Width(m.width - 4).Render(errorStyle.Render("● ") + m.status)
		} else {
			statusBar = statusBarStyle.Width(m.width - 4).Render(statusStyle.Render("● ") + m.status)
		}
	}

	if statusBar != "" {
		return lipgloss.JoinVertical(lipgloss.Left, mainContent, statusBar)
	}
	return mainContent
}

func (m Model) viewEnvironmentSelect() string {
	var sections []string

	// Title
	title := titleStyle.Render("D365 Web Resource Publisher")
	sections = append(sections, title)

	// Input mode
	if m.inputMode != InputNone {
		var inputContent strings.Builder
		switch m.inputMode {
		case InputEnvironmentName:
			inputContent.WriteString("Environment Name:\n")
		case InputEnvironmentURL:
			inputContent.WriteString("Environment URL:\n")
		case InputDeleteConfirm:
			if m.envSelected < len(m.config.Environments) {
				inputContent.WriteString(fmt.Sprintf("Delete '%s'? (y/n):\n", m.config.Environments[m.envSelected].Name))
			}
		}
		inputContent.WriteString(m.textInput.View())
		inputBox := contentBoxStyle.Width(m.width - 12).Render(inputContent.String())
		sections = append(sections, inputBox)
		sections = append(sections, helpStyle.Render("enter: confirm • esc: cancel"))
		return lipgloss.JoinVertical(lipgloss.Left, sections...)
	}

	// Environment list
	var envContent strings.Builder
	if len(m.config.Environments) == 0 {
		envContent.WriteString(dimStyle.Render("No environments configured\n"))
		envContent.WriteString(dimStyle.Render("Press 'a' to add an environment"))
	} else {
		for i, env := range m.config.Environments {
			line := fmt.Sprintf("  %s\n  %s", env.Name, dimStyle.Render(env.URL))
			if i == m.envSelected {
				envContent.WriteString(selectedStyle.Render("> " + line))
			} else {
				envContent.WriteString(normalStyle.Render("  " + line))
			}
			if i < len(m.config.Environments)-1 {
				envContent.WriteString("\n\n")
			}
		}
	}

	envBox := contentBoxStyle.Width(m.width - 12).Render(envContent.String())
	sections = append(sections, envBox)
	sections = append(sections, helpStyle.Render("↑/↓: navigate • enter: select • a: add • e: edit • d: delete • q: quit"))

	return lipgloss.JoinVertical(lipgloss.Left, sections...)
}

func (m Model) viewAuth() string {
	var sections []string

	// Title
	title := titleStyle.Render("Authentication Required")
	sections = append(sections, title)

	// Auth content
	var authContent strings.Builder
	authContent.WriteString(m.spinner.View())
	authContent.WriteString(" Opening browser for authentication...\n\n")
	authContent.WriteString("A browser window will open for you to sign in.\n")
	authContent.WriteString("After signing in, you can return to this application.")

	authBox := contentBoxStyle.Width(m.width - 12).Render(authContent.String())
	sections = append(sections, authBox)
	sections = append(sections, helpStyle.Render("esc: back • q: quit"))

	return lipgloss.JoinVertical(lipgloss.Left, sections...)
}

func (m Model) viewList() string {
	var sections []string

	// Title
	env := m.config.GetEnvironment(m.config.CurrentEnvironment)
	var title string
	if env != nil {
		title = titleStyle.Render(fmt.Sprintf("Web Resources - %s", env.Name))
	} else {
		title = titleStyle.Render("Web Resources")
	}
	sections = append(sections, title)

	// Resources content
	var resourceContent strings.Builder

	if len(m.displayItems) == 0 {
		resourceContent.WriteString(dimStyle.Render("No web resources found"))
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

				// Check if currently publishing
				var status string
				if m.publishing[res.ID] {
					status = lipgloss.NewStyle().Foreground(lipgloss.Color("214")).Render(m.spinner.View() + " [publishing]")
				} else if binding != nil {
					if binding.AutoPublish {
						status = boundStyle.Render("[auto]")
					} else {
						status = boundStyle.Render("[bound]")
					}
				} else {
					status = unboundStyle.Render("[unbound]")
				}

				line = fmt.Sprintf("%s  %s %s", indent, node.Name, status)
			}

			if i == m.resourceSelected {
				resourceContent.WriteString(selectedStyle.Render("> " + line))
			} else {
				resourceContent.WriteString(normalStyle.Render("  " + line))
			}
			resourceContent.WriteString("\n")
		}

		if len(m.displayItems) > visibleLines {
			resourceContent.WriteString(dimStyle.Render(fmt.Sprintf("\n[%d/%d]", m.resourceSelected+1, len(m.displayItems))))
		}
	}

	resourceBox := contentBoxStyle.Width(m.width - 12).Height(m.height - 12).Render(resourceContent.String())
	sections = append(sections, resourceBox)
	sections = append(sections, helpStyle.Render("↑/↓: navigate • enter: expand/collapse • b: bind • u: unbind • p: publish • a: toggle auto • r: refresh • esc: back • q: quit"))

	return lipgloss.JoinVertical(lipgloss.Left, sections...)
}

func (m Model) viewBinding() string {
	var sections []string

	// Title
	title := titleStyle.Render("Bind Web Resource")
	sections = append(sections, title)

	// Binding content
	var bindContent strings.Builder
	if m.resourceSelected < len(m.resources) {
		res := m.resources[m.resourceSelected]
		bindContent.WriteString(fmt.Sprintf("Resource: %s\n\n", res.Name))
	}
	bindContent.WriteString("Local file path:\n")
	bindContent.WriteString(m.textInput.View())

	bindBox := contentBoxStyle.Width(m.width - 12).Render(bindContent.String())
	sections = append(sections, bindBox)
	sections = append(sections, helpStyle.Render("enter: confirm • esc: cancel"))

	return lipgloss.JoinVertical(lipgloss.Left, sections...)
}

func (m Model) viewFilePicker() string {
	var b strings.Builder

	// Simple title
	b.WriteString(titleStyle.Render("Select Local File"))
	b.WriteString("\n\n")

	// Binding info
	if m.bindingResource != nil {
		b.WriteString(fmt.Sprintf("Binding: %s\n\n", m.bindingResource.Name))
	}

	// File picker takes remaining space
	b.WriteString(m.filepicker.View())
	b.WriteString("\n\n")
	b.WriteString(helpStyle.Render("↑/↓: navigate • enter: select • esc: cancel"))

	return b.String()
}
