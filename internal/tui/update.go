package tui

import (
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"codeberg.org/schalkuz/xrm-webresource-publisher/internal/auth"
	"codeberg.org/schalkuz/xrm-webresource-publisher/internal/config"
	"codeberg.org/schalkuz/xrm-webresource-publisher/internal/d365"
	"codeberg.org/schalkuz/xrm-webresource-publisher/internal/watcher"

	"github.com/charmbracelet/bubbles/filepicker"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
)

// Messages
type (
	tokenMsg         *auth.Token
	resourcesMsg     []d365.WebResource
	publishResultMsg struct {
		success    bool
		err        error
		path       string
		resourceID string
	}
	errMsg            error
	statusClearMsg    struct{}
	fileChangeMsg     string
	watcherReadyMsg   *watcher.Watcher
	tokenRefreshedMsg *auth.Token
	reAuthRequiredMsg struct{}
	solutionsMsg      []d365.Solution
	addToSolutionMsg  struct {
		success      bool
		err          error
		solutionName string
		resourceName string
	}
	createResourcesMsg struct {
		success bool
		err     error
		created []string
		failed  []string
	}
	folderFilesMsg []CreateFileInfo
)

// Init initializes the model
func (m Model) Init() tea.Cmd {
	return tea.Batch(m.spinner.Tick)
}

// Update handles messages
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	// Handle file picker states - they need to receive all messages
	if m.state == StateFilePicker {
		return m.handleFilePicker(msg)
	}
	if m.state == StateCreateFilePicker || m.state == StateCreateFolderPicker {
		return m.handleCreateFilePicker(msg)
	}

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		cmds = append(cmds, cmd)

	case tokenMsg:
		m.token = msg
		if env := m.config.GetEnvironment(m.config.CurrentEnvironment); env != nil {
			auth.SaveToken(env.Name, msg)
			m.client = d365.NewClient(env.URL, msg.AccessToken)
			// Set up token refresh callback
			m.setupTokenRefresh()
			m.state = StateList
			return m, m.fetchResources()
		}

	case tokenRefreshedMsg:
		// Token was refreshed automatically
		m.token = msg
		if env := m.config.GetEnvironment(m.config.CurrentEnvironment); env != nil {
			auth.SaveToken(env.Name, msg)
			m.status = "Token refreshed"
			m.statusIsError = false
		}

	case reAuthRequiredMsg:
		// Token refresh failed, need to re-authenticate
		m.status = "Session expired, re-authenticating..."
		m.statusIsError = false
		m.state = StateAuth
		return m, m.authenticateInteractive()

	case resourcesMsg:
		m.resources = msg
		m.buildTree()
		m.status = fmt.Sprintf("Loaded %d web resources", len(msg))
		m.statusIsError = false
		return m, m.setupWatchers()

	case watcherReadyMsg:
		m.watcher = msg
		// Start listening for file changes
		if m.fileChangeChan != nil {
			return m, waitForFileChange(m.fileChangeChan)
		}
		return m, nil

	case publishResultMsg:
		// Remove from publishing map
		if msg.resourceID != "" {
			delete(m.publishing, msg.resourceID)
		}
		if msg.success {
			m.status = fmt.Sprintf("Published: %s", filepath.Base(msg.path))
			m.statusIsError = false
		} else {
			m.status = fmt.Sprintf("Publish failed: %v", msg.err)
			m.statusIsError = true
		}

	case fileChangeMsg:
		// Mark resources as publishing if they have auto-publish enabled
		path := string(msg)
		bindings := m.config.GetBindingsForEnvironment(m.config.CurrentEnvironment)
		for _, b := range bindings {
			absPath, _ := filepath.Abs(b.LocalPath)
			if absPath == path && b.AutoPublish {
				m.publishing[b.WebResourceID] = true
				break
			}
		}
		// Continue listening for more file changes
		return m, tea.Batch(
			m.handleFileChange(path),
			waitForFileChange(m.fileChangeChan),
		)

	case errMsg:
		m.status = fmt.Sprintf("Error: %v", msg)
		m.statusIsError = true
		m.err = msg

	case solutionsMsg:
		m.solutions = msg
		m.loadingSolutions = false
		if len(msg) == 0 {
			m.status = "No solutions found"
			m.statusIsError = true
			m.state = StateList
			m.solutionResource = nil
		}

	case addToSolutionMsg:
		if msg.success {
			m.status = fmt.Sprintf("Added %s to %s", msg.resourceName, msg.solutionName)
			m.statusIsError = false
		} else {
			m.status = fmt.Sprintf("Failed to add to solution: %v", msg.err)
			m.statusIsError = true
		}
		m.state = StateList
		m.solutionResource = nil

	case folderFilesMsg:
		m.createFiles = msg
		if len(msg) == 0 {
			m.status = "No supported files found in folder"
			m.statusIsError = true
			m.state = StateList
		} else {
			m.state = StateCreatePrefixInput
			m.textInput.SetValue(m.createPrefix)
			m.textInput.Placeholder = "e.g., publisher_/AppName/"
		}

	case createResourcesMsg:
		m.creatingResources = false
		if msg.success {
			m.status = fmt.Sprintf("Created %d web resources", len(msg.created))
			m.statusIsError = false
		} else {
			if len(msg.created) > 0 {
				m.status = fmt.Sprintf("Created %d, failed %d: %v", len(msg.created), len(msg.failed), msg.err)
			} else {
				m.status = fmt.Sprintf("Failed to create resources: %v", msg.err)
			}
			m.statusIsError = true
		}
		m.state = StateList
		m.createSolution = nil
		m.createFiles = nil
		// Refresh the resource list
		return m, m.fetchResources()
	}

	return m, tea.Batch(cmds...)
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Handle text input modes
	if m.inputMode != InputNone {
		return m.handleInputMode(msg)
	}

	switch m.state {
	case StateEnvironmentSelect:
		return m.handleEnvSelectKey(msg)
	case StateAuth:
		return m.handleAuthKey(msg)
	case StateList:
		return m.handleListKey(msg)
	case StateBinding:
		return m.handleBindingKey(msg)
	case StateFilePicker:
		return m.handleFilePickerKey(msg)
	case StateSolutionPicker:
		return m.handleSolutionPickerKey(msg)
	case StateCreateModeSelect:
		return m.handleCreateModeSelectKey(msg)
	case StateCreateFilePicker, StateCreateFolderPicker:
		return m.handleCreateFilePickerKey(msg)
	case StateCreateNameInput:
		return m.handleCreateNameInputKey(msg)
	case StateCreatePrefixInput:
		return m.handleCreatePrefixInputKey(msg)
	case StateCreateConfirm:
		return m.handleCreateConfirmKey(msg)
	}

	return m, nil
}

func (m Model) handleInputMode(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.inputMode = InputNone
		m.textInput.SetValue("")
		m.editingEnvName = ""
		return m, nil

	case "enter":
		value := strings.TrimSpace(m.textInput.Value())
		m.textInput.SetValue("")

		switch m.inputMode {
		case InputEnvironmentName:
			if value != "" {
				if m.editingEnvName != "" {
					// Editing existing - check if this env exists
					existing := m.config.GetEnvironment(m.editingEnvName)
					if existing != nil {
						// Store new name temporarily, move to URL input
						m.inputMode = InputEnvironmentURL
						m.textInput.SetValue(existing.URL)
						m.textInput.Placeholder = "Environment URL"
						// Store new name in editingEnvName for later use
						m.editingEnvName = m.editingEnvName + "|" + value
					}
				} else {
					// Adding new
					m.editingEnvName = value
					m.inputMode = InputEnvironmentURL
					m.textInput.Placeholder = "https://org.crm.dynamics.com"
				}
			} else {
				m.inputMode = InputNone
			}
			return m, nil

		case InputEnvironmentURL:
			if value != "" {
				var err error
				if strings.Contains(m.editingEnvName, "|") {
					// Editing existing environment
					parts := strings.SplitN(m.editingEnvName, "|", 2)
					oldName := parts[0]
					newName := parts[1]
					err = m.config.UpdateEnvironment(oldName, newName, value)
				} else if m.editingEnvName != "" {
					// Adding new environment
					err = m.config.AddEnvironment(m.editingEnvName, value)
				}
				if err != nil {
					m.status = err.Error()
					m.statusIsError = true
				} else {
					m.status = "Environment saved"
					m.statusIsError = false
				}
			}
			m.inputMode = InputNone
			m.editingEnvName = ""
			return m, nil

		case InputBindingPath:
			if value != "" && m.resourceSelected < len(m.resources) {
				res := m.resources[m.resourceSelected]
				binding := config.Binding{
					Environment:      m.config.CurrentEnvironment,
					LocalPath:        value,
					WebResourceName:  res.Name,
					WebResourceID:    res.ID,
					LastKnownVersion: "1.0.0",
					AutoPublish:      true,
				}
				if err := m.config.AddBinding(binding); err != nil {
					m.status = fmt.Sprintf("Failed to save binding: %v", err)
					m.statusIsError = true
				} else {
					m.status = fmt.Sprintf("Bound %s to %s", res.Name, value)
					m.statusIsError = false
					// Add to watcher
					if m.watcher != nil {
						absPath, _ := filepath.Abs(value)
						m.watcher.AddFile(absPath)
					}
				}
			}
			m.inputMode = InputNone
			m.state = StateList
			return m, nil

		case InputDeleteConfirm:
			if strings.ToLower(value) == "y" && m.envSelected < len(m.config.Environments) {
				env := m.config.Environments[m.envSelected]
				if err := m.config.DeleteEnvironment(env.Name); err != nil {
					m.status = fmt.Sprintf("Failed to delete: %v", err)
					m.statusIsError = true
				} else {
					m.status = "Environment deleted"
					m.statusIsError = false
					if m.envSelected >= len(m.config.Environments) && m.envSelected > 0 {
						m.envSelected--
					}
				}
			}
			m.inputMode = InputNone
			return m, nil
		}
	}

	var cmd tea.Cmd
	m.textInput, cmd = m.textInput.Update(msg)
	return m, cmd
}

func (m Model) handleEnvSelectKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit

	case "up", "k":
		if m.envSelected > 0 {
			m.envSelected--
		}

	case "down", "j":
		if m.envSelected < len(m.config.Environments)-1 {
			m.envSelected++
		}

	case "a":
		m.inputMode = InputEnvironmentName
		m.textInput.Placeholder = "Environment name"
		m.textInput.SetValue("")
		m.editingEnvName = ""
		return m, nil

	case "e":
		if m.envSelected < len(m.config.Environments) {
			env := m.config.Environments[m.envSelected]
			m.editingEnvName = env.Name
			m.inputMode = InputEnvironmentName
			m.textInput.Placeholder = "Environment name"
			m.textInput.SetValue(env.Name)
		}
		return m, nil

	case "d":
		if m.envSelected < len(m.config.Environments) {
			m.inputMode = InputDeleteConfirm
			m.textInput.Placeholder = "Delete? (y/n)"
			m.textInput.SetValue("")
		}
		return m, nil

	case "c":
		if m.envSelected < len(m.config.Environments) {
			env := m.config.Environments[m.envSelected]
			if err := auth.DeleteToken(env.Name); err != nil {
				m.status = fmt.Sprintf("Failed to clear auth: %v", err)
				m.statusIsError = true
			} else {
				m.status = fmt.Sprintf("Cleared auth for %s", env.Name)
				m.statusIsError = false
			}
		}
		return m, nil

	case "enter":
		if m.envSelected < len(m.config.Environments) {
			env := m.config.Environments[m.envSelected]
			m.config.CurrentEnvironment = env.Name
			m.config.Save()

			// Try to load existing token
			if token, err := auth.LoadToken(env.Name); err == nil && !token.IsExpired() {
				m.token = token
				m.client = d365.NewClient(env.URL, token.AccessToken)
				m.state = StateList
				return m, m.fetchResources()
			}

			// Need to authenticate
			m.state = StateAuth
			return m, m.authenticateInteractive()
		}
	}

	return m, nil
}

func (m Model) handleAuthKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "esc":
		m.state = StateEnvironmentSelect
	}
	return m, nil
}

func (m Model) handleListKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit

	case "esc":
		if m.watcher != nil {
			m.watcher.Clear()
		}
		m.state = StateEnvironmentSelect
		m.resources = nil
		m.displayItems = nil
		m.treeRoot = nil
		return m, nil

	case "tab":
		// Switch between tabs
		if m.bindingTab == BindingTabBind {
			m.bindingTab = BindingTabList
			m.bindingSelected = 0
		} else {
			m.bindingTab = BindingTabBind
			m.resourceSelected = 0
		}
		return m, nil

	case "up", "k":
		if m.bindingTab == BindingTabBind {
			if m.resourceSelected > 0 {
				m.resourceSelected--
			}
		} else {
			if m.bindingSelected > 0 {
				m.bindingSelected--
			}
		}

	case "down", "j":
		if m.bindingTab == BindingTabBind {
			if m.resourceSelected < len(m.displayItems)-1 {
				m.resourceSelected++
			}
		} else {
			bindings := m.config.GetBindingsForEnvironment(m.config.CurrentEnvironment)
			if m.bindingSelected < len(bindings)-1 {
				m.bindingSelected++
			}
		}

	case "enter":
		if m.bindingTab == BindingTabBind {
			if m.resourceSelected < len(m.displayItems) {
				item := m.displayItems[m.resourceSelected]
				if item.Node.IsFolder {
					m.toggleFolder(item.Node.FullPath)
				}
			}
		}
		return m, nil

	case "b":
		// Only available in Bind Files tab
		if m.bindingTab == BindingTabBind {
			if m.resourceSelected < len(m.displayItems) {
				item := m.displayItems[m.resourceSelected]
				if !item.Node.IsFolder && item.Resource != nil {
					// Initialize file picker
					fp := filepicker.New()
					// fp.DirAllowed = false
					fp.CurrentDirectory, _ = os.UserHomeDir()
					fp.Height = m.height - 6
					m.filepicker = fp
					m.bindingResource = item.Resource
					m.state = StateFilePicker
					return m, m.filepicker.Init()
				} else {
					m.status = "Select a file to bind"
					m.statusIsError = true
				}
			}
		}
		return m, nil

	case "p":
		if m.bindingTab == BindingTabBind {
			if m.resourceSelected < len(m.displayItems) {
				item := m.displayItems[m.resourceSelected]
				if !item.Node.IsFolder && item.Resource != nil {
					// Mark as publishing
					m.publishing[item.Resource.ID] = true
					return m, m.publishResource(*item.Resource)
				} else {
					m.status = "Select a file to publish"
					m.statusIsError = true
				}
			}
		} else {
			// In File List tab
			bindings := m.config.GetBindingsForEnvironment(m.config.CurrentEnvironment)
			if m.bindingSelected < len(bindings) {
				binding := bindings[m.bindingSelected]
				// Find the resource
				for _, res := range m.resources {
					if res.ID == binding.WebResourceID {
						m.publishing[res.ID] = true
						return m, m.publishResource(res)
					}
				}
			}
		}

	case "a":
		if m.bindingTab == BindingTabBind {
			if m.resourceSelected < len(m.displayItems) {
				item := m.displayItems[m.resourceSelected]
				if !item.Node.IsFolder && item.Resource != nil {
					res := item.Resource
					if b := m.config.GetBinding(m.config.CurrentEnvironment, res.ID); b != nil {
						b.AutoPublish = !b.AutoPublish
						m.config.AddBinding(*b)
						if b.AutoPublish {
							m.status = "Auto-publish enabled"
						} else {
							m.status = "Auto-publish disabled"
						}
						m.statusIsError = false
					} else {
						m.status = "Bind a file first"
						m.statusIsError = true
					}
				} else {
					m.status = "Select a file to toggle auto-publish"
					m.statusIsError = true
				}
			}
		} else {
			// In File List tab
			bindings := m.config.GetBindingsForEnvironment(m.config.CurrentEnvironment)
			if m.bindingSelected < len(bindings) {
				binding := bindings[m.bindingSelected]
				binding.AutoPublish = !binding.AutoPublish
				m.config.AddBinding(binding)
				if binding.AutoPublish {
					m.status = "Auto-publish enabled"
				} else {
					m.status = "Auto-publish disabled"
				}
				m.statusIsError = false
			}
		}

	case "u":
		if m.bindingTab == BindingTabBind {
			if m.resourceSelected < len(m.displayItems) {
				item := m.displayItems[m.resourceSelected]
				if !item.Node.IsFolder && item.Resource != nil {
					res := item.Resource
					if b := m.config.GetBinding(m.config.CurrentEnvironment, res.ID); b != nil {
						// Remove from watcher if it was being watched
						if m.watcher != nil && b.AutoPublish {
							absPath, _ := filepath.Abs(b.LocalPath)
							m.watcher.RemoveFile(absPath)
						}
						// Delete the binding
						if err := m.config.DeleteBinding(m.config.CurrentEnvironment, res.ID); err != nil {
							m.status = fmt.Sprintf("Failed to unbind: %v", err)
							m.statusIsError = true
						} else {
							m.status = fmt.Sprintf("Unbound %s", res.Name)
							m.statusIsError = false
						}
					} else {
						m.status = "File is not bound"
						m.statusIsError = true
					}
				} else {
					m.status = "Select a file to unbind"
					m.statusIsError = true
				}
			}
		} else {
			// In File List tab
			bindings := m.config.GetBindingsForEnvironment(m.config.CurrentEnvironment)
			if m.bindingSelected < len(bindings) {
				binding := bindings[m.bindingSelected]
				// Remove from watcher if it was being watched
				if m.watcher != nil && binding.AutoPublish {
					absPath, _ := filepath.Abs(binding.LocalPath)
					m.watcher.RemoveFile(absPath)
				}
				// Delete the binding
				if err := m.config.DeleteBinding(m.config.CurrentEnvironment, binding.WebResourceID); err != nil {
					m.status = fmt.Sprintf("Failed to unbind: %v", err)
					m.statusIsError = true
				} else {
					m.status = fmt.Sprintf("Unbound %s", binding.WebResourceName)
					m.statusIsError = false
					// Adjust selection if needed
					if m.bindingSelected >= len(bindings)-1 && m.bindingSelected > 0 {
						m.bindingSelected--
					}
				}
			}
		}

	case "r":
		return m, m.fetchResources()

	case "l":
		// Manual re-authentication
		m.status = "Re-authenticating..."
		m.statusIsError = false
		m.state = StateAuth
		return m, m.authenticateInteractive()

	case "s":
		// Add to solution - get the selected resource
		var selectedResource *d365.WebResource
		if m.bindingTab == BindingTabBind {
			if m.resourceSelected < len(m.displayItems) {
				item := m.displayItems[m.resourceSelected]
				if !item.Node.IsFolder && item.Resource != nil {
					selectedResource = item.Resource
				}
			}
		} else {
			// In File List tab
			bindings := m.config.GetBindingsForEnvironment(m.config.CurrentEnvironment)
			if m.bindingSelected < len(bindings) {
				binding := bindings[m.bindingSelected]
				// Find the resource
				for i := range m.resources {
					if m.resources[i].ID == binding.WebResourceID {
						selectedResource = &m.resources[i]
						break
					}
				}
			}
		}

		if selectedResource != nil {
			m.solutionResource = selectedResource
			m.solutionSelected = 0
			m.loadingSolutions = true
			m.state = StateSolutionPicker
			return m, m.fetchSolutions()
		} else {
			m.status = "Select a file first"
			m.statusIsError = true
		}

	case "N":
		// Create new web resource - first select solution
		m.solutionSelected = 0
		m.loadingSolutions = true
		m.createSolution = nil
		m.createFiles = nil
		m.createMode = CreateModeSingleFile
		m.createModeSelected = 0
		m.state = StateSolutionPicker
		return m, m.fetchSolutions()
	}

	return m, nil
}

func (m Model) handleBindingKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if msg.String() == "esc" {
		m.inputMode = InputNone
		m.state = StateList
		return m, nil
	}
	return m.handleInputMode(msg)
}

func (m Model) handleFilePickerKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if msg.String() == "esc" {
		m.state = StateList
		m.bindingResource = nil
		return m, nil
	}

	var cmd tea.Cmd
	m.filepicker, cmd = m.filepicker.Update(msg)

	// Check if a file was selected
	if didSelect, path := m.filepicker.DidSelectFile(msg); didSelect {
		if m.bindingResource != nil {
			binding := config.Binding{
				Environment:      m.config.CurrentEnvironment,
				LocalPath:        path,
				WebResourceName:  m.bindingResource.Name,
				WebResourceID:    m.bindingResource.ID,
				LastKnownVersion: "1.0.0",
				AutoPublish:      true,
			}
			if err := m.config.AddBinding(binding); err != nil {
				m.status = fmt.Sprintf("Failed to save binding: %v", err)
				m.statusIsError = true
			} else {
				m.status = fmt.Sprintf("Bound %s to %s", m.bindingResource.Name, filepath.Base(path))
				m.statusIsError = false
				// Add to watcher
				if m.watcher != nil {
					m.watcher.AddFile(path)
				}
			}
		}
		m.state = StateList
		m.bindingResource = nil
		return m, nil
	}

	return m, cmd
}

func (m Model) handleFilePicker(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Handle escape key
	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		if keyMsg.String() == "esc" || keyMsg.String() == "q" || keyMsg.String() == "ctrl+c" {
			m.state = StateList
			m.bindingResource = nil
			return m, nil
		}
	}

	// Handle window resize
	if wsMsg, ok := msg.(tea.WindowSizeMsg); ok {
		m.width = wsMsg.Width
		m.height = wsMsg.Height
		m.filepicker.Height = wsMsg.Height - 6
	}

	// Update the filepicker with all messages
	var cmd tea.Cmd
	m.filepicker, cmd = m.filepicker.Update(msg)

	// Check if a file was selected
	if didSelect, path := m.filepicker.DidSelectFile(msg); didSelect {
		if m.bindingResource != nil {
			binding := config.Binding{
				Environment:      m.config.CurrentEnvironment,
				LocalPath:        path,
				WebResourceName:  m.bindingResource.Name,
				WebResourceID:    m.bindingResource.ID,
				LastKnownVersion: "1.0.0",
				AutoPublish:      true,
			}
			if err := m.config.AddBinding(binding); err != nil {
				m.status = fmt.Sprintf("Failed to save binding: %v", err)
				m.statusIsError = true
			} else {
				m.status = fmt.Sprintf("Bound %s to %s", m.bindingResource.Name, filepath.Base(path))
				m.statusIsError = false
				// Add to watcher
				if m.watcher != nil {
					m.watcher.AddFile(path)
				}
			}
		}
		m.state = StateList
		m.bindingResource = nil
		return m, nil
	}

	return m, cmd
}

func (m Model) handleCreateFilePicker(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Handle folderFilesMsg - this comes back after scanning a folder
	if filesMsg, ok := msg.(folderFilesMsg); ok {
		m.createFiles = filesMsg
		// Store original list for reset functionality
		m.createFilesOriginal = make([]CreateFileInfo, len(filesMsg))
		copy(m.createFilesOriginal, filesMsg)
		if len(filesMsg) == 0 {
			m.status = "No supported files found in folder"
			m.statusIsError = true
			m.state = StateCreateModeSelect
		} else {
			m.state = StateCreatePrefixInput
			m.textInput.SetValue(m.createPrefix)
			m.textInput.Placeholder = "e.g., publisher_/AppName/"
			m.textInput.Focus()
		}
		return m, nil
	}

	// Handle key messages
	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		if keyMsg.String() == "esc" {
			m.state = StateCreateModeSelect
			return m, nil
		}

		// For folder picker: 's' or space selects the current directory
		if m.state == StateCreateFolderPicker {
			if keyMsg.String() == "s" || keyMsg.String() == " " {
				// Select the current directory
				currentDir := m.filepicker.CurrentDirectory
				return m, m.scanFolderForFiles(currentDir)
			}
		}
	}

	// Handle window resize
	if wsMsg, ok := msg.(tea.WindowSizeMsg); ok {
		m.width = wsMsg.Width
		m.height = wsMsg.Height
		m.filepicker.Height = wsMsg.Height - 6
	}

	// Update the filepicker with all messages
	var cmd tea.Cmd
	m.filepicker, cmd = m.filepicker.Update(msg)

	if m.state == StateCreateFilePicker {
		// Single file selection
		if didSelect, path := m.filepicker.DidSelectFile(msg); didSelect {
			// Validate file type
			resourceType, err := d365.GetWebResourceTypeFromExtension(path)
			if err != nil {
				m.status = err.Error()
				m.statusIsError = true
				return m, nil
			}

			m.createFiles = []CreateFileInfo{{
				LocalPath:    path,
				ResourceType: resourceType,
			}}
			m.state = StateCreateNameInput
			m.textInput.SetValue("")
			m.textInput.Placeholder = "e.g., publisher_/folder/filename.js"
			return m, nil
		}
	}

	return m, cmd
}

// Commands
func (m Model) authenticateInteractive() tea.Cmd {
	return func() tea.Msg {
		env := m.config.GetEnvironment(m.config.CurrentEnvironment)
		if env == nil {
			return errMsg(fmt.Errorf("environment not found"))
		}
		token, err := auth.AcquireTokenInteractive(env.URL)
		if err != nil {
			return errMsg(err)
		}
		return tokenMsg(token)
	}
}

func (m Model) fetchResources() tea.Cmd {
	return func() tea.Msg {
		if m.client == nil {
			return errMsg(fmt.Errorf("not connected"))
		}
		resources, err := m.client.ListWebResources()
		if err != nil {
			return errMsg(err)
		}
		return resourcesMsg(resources)
	}
}

func (m Model) setupWatchers() tea.Cmd {
	cfg := m.config
	fileChangeChan := m.fileChangeChan

	return func() tea.Msg {
		w, err := watcher.New(func(path string) {
			// Send file change notification through channel
			if fileChangeChan != nil {
				select {
				case fileChangeChan <- path:
				default:
					// Channel full, skip
				}
			}
		})
		if err != nil {
			return errMsg(err)
		}

		bindings := cfg.GetBindingsForEnvironment(cfg.CurrentEnvironment)
		for _, b := range bindings {
			if b.AutoPublish {
				absPath, err := filepath.Abs(b.LocalPath)
				if err == nil {
					w.AddFile(absPath)
				}
			}
		}

		return watcherReadyMsg(w)
	}
}

// waitForFileChange is a subscription that waits for file changes
func waitForFileChange(fileChangeChan chan string) tea.Cmd {
	return func() tea.Msg {
		path := <-fileChangeChan
		return fileChangeMsg(path)
	}
}

func (m Model) publishResource(res d365.WebResource) tea.Cmd {
	cfg := m.config
	client := m.client

	return func() tea.Msg {
		binding := cfg.GetBinding(cfg.CurrentEnvironment, res.ID)
		if binding == nil {
			return errMsg(fmt.Errorf("no binding for this resource"))
		}

		content, err := os.ReadFile(binding.LocalPath)
		if err != nil {
			return publishResultMsg{success: false, err: err, path: binding.LocalPath, resourceID: res.ID}
		}

		encoded := base64.StdEncoding.EncodeToString(content)

		if err := client.UpdateWebResourceContent(res.ID, encoded); err != nil {
			return publishResultMsg{success: false, err: err, path: binding.LocalPath, resourceID: res.ID}
		}

		if err := client.PublishWebResource(res.ID); err != nil {
			return publishResultMsg{success: false, err: err, path: binding.LocalPath, resourceID: res.ID}
		}

		// Increment version
		newVersion := incrementVersion(binding.LastKnownVersion)
		cfg.UpdateBindingVersion(cfg.CurrentEnvironment, res.ID, newVersion)

		return publishResultMsg{success: true, path: binding.LocalPath, resourceID: res.ID}
	}
}

func (m Model) handleFileChange(path string) tea.Cmd {
	cfg := m.config
	client := m.client
	resources := m.resources

	return func() tea.Msg {
		bindings := cfg.GetBindingsForEnvironment(cfg.CurrentEnvironment)
		for _, b := range bindings {
			absPath, _ := filepath.Abs(b.LocalPath)
			if absPath == path && b.AutoPublish {
				// Find the resource and publish
				for _, res := range resources {
					if res.ID == b.WebResourceID {
						// Retry reading file to handle atomic saves
						// Editors like Neovim delete the original and rename a temp file
						var content []byte
						var err error
						for attempt := 0; attempt < 5; attempt++ {
							if attempt > 0 {
								time.Sleep(50 * time.Millisecond)
							}
							content, err = os.ReadFile(b.LocalPath)
							if err == nil {
								break
							}
						}
						if err != nil {
							return publishResultMsg{
								success:    false,
								err:        fmt.Errorf("reading %s: %w", b.LocalPath, err),
								path:       b.LocalPath,
								resourceID: res.ID,
							}
						}

						encoded := base64.StdEncoding.EncodeToString(content)

						if err := client.UpdateWebResourceContent(res.ID, encoded); err != nil {
							return publishResultMsg{success: false, err: err, path: b.LocalPath, resourceID: res.ID}
						}

						if err := client.PublishWebResource(res.ID); err != nil {
							return publishResultMsg{success: false, err: err, path: b.LocalPath, resourceID: res.ID}
						}

						newVersion := incrementVersion(b.LastKnownVersion)
						cfg.UpdateBindingVersion(cfg.CurrentEnvironment, res.ID, newVersion)

						return publishResultMsg{success: true, path: b.LocalPath, resourceID: res.ID}
					}
				}
			}
		}
		return nil
	}
}

func incrementVersion(version string) string {
	parts := strings.Split(version, ".")
	if len(parts) != 3 {
		return "1.0.1"
	}

	minor, err := strconv.Atoi(parts[2])
	if err != nil {
		return "1.0.1"
	}

	parts[2] = strconv.Itoa(minor + 1)
	return strings.Join(parts, ".")
}

// setupTokenRefresh configures the client's token refresh callback
func (m *Model) setupTokenRefresh() {
	if m.client == nil {
		return
	}

	env := m.config.GetEnvironment(m.config.CurrentEnvironment)
	if env == nil {
		return
	}

	orgURL := env.URL
	envName := env.Name

	m.client.SetTokenRefreshFunc(func() (string, error) {
		// Try to refresh the token silently
		newToken, err := auth.RefreshAccessToken("", orgURL)
		if err != nil {
			return "", err
		}

		// Save the new token
		auth.SaveToken(envName, newToken)

		return newToken.AccessToken, nil
	})
}

func (m Model) handleSolutionPickerKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit

	case "esc":
		m.state = StateList
		m.solutionResource = nil
		m.createSolution = nil
		m.solutions = nil
		return m, nil

	case "up", "k":
		if m.solutionSelected > 0 {
			m.solutionSelected--
		}

	case "down", "j":
		if m.solutionSelected < len(m.solutions)-1 {
			m.solutionSelected++
		}

	case "enter":
		if m.solutionSelected < len(m.solutions) {
			solution := m.solutions[m.solutionSelected]

			// Check if this is for adding existing resource or creating new
			if m.solutionResource != nil {
				// Adding existing resource to solution
				resource := m.solutionResource
				m.status = fmt.Sprintf("Adding %s to %s...", resource.Name, solution.FriendlyName)
				m.statusIsError = false
				return m, m.addToSolution(solution, *resource)
			} else {
				// Creating new web resource - move to mode selection
				m.createSolution = &solution
				m.state = StateCreateModeSelect
				m.createModeSelected = 0
				return m, nil
			}
		}
	}

	return m, nil
}

func (m Model) fetchSolutions() tea.Cmd {
	client := m.client

	return func() tea.Msg {
		if client == nil {
			return errMsg(fmt.Errorf("not connected"))
		}
		solutions, err := client.ListSolutions()
		if err != nil {
			return errMsg(err)
		}
		return solutionsMsg(solutions)
	}
}

func (m Model) addToSolution(solution d365.Solution, resource d365.WebResource) tea.Cmd {
	client := m.client

	return func() tea.Msg {
		if client == nil {
			return errMsg(fmt.Errorf("not connected"))
		}
		err := client.AddWebResourceToSolution(solution.UniqueName, resource.ID)
		if err != nil {
			return addToSolutionMsg{
				success:      false,
				err:          err,
				solutionName: solution.FriendlyName,
				resourceName: resource.Name,
			}
		}
		return addToSolutionMsg{
			success:      true,
			solutionName: solution.FriendlyName,
			resourceName: resource.Name,
		}
	}
}

func (m Model) handleCreateModeSelectKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit

	case "esc":
		m.state = StateList
		m.createSolution = nil
		return m, nil

	case "up", "k":
		if m.createModeSelected > 0 {
			m.createModeSelected--
		}

	case "down", "j":
		if m.createModeSelected < 1 {
			m.createModeSelected++
		}

	case "enter":
		if m.createModeSelected == 0 {
			// Single file mode
			m.createMode = CreateModeSingleFile
			fp := filepicker.New()
			fp.CurrentDirectory, _ = os.UserHomeDir()
			fp.Height = m.height - 6
			m.filepicker = fp
			m.state = StateCreateFilePicker
			return m, m.filepicker.Init()
		} else {
			// Folder mode
			m.createMode = CreateModeFolder
			fp := filepicker.New()
			fp.CurrentDirectory, _ = os.UserHomeDir()
			fp.DirAllowed = true
			fp.FileAllowed = false
			fp.Height = m.height - 6
			m.filepicker = fp
			m.state = StateCreateFolderPicker
			return m, m.filepicker.Init()
		}
	}

	return m, nil
}

func (m Model) handleCreateFilePickerKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if msg.String() == "esc" {
		m.state = StateCreateModeSelect
		return m, nil
	}

	var cmd tea.Cmd
	m.filepicker, cmd = m.filepicker.Update(msg)

	if m.state == StateCreateFilePicker {
		// Single file selection
		if didSelect, path := m.filepicker.DidSelectFile(msg); didSelect {
			// Validate file type
			resourceType, err := d365.GetWebResourceTypeFromExtension(path)
			if err != nil {
				m.status = err.Error()
				m.statusIsError = true
				return m, nil
			}

			m.createFiles = []CreateFileInfo{{
				LocalPath:    path,
				ResourceType: resourceType,
			}}
			m.state = StateCreateNameInput
			m.textInput.SetValue("")
			m.textInput.Placeholder = "e.g., publisher_/folder/filename.js"
			return m, nil
		}
	} else {
		// Folder selection
		if didSelect, path := m.filepicker.DidSelectFile(msg); didSelect {
			// Scan folder for files
			return m, m.scanFolderForFiles(path)
		}
	}

	return m, cmd
}

func (m Model) handleCreateNameInputKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.textInput.Blur()
		m.state = StateCreateFilePicker
		m.textInput.SetValue("")
		return m, m.filepicker.Init()

	case "enter":
		name := strings.TrimSpace(m.textInput.Value())
		if name == "" {
			m.status = "Name cannot be empty"
			m.statusIsError = true
			return m, nil
		}

		m.textInput.Blur()
		// Update the file info with the name
		if len(m.createFiles) > 0 {
			m.createFiles[0].WebResName = name
		}
		m.state = StateCreateConfirm
		m.createFileSelected = 0
		return m, nil
	}

	var cmd tea.Cmd
	m.textInput, cmd = m.textInput.Update(msg)
	return m, cmd
}

func (m Model) handleCreatePrefixInputKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.textInput.Blur()
		m.state = StateCreateFolderPicker
		m.textInput.SetValue("")
		return m, m.filepicker.Init()

	case "enter":
		prefix := strings.TrimSpace(m.textInput.Value())
		m.createPrefix = prefix
		m.textInput.Blur()

		// Apply prefix to all files
		for i := range m.createFiles {
			m.createFiles[i].WebResName = prefix + m.createFiles[i].WebResName
		}
		m.state = StateCreateConfirm
		m.createFileSelected = 0
		return m, nil
	}

	var cmd tea.Cmd
	m.textInput, cmd = m.textInput.Update(msg)
	return m, cmd
}

func (m Model) handleCreateConfirmKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit

	case "esc":
		// Go back based on mode
		if m.createMode == CreateModeSingleFile {
			m.state = StateCreateNameInput
			m.textInput.SetValue(m.createFiles[0].WebResName)
			m.textInput.Focus()
		} else {
			// Restore original files and go back to prefix input
			m.createFiles = make([]CreateFileInfo, len(m.createFilesOriginal))
			copy(m.createFiles, m.createFilesOriginal)
			m.state = StateCreatePrefixInput
			m.textInput.SetValue(m.createPrefix)
			m.textInput.Focus()
		}
		return m, nil

	case "r":
		// Reset file list to original (folder mode only)
		if m.createMode == CreateModeFolder && len(m.createFilesOriginal) > 0 {
			m.createFiles = make([]CreateFileInfo, len(m.createFilesOriginal))
			copy(m.createFiles, m.createFilesOriginal)
			// Re-apply prefix
			for i := range m.createFiles {
				m.createFiles[i].WebResName = m.createPrefix + m.createFiles[i].WebResName
			}
			m.createFileSelected = 0
			m.status = "File list reset"
			m.statusIsError = false
		}

	case "up", "k":
		if m.createFileSelected > 0 {
			m.createFileSelected--
		}

	case "down", "j":
		if m.createFileSelected < len(m.createFiles)-1 {
			m.createFileSelected++
		}

	case "d", "delete", "backspace":
		// Remove selected file from list (only for folder mode with multiple files)
		if m.createMode == CreateModeFolder && len(m.createFiles) > 1 {
			m.createFiles = append(m.createFiles[:m.createFileSelected], m.createFiles[m.createFileSelected+1:]...)
			if m.createFileSelected >= len(m.createFiles) {
				m.createFileSelected = len(m.createFiles) - 1
			}
		}

	case "enter", "y":
		// Create all the resources
		m.creatingResources = true
		m.status = "Creating web resources..."
		m.statusIsError = false
		return m, m.createWebResources()
	}

	return m, nil
}

func (m Model) scanFolderForFiles(folderPath string) tea.Cmd {
	return func() tea.Msg {
		var files []CreateFileInfo

		err := filepath.Walk(folderPath, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() {
				return nil
			}

			resourceType, err := d365.GetWebResourceTypeFromExtension(path)
			if err != nil {
				// Skip unsupported files
				return nil
			}

			// Get relative path from folder
			relPath, _ := filepath.Rel(folderPath, path)
			// Convert to forward slashes for web resource naming
			relPath = strings.ReplaceAll(relPath, string(os.PathSeparator), "/")

			files = append(files, CreateFileInfo{
				LocalPath:    path,
				WebResName:   relPath,
				ResourceType: resourceType,
			})

			return nil
		})

		if err != nil {
			return errMsg(err)
		}

		return folderFilesMsg(files)
	}
}

func (m Model) createWebResources() tea.Cmd {
	client := m.client
	solution := m.createSolution
	files := m.createFiles
	cfg := m.config
	currentEnv := m.config.CurrentEnvironment

	return func() tea.Msg {
		if client == nil {
			return createResourcesMsg{success: false, err: fmt.Errorf("not connected")}
		}

		var created []string
		var failed []string
		var lastErr error

		for _, file := range files {
			// Read file content
			content, err := os.ReadFile(file.LocalPath)
			if err != nil {
				failed = append(failed, file.WebResName)
				lastErr = err
				continue
			}

			encoded := base64.StdEncoding.EncodeToString(content)

			// Create the web resource
			resourceID, err := client.CreateWebResource(
				file.WebResName,
				filepath.Base(file.WebResName),
				encoded,
				file.ResourceType,
			)
			if err != nil {
				failed = append(failed, file.WebResName)
				lastErr = err
				continue
			}

			// Add to solution
			if solution != nil {
				if err := client.AddWebResourceToSolution(solution.UniqueName, resourceID); err != nil {
					// Resource created but failed to add to solution
					failed = append(failed, file.WebResName+" (add to solution)")
					lastErr = err
				}
			}

			// Publish the resource
			if err := client.PublishWebResource(resourceID); err != nil {
				// Resource created but failed to publish
				failed = append(failed, file.WebResName+" (publish)")
				lastErr = err
			}

			created = append(created, file.WebResName)

			// Create binding
			binding := config.Binding{
				Environment:      currentEnv,
				LocalPath:        file.LocalPath,
				WebResourceName:  file.WebResName,
				WebResourceID:    resourceID,
				LastKnownVersion: "1.0.0",
				AutoPublish:      true,
			}
			cfg.AddBinding(binding)
		}

		if len(failed) > 0 {
			return createResourcesMsg{
				success: false,
				err:     lastErr,
				created: created,
				failed:  failed,
			}
		}

		return createResourcesMsg{
			success: true,
			created: created,
		}
	}
}
