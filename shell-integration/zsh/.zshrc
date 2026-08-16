# Cmdex shell integration — zsh
# See .zshenv in this directory for the overall approach.

# By the time this file runs, /etc/zshrc has already run (zsh's startup order
# is .zshenv -> .zprofile -> /etc/zshrc -> .zshrc -> .zlogin) and, on macOS,
# /etc/zshrc sets HISTFILE=${ZDOTDIR:-$HOME}/.zsh_history — which at that
# point is still OUR integration directory, not the user's real $HOME. Left
# alone, every Cmdex session would silently write shell history into
# ~/.cmdex/shell-integration/zsh/.zsh_history instead of the user's real
# history file. Point it back before the user's own .zshrc loads, so an
# explicit HISTFILE there still takes precedence (it runs after this).
if [ "$HISTFILE" = "$ZDOTDIR/.zsh_history" ]; then
    HISTFILE="${CMDEX_USER_ZDOTDIR:-$HOME}/.zsh_history"
fi

if [ -n "$CMDEX_USER_ZDOTDIR" ] && [ -r "$CMDEX_USER_ZDOTDIR/.zshrc" ]; then
    source "$CMDEX_USER_ZDOTDIR/.zshrc"
fi

# The user's own config has now loaded. Restore ZDOTDIR to their real
# dotfile directory so anything spawned from within this session — a nested
# interactive zsh, a script with a zsh shebang — gets their normal setup
# instead of Cmdex's. zsh reads $ZDOTDIR fresh before looking for .zlogin, so
# this also makes their real .zlogin load next, without Cmdex needing to
# ship one of its own.
export ZDOTDIR="$CMDEX_USER_ZDOTDIR"
unset CMDEX_USER_ZDOTDIR
