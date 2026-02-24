# D365 TUI Web Resource Publisher

A lightweight terminal-based (TUI) application for managing Microsoft Dynamics 365 web resources. Authenticate against Microsoft Entra ID, bind web resources to local files, and automatically publish updates when files change.

## Installation

### Using go install

```bash
go install codeberg.org/schalkuz/xrm-webresource-publisher/cmd/d365tui@latest
```

This will install the `d365tui` binary to your `$GOPATH/bin` directory (usually `~/go/bin`).

### From Source

```bash
git clone ssh://git@codeberg.org/schalkuz/xrm-webresource-publisher.git
cd xrm-webresource-publisher
go build -o d365tui ./cmd/d365tui
```

## Usage

Start the application:

```bash
d365tui
```

### Environment Setup

1. Add your Dynamics 365 environment (name and URL)
2. Select an environment to authenticate
3. Complete the device code authentication flow in your browser

### Managing Web Resources

#### Bind Files Tab

- Navigate the tree structure of web resources
- Expand/collapse folders with `enter`
- Bind files to web resources with `b`
- Enable auto-publishing with `a`
- Publish manually with `p`
- Toggle managed/unmanaged view with `m`
- Unbind with `u`

#### File List Tab

- View all bound files in a clean list
- See file paths and binding status at a glance
- Toggle auto-publish with `a`
- Publish with `p`
- Toggle managed/unmanaged view with `m`
- Unbind with `u`

### Keyboard Shortcuts

| Key             | Action                                  |
| --------------- | --------------------------------------- |
| `tab`           | Switch between tabs                     |
| `↑/↓` or `k/j`  | Navigate                                |
| `enter`         | Expand/collapse folder (Bind Files tab) |
| `b`             | Bind file (Bind Files tab only)         |
| `u`             | Unbind file                             |
| `p`             | Publish resource                        |
| `a`             | Toggle auto-publish                     |
| `m`             | Toggle managed/unmanaged filter        |
| `r`             | Refresh resources                       |
| `esc`           | Back/Cancel                             |
| `q` or `ctrl+c` | Quit                                    |

## Configuration

Configuration is stored in:

- **macOS/Linux**: `~/.config/d365tui/config.json`
- **Windows**: `%APPDATA%\d365tui\config.json`

Tokens are stored in:

- **macOS/Linux**: `~/.config/d365tui/tokens/`
- **Windows**: `%APPDATA%\d365tui\tokens\`

## Requirements

- Go 1.22 or higher (for installation from source)

## Authentication

This tool uses the Microsoft Dynamics 365 public client app registration (`51f81489-12ee-4a9e-aaae-a2591f45987d`) for authentication via OAuth 2.0 Browser Flow.
