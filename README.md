# snip

A small snippet manager for your shell, inspired by [pet](https://github.com/knqyf263/pet).
The difference: its UI never takes over your terminal. It opens right under
your prompt, takes only the lines it needs and disappears when you're done,
so your scrollback stays where it was.

```
~/demo % dock
❯ dock                                                                       2/5
▌ List docker containers       docker ps -a --format 'table {{.Names}}'  #docker
  Run a throwaway container…   docker run --rm -it --name <name=scratch>…  #docker

↵ select  ·  ^e edit  ·  ^n new  ·  ^d delete  ·  esc quit
```

Pick a snippet with parameters and you fill them in right there, with a live
preview of the final command:

```
$ ssh root@example.com -p 22
  user  root
❯ host  example.com
  port  22
↵ next  ·  ⇥ move  ·  esc back
```

## Features

- Save commands with a description and tags
- Fuzzy search over descriptions, commands and tags (space-separated terms, smart case)
- Parameters: `<name>`, `<name=default>` and choices `<name=|_one_||_two_|>`
- Create, edit and delete snippets right from the picker
- A `Ctrl-S` zsh widget that puts the chosen command on your prompt
- Plain TOML file you can also edit by hand

## Install

```sh
go install github.com/HonzaKubita/snip@latest
```

or build it yourself:

```sh
git clone https://github.com/HonzaKubita/snip && cd snip
go build -o ~/bin/snip .
```

Works on macOS and Linux.

## Shell integration (zsh)

Add to `~/.zshrc`:

```sh
eval "$(snip init zsh)"
```

- **`Ctrl-S`** opens the picker. Whatever you already typed becomes the search
  query, and the chosen command replaces your prompt line, so you can check it
  before pressing enter and it ends up in your history.
- **`snip-prev`** saves the command you just ran as a new snippet.

To use another key, rebind the widget after the `eval`, e.g.
`bindkey '^G' snip-widget`. Other shells: `snip search` prints the chosen
command to stdout and draws its UI on `/dev/tty`, so it can be wired up the
same way.

## Usage

```
snip [query]           pick a snippet and run it
snip search [-q query] pick a snippet and print it (used by the shell widget)
snip exec [-q query]   pick a snippet and run it
snip new [command]     save a new snippet (the command is prefilled if given)
snip edit [-q query]   pick a snippet and edit it
snip edit --file       open the snippet file in $EDITOR
snip list              print all snippets
snip init zsh          print the zsh integration
snip path              print where snippets are stored
```

`snip` with no subcommand runs the chosen command (printed as `$ …` first);
when its output is piped it prints the command instead.

### Keys

| Where      | Keys |
|------------|------|
| Picker     | type to filter · `↑`/`↓` (`^p`/`^j`/`^k`) move · `enter` use · `^e` edit · `^n` new · `^d` delete · `esc` quit |
| Form       | `enter` next field (saves on the last) · `tab`/`shift-tab` move · `^s` save · `esc` back |
| Parameters | `enter` next (finishes on the last) · `tab` move · `←`/`→` change a choice · `esc` back |

Text fields have the usual readline keys: `^a`/`^e`, `^w`, `^u`, `^k`,
`alt-b`/`alt-f`, and so on.

## Parameters

| Syntax                    | Asks for                         |
|---------------------------|----------------------------------|
| `<name>`                  | a value                          |
| `<name=default>`          | a value, prefilled with default  |
| `<name=\|_one_\|\|_two_\|>` | one of the options (`←`/`→`)   |

A name used several times is asked for once. Names start with a letter or
`_`, so shell syntax like `< file`, `<<EOF` or `<(cmd)` is left alone.

## Snippet file

`~/.config/snip/snippets.toml` (or `$XDG_CONFIG_HOME/snip/snippets.toml`, or
`$SNIP_FILE`). Symlinks are written through, so it can live in a dotfiles repo.

```toml
[[snippet]]
description = "SSH into a server"
command = "ssh <user=root>@<host> -p <port=22>"
tags = ["ssh"]
```

## License

MIT
