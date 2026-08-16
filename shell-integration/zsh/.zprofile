# Cmdex shell integration — zsh
# See .zshenv in this directory for the overall approach.
if [ -n "$CMDEX_USER_ZDOTDIR" ] && [ -r "$CMDEX_USER_ZDOTDIR/.zprofile" ]; then
    source "$CMDEX_USER_ZDOTDIR/.zprofile"
fi
