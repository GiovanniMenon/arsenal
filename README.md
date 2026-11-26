<p align="center">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset=".github/assets/banner-dark.svg">
    <img alt="Arsenal" src=".github/assets/banner-light.svg">
  </picture>
</p>

**Arsenal** is a terminal tool to store, search and reuse the commands you need during a penetration test.

Between work and study I use a lot of commands, and I needed one place to keep them and find them again quickly, without leaving the terminal. I took inspiration from the [Arsenal](https://github.com/Orange-Cyberdefense/arsenal) project and built something easy and fast to use.

On top of that it keeps your work organized per engagement so each session has its own variables, notes, command log and terminal recordings.

> [!NOTE]
> This project was written with heavy use of LLMs. If you prefer a version written without them, I recommend the original [Arsenal](https://github.com/Orange-Cyberdefense/arsenal) project.

## Features

- **Search:** fuzzy search on title, command, description and tags
- **Variables:** support both global and session scoped variables (e.g. `<target>`)
- **Cheatsheets:** supports `.md` and `.yml` cheatsheets (as Arsenal)
- **Fully customizable:** add your own commands with `ars add`, edit or delete them from the search, and add your own cheatsheet folders
- **Sessions:** one scoped folder per engagement
- **Notes:** timestamped Markdown notes for each session, searchable and editable
- **Command log and recording:** every command you run in a session is logged, and `ars rec` records the terminal to replay later

## Setup

Requirements: macOS or Linux, Go and `make`.

```bash
git clone https://github.com/GiovanniMenon/arsenal.git
```

```bash
cd arsenal && make install
```

This puts `ars` and the `arsenal` alias in `~/.local/bin`. Then run the setup wizard:

```bash
ars setup
```

The wizard asks for your pentest folder (default `~/Documents/pentest`), whether to add the original Arsenal commands to your cheatsheets, whether to record sessions by default, and adds one line to your shell rc file so that `ars` can put commands in your prompt and `cd` into sessions.

Run `make uninstall` to remove it (your config is kept).

## Usage

A typical engagement:

```bash
ars init acme
ars set target=10.10.10.5 domain=acme.com
ars # start with a search
ars note -t "Admin panel" "default creds rejected, try the reset form"
ars history --failed --since 2h
```

## Commands

| Command             | What it does                                                 |
| ------------------- | ------------------------------------------------------------ |
| `ars`               | Search the commands and put the chosen one in your prompt    |
| `ars init [name]`   | Create or resume a session                                   |
| `ars sessions`      | List your sessions                                           |
| `ars set key=value` | Set variables used to fill commands (`-g` for every session) |
| `ars unset key`     | Remove variables                                             |
| `ars vars`          | Show the variables in effect and where each comes from       |
| `ars add [command]` | Save a command to your cheatsheets                           |
| `ars note [text]`   | Add a note to the session                                    |
| `ars notes`         | Browse, search, edit and delete the notes of the session     |
| `ars history`       | Search the commands you ran in a session                     |
| `ars rec`           | Record your terminal so you can replay it                    |
| `ars replay [id]`   | Play back a recording                                        |
| `ars pause`         | Pause or resume logging and recording                        |
| `ars status`        | Show a summary of the current session                        |
| `ars config`        | Show where Arsenal keeps its files                           |
| `ars setup`         | Run the setup wizard                                         |

Main flags of the search:

| Flag                 | What it does                                                    |
| -------------------- | --------------------------------------------------------------- |
| `-s, --search QUERY` | start with QUERY in the search box                              |
| `-p, --print`        | print the command to stdout                                     |
| `-x, --copy`         | copy the command to the clipboard                               |
| `-t, --tmux`         | send the command to another tmux pane (with `-e`, run it there) |
| `-f, --prefix`       | add your prefix command (`arsenal_prefix_cmd`) in front         |
| `-o, --outfile FILE` | write the command to FILE                                       |
| `-c, --check`        | check which programs used by the cheatsheets are installed      |

Every command has its own help with examples: `ars <command> --help`.

## Credits

- [Arsenal](https://github.com/Orange-Cyberdefense/arsenal) by [Orange Cyberdefense](https://github.com/Orange-Cyberdefense): the idea, the behavior and the cheatsheets bundled in `arsenal-cheats/` come from this project. This is a Go rewrite of it.
- Built with [cobra](https://github.com/spf13/cobra) and the [Charm](https://github.com/charmbracelet) libraries (bubbletea, bubbles, lipgloss, huh, glamour), [sahilm/fuzzy](https://github.com/sahilm/fuzzy) and [creack/pty](https://github.com/creack/pty).

## License

[GPL-3.0](LICENSE), the same as the original Arsenal.

## Maintenance

I will maintain this tool for as long as I actively use it.
