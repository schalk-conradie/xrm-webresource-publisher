package tui

import (
	"sort"
	"strings"

	"d365tui/internal/auth"
	"d365tui/internal/config"
	"d365tui/internal/d365"
	"d365tui/internal/watcher"

	"github.com/charmbracelet/bubbles/filepicker"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
)

// State represents the application state
type State int

const (
	StateEnvironmentSelect State = iota
	StateAuth
	StateList
	StateBinding
	StateFilePicker
)

// InputMode represents the current input mode
type InputMode int

const (
	InputNone InputMode = iota
	InputEnvironmentName
	InputEnvironmentURL
	InputBindingPath
	InputDeleteConfirm
)

// TreeNode represents a folder or file in the tree
type TreeNode struct {
	Name     string
	FullPath string
	IsFolder bool
	Resource *d365.WebResource
	Children []*TreeNode
	Expanded bool
	Depth    int
}

// DisplayItem represents a flattened item for display
type DisplayItem struct {
	Node     *TreeNode
	Resource *d365.WebResource
}

// Model represents the Bubble Tea model
type Model struct {
	state            State
	config           *config.Config
	token            *auth.Token
	client           *d365.Client
	watcher          *watcher.Watcher
	fileChangeChan   chan string
	resources        []d365.WebResource
	treeRoot         *TreeNode
	displayItems     []DisplayItem
	expandedFolders  map[string]bool
	envSelected      int
	resourceSelected int
	status           string
	statusIsError    bool
	inputMode        InputMode
	textInput        textinput.Model
	spinner          spinner.Model
	filepicker       filepicker.Model
	bindingResource  *d365.WebResource
	editingEnvName   string
	publishing       map[string]bool // tracks which resource IDs are currently publishing
	width            int
	height           int
	err              error
}

// NewModel creates a new application model
func NewModel() Model {
	ti := textinput.New()
	ti.Focus()

	s := spinner.New()
	s.Spinner = spinner.Dot

	cfg, err := config.Load()
	if err != nil {
		cfg = &config.Config{
			Environments: []config.Environment{},
			Bindings:     []config.Binding{},
		}
	}

	return Model{
		state:           StateEnvironmentSelect,
		config:          cfg,
		textInput:       ti,
		spinner:         s,
		width:           80,
		height:          24,
		expandedFolders: make(map[string]bool),
		publishing:      make(map[string]bool),
		fileChangeChan:  make(chan string, 10), // Buffered channel for file changes
	}
}

// buildTree creates a tree structure from flat web resources
func (m *Model) buildTree() {
	root := &TreeNode{
		Name:     "root",
		IsFolder: true,
		Children: []*TreeNode{},
		Expanded: true,
	}

	for i := range m.resources {
		res := &m.resources[i]
		parts := strings.Split(res.Name, "/")
		current := root

		for j, part := range parts {
			if j == len(parts)-1 {
				// This is the file
				node := &TreeNode{
					Name:     part,
					FullPath: res.Name,
					IsFolder: false,
					Resource: res,
					Depth:    j,
				}
				current.Children = append(current.Children, node)
			} else {
				// This is a folder
				folderPath := strings.Join(parts[:j+1], "/")
				found := false
				for _, child := range current.Children {
					if child.IsFolder && child.Name == part {
						current = child
						found = true
						break
					}
				}
				if !found {
					node := &TreeNode{
						Name:     part,
						FullPath: folderPath,
						IsFolder: true,
						Children: []*TreeNode{},
						Expanded: m.expandedFolders[folderPath],
						Depth:    j,
					}
					current.Children = append(current.Children, node)
					current = node
				}
			}
		}
	}

	// Sort children at each level (folders first, then alphabetically)
	sortChildren(root)
	m.treeRoot = root
	m.flattenTree()
}

func sortChildren(node *TreeNode) {
	sort.Slice(node.Children, func(i, j int) bool {
		// Folders come before files
		if node.Children[i].IsFolder != node.Children[j].IsFolder {
			return node.Children[i].IsFolder
		}
		// Alphabetical within same type
		return node.Children[i].Name < node.Children[j].Name
	})
	for _, child := range node.Children {
		if child.IsFolder {
			sortChildren(child)
		}
	}
}

// flattenTree creates a flat list of visible items for display
func (m *Model) flattenTree() {
	m.displayItems = []DisplayItem{}
	if m.treeRoot == nil {
		return
	}
	m.flattenNode(m.treeRoot)
}

func (m *Model) flattenNode(node *TreeNode) {
	for _, child := range node.Children {
		m.displayItems = append(m.displayItems, DisplayItem{
			Node:     child,
			Resource: child.Resource,
		})
		if child.IsFolder && child.Expanded {
			m.flattenNode(child)
		}
	}
}

// toggleFolder expands or collapses a folder
func (m *Model) toggleFolder(path string) {
	m.expandedFolders[path] = !m.expandedFolders[path]
	m.buildTree()
}
