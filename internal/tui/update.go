package tui

import (
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"d365tui/internal/auth"
	"d365tui/internal/config"
	"d365tui/internal/d365"
	"d365tui/internal/watcher"

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
	errMsg          error
	statusClearMsg  struct{}
	fileChangeMsg   string
	watcherReadyMsg *watcher.Watcher
)

// Init initializes the model
func (m Model) Init() tea.Cmd {
	return tea.Batch(m.spinner.Tick)
}

// Update handles messages
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	// Handle file picker state - it needs to receive all messages
	if m.state == StateFilePicker {
		return m.handleFilePicker(msg)
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
			m.state = StateList
			return m, m.fetchResources()
		}

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
