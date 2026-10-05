package main

// Shell integration: Ctrl-S opens the picker and puts the chosen command on
// your prompt, so you can check or tweak it before pressing enter (and it
// lands in your shell history). The snip function runs picked snippets in
// your shell rather than a new one, so your aliases and functions work.
// snip-prev saves the previous command.
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

# snip hands the command it would run over on fd 3 and it runs here, with
# your aliases and functions. Its own output still goes to your terminal.
snip() {
  local _snip_cmd _snip_ret
  { _snip_cmd="$(SNIP_EVAL_FD=3 command snip "$@" 3>&1 1>&4 4>&-)"; _snip_ret=$?; } 4>&1
  (( _snip_ret == 0 )) && [[ -n $_snip_cmd ]] || return $_snip_ret
  print -s -- "$_snip_cmd"
  eval "$_snip_cmd"
}

snip-prev() { command snip new "$(fc -ln -1)"; }
`,
}
