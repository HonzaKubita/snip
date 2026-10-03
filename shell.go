package main

// Shell integration: Ctrl-S opens the picker and puts the chosen command on
// your prompt, so you can check or tweak it before pressing enter (and it
// lands in your shell history). snip-prev saves the previous command.
var shellScripts = map[string]string{
	"zsh": `# snip: eval "$(snip init zsh)" in ~/.zshrc
snip-widget() {
  local selected ret
  selected="$(command snip search --query "$BUFFER")"
  ret=$?
  # snip turns bracketed paste off on exit; zle expects it on.
  [[ -n ${zle_bracketed_paste[1]-} ]] && print -rn -- "${zle_bracketed_paste[1]}" > /dev/tty
  if (( ret == 0 )) && [[ -n $selected ]]; then
    BUFFER=$selected
    CURSOR=${#BUFFER}
  fi
  zle reset-prompt
}
zle -N snip-widget
unsetopt flow_control  # free up Ctrl-S
bindkey -M emacs '^S' snip-widget
bindkey -M viins '^S' snip-widget
bindkey -M vicmd '^S' snip-widget

snip-prev() { command snip new "$(fc -ln -1)"; }
`,
}
