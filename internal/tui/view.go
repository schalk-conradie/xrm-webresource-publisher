package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

const (
	COLOR_Primary     = lipgloss.Color("33")  // Blue-ish
	COLOR_Secondary   = lipgloss.Color("39")  // Cyan-ish
	COLOR_Accent      = lipgloss.Color("141") // Magenta accent
	COLOR_AccentLight = lipgloss.Color("53")  // Lighter accent for selections
	COLOR_Success     = lipgloss.Color("82")  // Green
	COLOR_Warning     = lipgloss.Color("214") // Orange
	COLOR_Error       = lipgloss.Color("196") // Red
	COLOR_Muted       = lipgloss.Color("244") // Gray
	COLOR_MutedDark   = lipgloss.Color("241") // Darker gray
	COLOR_MutedLight  = lipgloss.Color("252") // Lighter gray
	COLOR_Surface     = lipgloss.Color("236") // Dark panel background
	COLOR_SurfaceAlt  = lipgloss.Color("237") // Alternative dark surface
	COLOR_Border      = lipgloss.Color("240") // Border color
	COLOR_BorderLight = lipgloss.Color("63")  // Lighter border
	COLOR_TextBright  = lipgloss.Color("229") // Bright text
	COLOR_TextDark    = lipgloss.Color("235") // Dark text
	COLOR_TextWhite   = lipgloss.Color("255") // White text
	COLOR_Pink        = lipgloss.Color("205") // Pink/Magenta title
)

var (
	titleStyle = lipgloss.NewStyle().
			Align(lipgloss.Center).
			Bold(true).
			Foreground(COLOR_Pink).
			Background(COLOR_TextDark).
			Padding(0, 1)

	selectedStyle = lipgloss.NewStyle().
			Foreground(COLOR_TextBright).
			Background(COLOR_AccentLight).
			Bold(true)

	normalStyle = lipgloss.NewStyle().
			Foreground(COLOR_MutedLight)

	dimStyle = lipgloss.NewStyle().
			Foreground(COLOR_MutedDark)

	helpStyle = lipgloss.NewStyle().
			Foreground(COLOR_MutedDark).
			Padding(0, 1).
			MaxWidth(80) // Allow text to wrap instead of extending beyond

	boundStyle = lipgloss.NewStyle().
			Foreground(COLOR_Success)

	unboundStyle = lipgloss.NewStyle().
			Foreground(COLOR_MutedDark)

	// Border styles
	mainBorderStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(COLOR_BorderLight).
			Padding(1, 2)

	contentBoxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(COLOR_Border).
			Padding(0, 1)

	// Tab styles
	activeTabStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(COLOR_TextBright).
			Background(COLOR_AccentLight).
			Padding(0, 2)

	inactiveTabStyle = lipgloss.NewStyle().
				Foreground(COLOR_MutedDark).
				Background(COLOR_TextDark).
				Padding(0, 2)

	tabContainerStyle = lipgloss.NewStyle().
				Border(lipgloss.NormalBorder(), false, false, true, false).
				BorderForeground(COLOR_Border).
				MarginBottom(1)

	// Status bar styles (Lualine-inspired)
	statusBarReadyStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(COLOR_TextDark).
				Background(COLOR_Success).
				Padding(0, 1)

	statusBarPublishingStyle = lipgloss.NewStyle().
					Bold(true).
					Foreground(COLOR_TextDark).
					Background(COLOR_Warning).
					Padding(0, 1)

	statusBarErrorStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(COLOR_TextWhite).
				Background(COLOR_Error).
				Padding(0, 1)

	statusBarMessageStyle = lipgloss.NewStyle().
				Foreground(COLOR_MutedLight).
				Background(COLOR_SurfaceAlt).
				Padding(0, 1)

	statusBarCountStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(COLOR_TextDark).
				Background(COLOR_BorderLight).
				Padding(0, 1)

	statusBarContainerStyle = lipgloss.NewStyle().
				MarginTop(1)
)

// renderStatusBar creates a Lualine-style status bar
func (m Model) renderStatusBar(width int) string {
	// Determine status state
	var stateSection string
	isPublishing := false
	for _, publishing := range m.publishing {
		if publishing {
			isPublishing = true
			break
		}
	}

	if m.statusIsError {
		stateSection = statusBarErrorStyle.Render(" ERROR ")
	} else if isPublishing {
		stateSection = statusBarPublishingStyle.Render(" PUBLISHING ")
	} else {
		stateSection = statusBarReadyStyle.Render(" READY ")
	}

	// Count section (resources found)
	var countSection string
	if m.state == StateList && len(m.resources) > 0 {
		countSection = statusBarCountStyle.Render(fmt.Sprintf(" %d resources ", len(m.resources)))
	}

	// Message section (middle)
	message := m.status
	if message == "" {
		message = "Ready"
	}

	// Calculate available space for message
	stateWidth := lipgloss.Width(stateSection)
	countWidth := lipgloss.Width(countSection)
	messageWidth := max(width-stateWidth-countWidth, 10)

	messageSection := statusBarMessageStyle.Width(messageWidth).Render(message)

	// Join sections - this will fill the full width
	statusBar := lipgloss.JoinHorizontal(
		lipgloss.Left,
		stateSection,
		messageSection,
		countSection,
	)

	return statusBar
}

// View renders the UI
func (m Model) View() string {
	// File picker needs full screen - don't wrap in borders
	if m.state == StateFilePicker {
		return m.viewFilePicker()
	}

	var content string
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

	statusBar := m.renderStatusBar(m.width - 12) // Account for main border and padding
	contentWithStatus := lipgloss.JoinVertical(lipgloss.Left, content, "", statusBar)

	// Wrap in main border
	bordered := mainBorderStyle.Render(contentWithStatus)

	// Center the entire UI on screen
	return lipgloss.Place(
		m.width,
		m.height,
		lipgloss.Center,
		lipgloss.Center,
		bordered,
	)
}

func (m Model) viewEnvironmentSelect() string {
	availableWidth := m.width - 12 // Main border (4) + content padding (8)

	// Title
	title := titleStyle.Render("D365 Web Resource Publisher")

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
		inputBox := contentBoxStyle.Width(availableWidth).Render(inputContent.String())
		helpRendered := helpStyle.Width(availableWidth).Render("enter: confirm • esc: cancel")
		return lipgloss.JoinVertical(lipgloss.Left, title, inputBox, helpRendered)
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

	envBox := contentBoxStyle.Width(availableWidth).Render(envContent.String())
	helpRendered := helpStyle.Width(availableWidth).Render("↑/↓: navigate • enter: select • a: add • e: edit • d: delete • c: clear auth • q: quit")

	return lipgloss.JoinVertical(lipgloss.Left, title, envBox, helpRendered)
}

func (m Model) viewAuth() string {
	availableWidth := m.width - 12 // Main border (4) + content padding (8)

	// Title
	title := titleStyle.Render("Authentication Required")

	// Auth content
	var authContent strings.Builder
	authContent.WriteString(m.spinner.View())
	authContent.WriteString(" Opening browser for authentication...\n\n")
	authContent.WriteString("A browser window will open for you to sign in.\n")
	authContent.WriteString("After signing in, you can return to this application.")

	authBox := contentBoxStyle.Width(availableWidth).Render(authContent.String())
	helpRendered := helpStyle.Width(availableWidth).Render("esc: back • q: quit")

	return lipgloss.JoinVertical(lipgloss.Left, title, authBox, helpRendered)
}

func (m Model) viewList() string {
	// Calculate available width accounting for borders and padding
	availableWidth := m.width - 12 // Main border (4) + content padding (8)

	// Title
	env := m.config.GetEnvironment(m.config.CurrentEnvironment)
	var title string
	if env != nil {
		title = titleStyle.Render(fmt.Sprintf("Web Resources - %s", env.Name))
	} else {
		title = titleStyle.Render("Web Resources")
	}

	// Tabs
	tabs := m.renderTabs(availableWidth)

	// Help text based on active tab
	var helpText string
	if m.bindingTab == BindingTabBind {
		helpText = "tab: switch • ↑/↓: navigate • enter: expand/collapse • b: bind • u: unbind • p: publish • a: toggle auto • r: refresh • esc: back • q: quit"
	} else {
		helpText = "tab: switch • ↑/↓: navigate • u: unbind • a: toggle auto • p: publish • esc: back • q: quit"
	}
	helpRendered := helpStyle.Width(availableWidth).Render(helpText)

	// Calculate heights
	titleHeight := lipgloss.Height(title)
	tabsHeight := lipgloss.Height(tabs)
	helpHeight := lipgloss.Height(helpRendered)
	statusBarHeight := 2 // Status bar takes approximately 2 lines (bar itself + margin)

	// Account for main border padding (2 top + 2 bottom), spacing between elements, content box border, and status bar
	fixedHeight := titleHeight + tabsHeight + helpHeight + statusBarHeight + 8 // 2 padding top, 2 padding bottom, 2 for content border, 2 for spacing
	contentHeight := m.height - fixedHeight
	if contentHeight < 5 {
		contentHeight = 5
	}

	// Tab content
	var content string
	switch m.bindingTab {
	case BindingTabBind:
		content = m.viewBindFilesTab(availableWidth, contentHeight)
	case BindingTabList:
		content = m.viewFileListTab(availableWidth, contentHeight)
	}

	// Join all sections
	return lipgloss.JoinVertical(lipgloss.Left, title, tabs, content, helpRendered)
}

func (m Model) renderTabs(width int) string {
	var tabs []string

	// Tab 1: Bind Files
	if m.bindingTab == BindingTabBind {
		tabs = append(tabs, activeTabStyle.Render("Bind Files"))
	} else {
		tabs = append(tabs, inactiveTabStyle.Render("Bind Files"))
	}

	// Tab 2: File List
	if m.bindingTab == BindingTabList {
		tabs = append(tabs, activeTabStyle.Render("File List"))
	} else {
		tabs = append(tabs, inactiveTabStyle.Render("File List"))
	}

	row := lipgloss.JoinHorizontal(lipgloss.Top, tabs...)
	return tabContainerStyle.Width(width).Render(row)
}

func (m Model) viewBindFilesTab(width, height int) string {
	var resourceContent strings.Builder

	if len(m.displayItems) == 0 {
		resourceContent.WriteString(dimStyle.Render("No web resources found"))
	} else {
		// Calculate visible range for scrolling based on actual available height
		visibleLines := height - 2 // Account for content box border
		if visibleLines < 3 {
			visibleLines = 3
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
					status = lipgloss.NewStyle().Foreground(COLOR_Warning).Render(m.spinner.View() + " [publishing]")
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

	return contentBoxStyle.Width(width).Height(height).Render(resourceContent.String())
}

func (m Model) viewFileListTab(width, height int) string {
	var listContent strings.Builder

	// Get all bindings for current environment
	bindings := m.config.GetBindingsForEnvironment(m.config.CurrentEnvironment)

	if len(bindings) == 0 {
		listContent.WriteString(dimStyle.Render("No bound files\n"))
		listContent.WriteString(dimStyle.Render("Switch to 'Bind Files' tab to bind web resources"))
	} else {
		// Calculate visible range for scrolling based on actual available height
		// Each binding takes 2 lines (name + path), plus 1 line spacing = 3 lines per item
		linesPerItem := 3
		visibleItems := (height - 2) / linesPerItem // Account for content box border
		if visibleItems < 1 {
			visibleItems = 1
		}

		start := 0
		if m.bindingSelected >= visibleItems {
			start = m.bindingSelected - visibleItems + 1
		}

		end := start + visibleItems
		if end > len(bindings) {
			end = len(bindings)
		}

		for i := start; i < end; i++ {
			binding := bindings[i]

			// Build the line
			var line strings.Builder
			line.WriteString(binding.WebResourceName)
			line.WriteString("\n  ")
			line.WriteString(dimStyle.Render("→ " + binding.LocalPath))
			line.WriteString("  ")

			// Status indicators
			var status string
			if m.publishing[binding.WebResourceID] {
				status = lipgloss.NewStyle().Foreground(COLOR_Warning).Render(m.spinner.View() + " [publishing]")
			} else if binding.AutoPublish {
				status = boundStyle.Render("[auto]")
			} else {
				status = boundStyle.Render("[bound]")
			}
			line.WriteString(status)

			lineStr := line.String()
			if i == m.bindingSelected {
				listContent.WriteString(selectedStyle.Render("> " + lineStr))
			} else {
				listContent.WriteString(normalStyle.Render("  " + lineStr))
			}

			if i < end-1 {
				listContent.WriteString("\n\n")
			}
		}

		if len(bindings) > visibleItems {
			listContent.WriteString(dimStyle.Render(fmt.Sprintf("\n\n[%d/%d]", m.bindingSelected+1, len(bindings))))
		}
	}

	return contentBoxStyle.Width(width).Height(height).Render(listContent.String())
}

func (m Model) viewBinding() string {
	availableWidth := m.width - 12 // Main border (4) + content padding (8)

	// Title
	title := titleStyle.Render("Bind Web Resource")

	// Binding content
	var bindContent strings.Builder
	if m.resourceSelected < len(m.resources) {
		res := m.resources[m.resourceSelected]
		bindContent.WriteString(fmt.Sprintf("Resource: %s\n\n", res.Name))
	}
	bindContent.WriteString("Local file path:\n")
	bindContent.WriteString(m.textInput.View())

	bindBox := contentBoxStyle.Width(availableWidth).Render(bindContent.String())
	helpRendered := helpStyle.Width(availableWidth).Render("enter: confirm • esc: cancel")

	return lipgloss.JoinVertical(lipgloss.Left, title, bindBox, helpRendered)
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
