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

# Restore ZDOTDIR to the user's real dotfile directory BEFORE sourcing their
# .zshrc, not after: their config may itself reference $ZDOTDIR (a common
# pattern for locating a plugin/alias file alongside .zshrc, e.g. `source
# "$ZDOTDIR/aliases.zsh"`), and until this line it still points at Cmdex's
# own integration directory, not theirs — such a reference would silently
# fail to load. This also means anything spawned from within this session —
# a nested interactive zsh, a script with a zsh shebang — gets their normal
# setup instead of Cmdex's, and zsh reads $ZDOTDIR fresh before looking for
# .zlogin, so this makes their real .zlogin load next too, without Cmdex
# needing to ship one of its own.
if [ -n "$CMDEX_USER_ZDOTDIR" ]; then
    userZDOTDIR="$CMDEX_USER_ZDOTDIR"
    export ZDOTDIR="$userZDOTDIR"
    unset CMDEX_USER_ZDOTDIR

    if [ -r "$userZDOTDIR/.zshrc" ]; then
        source "$userZDOTDIR/.zshrc"
    fi
    unset userZDOTDIR
fi
