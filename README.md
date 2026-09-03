# CmDex

<p align="center">
  <img src="assets/logo.svg" alt="CmDex Logo" width="120">
</p>

<p align="center">
  <img src="assets/demo.gif" alt="CmDex Demo" width="800">
</p>

> Your command library, everywhere. Save, organize, and run CLI commands with smart templates — on **macOS**, **Windows**, and **Linux**.

CmDex is a beautiful cross-platform desktop app that turns your scattered terminal commands into a searchable, categorized library. No more digging through shell history or forgetting that one Docker command you only run once a month.

## Download

**Ready to use?** Grab the latest installer for your platform from the [GitHub Releases](https://github.com/locnguyen1842/cmdex/releases) page:

- **macOS** — `.dmg` disk image
- **Windows** — `.exe` installer
- **Linux** — `.AppImage`, `.deb`, or `.rpm`

No build tools, no terminal setup — just download, install, and start saving commands.

> **macOS users:** Because CmDex is not signed with an Apple Developer certificate, macOS may show a security warning on first launch. If you see *"CmDex can't be opened because it was not downloaded from the App Store"* or a quarantine dialog, run this in Terminal:
>
> ```bash
> xattr -d com.apple.quarantine /Applications/cmdex.app
> ```
>
> Then relaunch the app.

## Features

- **Template Variables** — Drop `{{variableName}}` into any command. CmDex auto-detects variables as you type and prompts you when you run.
  ```bash
  docker logs -f --tail {{lines}} {{container_name}}
  ```
- **Organized Library** — Group commands into color-coded categories. Keep work scripts, deployment commands, and personal utilities in their own spaces.
- **Variable Presets** — Save named sets of values (e.g., "staging" vs. "production" configs) and switch between them instantly.
- **Smart Defaults** — Defaults support CEL expressions like `now()`, `env("HOME")`, and `date("2006-01-02")` so your commands are always up to date.
- **Real Built-in Terminal** — Commands run in a genuine PTY-backed terminal inside CmDex (powered by xterm.js), so interactive prompts, colors, and Ctrl+C all work. Open up to 10 terminal sessions side by side.
- **Exact Output Copying** — Optional shell integration (bash, zsh, fish, PowerShell) marks where each command's output begins and ends, so "copy last output" gives you exactly that — no prompt text, no reflow artifacts.
- **Lightning Search** — Find any command by title, description, tag, or script content in milliseconds (powered by SQLite FTS5).
- **Global Quick Launcher** — Summon your commands from anywhere with `Cmd/Ctrl+Shift+K`, Spotlight-style. Search, hit Enter, and watch the output stream in a panel right below — then press Esc and get back to what you were doing. ([details & platform notes](docs/CONFIGURATION.md#3a-global-quick-launcher))
- **Fully Local** — Everything lives on your machine in `~/.cmdex/cmdex.db`. No accounts, no cloud, no subscriptions.
- **Dark & Polished** — A premium dark UI with glassmorphism, smooth animations, and a layout that feels right at home on any OS.

## Documentation

| Doc | What you'll find |
|-----|------------------|
| [Getting Started](docs/GETTING-STARTED.md) | Prerequisites, install steps, and first run |
| [Development](docs/DEVELOPMENT.md) | Daily dev workflow, build commands, and code style |
| [Architecture](docs/ARCHITECTURE.md) | System design, data flow, and database schema |
| [Configuration](docs/CONFIGURATION.md) | App settings, themes, fonts, keybindings, and shell integration |
| [Testing](docs/TESTING.md) | Testing approach and manual test workflows |
| [Deployment](docs/DEPLOYMENT.md) | Platform packages, CI, Docker, and release process |
| [Contributing](CONTRIBUTING.md) | Bug reports, PR process, and coding standards |
| [Agents](AGENTS.md) | Quick reference for AI agents on this codebase |

## Quick Start

Want to build from source? See [Getting Started](docs/GETTING-STARTED.md) for full setup instructions.

```bash
git clone https://github.com/locnguyen1842/cmdex.git
cd cmdex
cd frontend && pnpm install && cd ..
wails3 dev
```

## Development

See [Development](docs/DEVELOPMENT.md) for the full guide on frontend and backend workflows, rebuilding Wails bindings, and code conventions.

## Architecture

Curious how the pieces fit together? Read the [Architecture Overview](docs/ARCHITECTURE.md) for system design, data flow, and key decisions.

## Contributing

We welcome bug fixes, features, and docs improvements. Please read [CONTRIBUTING.md](CONTRIBUTING.md) before opening an issue or pull request.

## License

CmDex is licensed under the [Apache License 2.0](LICENSE).

- The **core app** is free and open source — you can use, modify, and distribute it freely.
