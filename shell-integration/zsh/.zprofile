# Cmdex shell integration — zsh
# See .zshenv in this directory for the overall approach, and its comment on
# the swap-ZDOTDIR-then-restore-it pattern used here for the same reason:
# the user's own .zprofile may reference $ZDOTDIR and expects it to mean
# their real directory, but zsh itself still needs it pointing at Cmdex's
# directory afterward to find .zshrc next.
if [ -n "$CMDEX_USER_ZDOTDIR" ] && [ -r "$CMDEX_USER_ZDOTDIR/.zprofile" ]; then
    __cmdex_zdotdir="$ZDOTDIR"
    ZDOTDIR="$CMDEX_USER_ZDOTDIR"
    source "$CMDEX_USER_ZDOTDIR/.zprofile"
    ZDOTDIR="$__cmdex_zdotdir"
    unset __cmdex_zdotdir
fi
