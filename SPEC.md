# D365 TUI Web Resource Publisher - Implementation Instructions

## Project Overview

Build a terminal-based (TUI) application in Go that allows developers to authenticate against Microsoft Entra ID, list Dynamics 365 Web Resources, bind them to local files, and automatically publish updates to Dynamics 365 when local files change.

**Target:** Lightweight, developer-first alternative to XRMToolbox Autopublisher

## Technical Requirements

### Language & Framework
- **Go:** Version 1.22 or higher
- **TUI Framework:** Bubble Tea ecosystem
  - `github.com/charmbracelet/bubbletea`
  - `github.com/charmbracelet/bubbles`
  - `github.com/charmbracelet/lipgloss`
- **Additional Libraries:**
  - `github.com/fsnotify/fsnotify` for file watching
  - Standard library: `net/http`, `encoding/json`, `encoding/base64`, `os`, `time`

### Platform Support
Must work on macOS, Linux, and Windows with no platform-specific APIs or GUI dependencies.

## Architecture

```
┌────────────────────┐
│   Bubble Tea       │
│  (TUI State Loop)  │
└─────────┬──────────┘
          │
┌─────────▼──────────┐
│ Application Core   │
│ (State & Actions)  │
└─────────┬──────────┘
          │
 ┌────────▼─────────┐
 │ Auth (MSAL)      │
 │ Device Code Flow │
 └────────┬─────────┘
          │
 ┌────────▼─────────┐
 │ Dynamics Client  │
 │ Web API v9.2     │
 └────────┬─────────┘
          │
 ┌────────▼─────────┐
 │ File Watcher     │
 │ Auto Publish     │
 └──────────────────┘
```

## Package Structure

Organize code as follows:

```
cmd/d365tui/
  └── main.go

internal/
  ├── auth/
  │   ├── device_code.go
  │   └── token_store.go
  ├── config/
  │   └── config.go
  ├── d365/
  │   ├── client.go
  │   ├── webresources.go
  │   └── publish.go
  ├── watcher/
  │   └── watcher.go
  └── tui/
      ├── model.go
      ├── update.go
      └── view.go
```

## Authentication Implementation

### OAuth 2.0 Device Code Flow

**Requirements:**
- No client secrets or certificates
- Public client authentication only
- Token storage in local file system
- Use Dataverse App Registration: `51f81489-12ee-4a9e-aaae-a2591f45987d`

**Endpoints:**
1. **Device Code Request:**
   ```
   POST https://login.microsoftonline.com/common/oauth2/v2.0/devicecode
   Content-Type: application/x-www-form-urlencoded
   
   client_id=51f81489-12ee-4a9e-aaae-a2591f45987d
   &scope={ORG_URL}/.default
   ```

2. **Token Request:**
   ```
   POST https://login.microsoftonline.com/common/oauth2/v2.0/token
   Content-Type: application/x-www-form-urlencoded
   
   client_id=51f81489-12ee-4a9e-aaae-a2591f45987d
   &grant_type=urn:ietf:params:oauth:grant-type:device_code
   &device_code={DEVICE_CODE}
   ```

**Token Model:**
```go
type Token struct {
    AccessToken  string    `json:"access_token"`
    RefreshToken string    `json:"refresh_token"`
    ExpiresAt    time.Time `json:"expires_at"`
}
```

**Token Storage:**
- Store in `~/.d365tui/token.json`
- Automatically refresh before expiry
- Re-authenticate on refresh failure

**Token Scope per Environment:**
- Tokens are environment-specific (each environment gets its own token)
- Token file naming: `~/.d365tui/token-{environment-name}.json`
- Switching environments may require re-authentication if token is expired or missing
- Tokens should be loaded/saved when switching environments

**Required Scope:**
- `{ORG_URL}/.default` where ORG_URL is the Dynamics 365 organization URL

## Configuration Management

### Config File Location
`~/.d365tui/config.json`

### Config Schema
```json
{
  "currentEnvironment": "Production",
  "environments": [
    {
      "name": "Production",
      "url": "https://myorg.crm.dynamics.com"
    },
    {
      "name": "Development",
      "url": "https://myorg-dev.crm.dynamics.com"
    }
  ],
  "publisherPrefix": "ec",
  "bindings": [
    {
      "environment": "Production",
      "localPath": "./src/index.js",
      "webResourceName": "ec_/scripts/index.js",
      "webResourceId": "GUID",
      "lastKnownVersion": "1.0.12",
      "autoPublish": true
    }
  ]
}
```

### Config Rules
- File must be user-editable JSON
- Application must tolerate missing or partial config
- Bindings must be persisted immediately after changes
- Create config directory if it doesn't exist

### Environment Management Rules
- Each environment must have a unique name
- Environment names are case-sensitive
- Environment URLs must be valid Dynamics 365 URLs (format: `https://*.crm*.dynamics.com`)
- Bindings are scoped to environments (same local file can bind to different resources per environment)
- Deleting an environment removes all associated bindings
- Current environment selection persists across sessions
- If current environment is deleted, clear the selection and prompt on next launch

### Environment URL Validation
When adding or editing environments, validate URLs with the following rules:
- Must start with `https://`
- Must contain `.crm` followed by an optional region (e.g., `.crm`, `.crm4`, `.crm11`)
- Must end with `.dynamics.com`
- Examples of valid URLs:
  - `https://myorg.crm.dynamics.com`
  - `https://myorg.crm4.dynamics.com`
  - `https://myorg-dev.crm11.dynamics.com`
- Reject malformed URLs with clear error message

## Dynamics 365 Web API Integration

### API Version
Use Web API v9.2

### Base URL
`{orgUrl}/api/data/v9.2`

### Required Operations

#### 1. List Web Resources
```
GET /api/data/v9.2/webresourceset?$select=webresourceid,name,version
Authorization: Bearer {access_token}
```

**Response:** Array of web resources with id, name, and version

#### 2. Update Web Resource Content
```
PATCH /api/data/v9.2/webresourceset({webresourceid})
Authorization: Bearer {access_token}
Content-Type: application/json

{
  "content": "{base64_encoded_content}"
}
```

#### 3. Publish Web Resource
```
POST /api/data/v9.2/PublishXml
Authorization: Bearer {access_token}
Content-Type: application/json

{
  "ParameterXml": "<importexportxml><webresources><webresource>{GUID}</webresource></webresources></importexportxml>"
}
```

### Version Management
- Version is manually incremented
- Use semantic versioning format (x.y.z) preferred
- Auto-increment minor version on auto-publish

## File Binding & Watching

### Binding Definition
A binding maps one local file to one Dynamics 365 web resource within a specific environment. The same local file can have different bindings for different environments (e.g., development vs. production).

### File Watcher Implementation
- Create one watcher per bound file for the current environment only
- When switching environments, stop all current watchers and start new ones for the new environment
- Use `fsnotify` library
- Trigger on file write/save events
- Debounce changes (minimum 300ms between publishes)

### Auto-Publish Flow
1. File change detected by watcher
2. Read file contents from disk
3. Base64 encode contents
4. PATCH web resource content to Dynamics
5. Increment version number
6. POST PublishXml request
7. Update local config with new version
8. Display status in UI

## TUI Design Specification

### Application States
```go
type State int

const (
    StateEnvironmentSelect State = iota
    StateAuth
    StateList
    StateDetails
)
```

### Bubble Tea Model
```go
type Model struct {
    state              State
    token              *auth.Token
    config             *config.Config
    currentEnvironment *config.Environment
    resources          []d365.WebResource
    selected           int
    status             string
    inputMode          InputMode  // for text input during add/edit
    inputBuffer        string     // temporary input storage
}

type InputMode int

const (
    InputNone InputMode = iota
    InputEnvironmentName
    InputEnvironmentURL
)
```

### Screen 0: Environment Selection
**Initial screen shown on application load**

- Display list of saved environments (name + URL)
- Highlight currently selected environment
- Keyboard navigation (arrow keys)
- Actions available:
  - `Enter`: Select environment and proceed to authentication
  - `a`: Add new environment (prompt for name and URL)
  - `d`: Delete selected environment (with confirmation)
  - `e`: Edit selected environment
  - `q`: Quit application

**Add Environment Flow:**
1. Press `a` to trigger add mode
2. Prompt for environment name (text input)
3. Prompt for environment URL (text input with validation)
4. Validate URL format (must be valid Dynamics 365 URL)
5. Add to config and save
6. Return to environment list

**Delete Environment Flow:**
1. Select environment to delete
2. Press `d` to trigger delete
3. Show confirmation prompt: "Delete '{name}'? (y/n)"
4. If confirmed, remove from config and save
5. If it was the current environment, clear current selection
6. Return to environment list

**Edit Environment Flow:**
1. Select environment to edit
2. Press `e` to trigger edit mode
3. Show current values, allow modification
4. Update config and save
5. Return to environment list

### Screen 1: Authentication
- Display device code prominently
- Show verification URL
- Show spinner during token polling
- Automatically transition to list view on success

### Screen 2: Web Resource List
- Scrollable list of web resources
- Display columns:
  - Name
  - Version
  - Status (Bound/Unbound)
- Keyboard navigation (arrow keys)
- Visual indicator for selected item

### Screen 3: Actions Menu
Context-sensitive actions for selected web resource:
- Bind to local file (file picker)
- Publish now (manual trigger)
- Toggle auto-publish on/off

### Status Bar
- Display at bottom of screen
- Show last action result
- Display errors non-modally (don't block interaction)
- Clear after timeout or new action

### Keyboard Controls

**Environment Selection Screen:**
- `↑`/`↓` or `j`/`k`: Navigate environment list
- `Enter`: Select environment and continue
- `a`: Add new environment
- `e`: Edit selected environment
- `d`: Delete selected environment
- `q`: Quit application

**Web Resource List Screen:**
- `↑`/`↓` or `j`/`k`: Navigate list
- `Enter`: Select/activate action
- `b`: Bind file
- `p`: Publish selected
- `a`: Toggle auto-publish
- `Esc`: Return to environment selection
- `q`: Quit application

## Error Handling Strategy

### Authentication Errors
- Halt application and display error message
- Provide clear instructions for resolution
- Exit gracefully

### API Errors
- Display in status bar (non-modal)
- Log to file for debugging
- Don't crash the application

### File Watcher Errors
- Log errors but continue operation
- Display warning in status bar
- Don't stop other watchers

### Token Refresh Failures
- Trigger full re-authentication
- Clear invalid token
- Return to auth screen

## MVP Acceptance Criteria

The implementation is complete when:

1. ✅ User can manage environments (add, edit, delete, select)
2. ✅ User can authenticate via device code flow for selected environment
3. ✅ Web resources are listed in scrollable UI
4. ✅ Local files can be bound to web resources
5. ✅ Saving a bound file automatically publishes to Dynamics
6. ✅ UI remains responsive during publish operations
7. ✅ Status messages display success/failure
8. ✅ Config persists between sessions
9. ✅ Environment selection persists between sessions
10. ✅ Bindings are environment-specific

## Explicit Non-Goals (Out of Scope)

Do NOT implement:
- Solution import/export functionality
- Web resource creation or deletion
- GUI or browser-based UI
- Secret-based OAuth flows (client secret/cert)
- Multi-user shared state
- CI/CD pipeline integration
- Diff previews
- Batch publish operations
- Headless mode
- Multi-org profiles

## Implementation Notes

### Startup Behavior
- On first launch, show empty environment list with prompt to add environment
- On subsequent launches, show environment selection screen
- If `currentEnvironment` is set in config, highlight it as default selection
- User must explicitly select an environment to proceed
- After environment selection, check for valid token before proceeding to resource list

### Security
- Never store client secrets
- Never embed credentials in code
- Token file should have restricted permissions (0600)

### Performance
- Use goroutines for file watching
- Keep UI responsive during API calls
- Debounce file events to prevent rapid-fire publishes

### User Experience
- Provide clear feedback for all operations
- Show progress indicators for long-running tasks
- Handle connection errors gracefully
- Allow user to cancel operations when possible
- Validate environment URLs before saving
- Prevent duplicate environment names
- Confirm destructive actions (delete environment)
- Show helpful error messages for invalid URLs
- Display current environment in status bar or header

### Testing Considerations
- Mock API responses for testing
- Test error scenarios thoroughly
- Verify token refresh logic
- Test file watcher debouncing

## Development Workflow

1. Implement configuration management with multi-environment support
2. Build environment selection UI
3. Implement authentication module
4. Build Dynamics API client
5. Create basic TUI skeleton with state management
6. Implement web resource listing
7. Add file binding functionality
8. Implement file watcher
9. Add auto-publish logic
10. Polish UI and error handling
11. Test end-to-end workflow across multiple environments

## Configuration Examples

### Minimal Config (First Run)
```json
{
  "currentEnvironment": "",
  "environments": [],
  "publisherPrefix": "new",
  "bindings": []
}
```

### Single Environment Config
```json
{
  "currentEnvironment": "Production",
  "environments": [
    {
      "name": "Production",
      "url": "https://myorg.crm.dynamics.com"
    }
  ],
  "publisherPrefix": "ec",
  "bindings": []
}
```

### Full Multi-Environment Config
```json
{
  "currentEnvironment": "Development",
  "environments": [
    {
      "name": "Production",
      "url": "https://myorg.crm.dynamics.com"
    },
    {
      "name": "Development",
      "url": "https://myorg-dev.crm.dynamics.com"
    },
    {
      "name": "UAT",
      "url": "https://myorg-uat.crm.dynamics.com"
    }
  ],
  "publisherPrefix": "ec",
  "bindings": [
    {
      "environment": "Development",
      "localPath": "./src/main.js",
      "webResourceName": "ec_/scripts/main.js",
      "webResourceId": "12345678-1234-1234-1234-123456789abc",
      "lastKnownVersion": "1.0.0",
      "autoPublish": true
    },
    {
      "environment": "Production",
      "localPath": "./src/main.js",
      "webResourceName": "ec_/scripts/main.js",
      "webResourceId": "87654321-4321-4321-4321-cba987654321",
      "lastKnownVersion": "2.1.5",
      "autoPublish": false
    },
    {
      "environment": "Development",
      "localPath": "./styles/app.css",
      "webResourceName": "ec_/styles/app.css",
      "webResourceId": "11111111-2222-3333-4444-555555555555",
      "lastKnownVersion": "1.5.0",
      "autoPublish": true
    }
  ]
}
```

## Success Metrics

The application should:
- Authenticate in under 30 seconds
- Load web resources in under 5 seconds
- Publish changes in under 10 seconds
- Remain responsive at all times
- Never lose bindings between sessions
- Handle network interruptions gracefully