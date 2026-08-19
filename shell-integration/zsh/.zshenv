# Cmdex shell integration — zsh
#
# This directory is pointed to by $ZDOTDIR (see integrationFor in
# shell_integration.go) so zsh loads these files instead of the user's own
# ~/.zshenv, ~/.zprofile, ~/.zshrc. $ZDOTDIR is read fresh by zsh before each
# of the four startup files, so .zshrc (below) restores it to the user's real
# dotfile directory once their own config has loaded, and .zlogin is picked
# up from there automatically — this file never needs to ship its own
# .zlogin.
#
# .zshenv runs for every zsh invocation, not just interactive login shells
# (plain `zsh -c ...`, a script with a zsh shebang, etc). That's intentional:
# it's also where we install our OSC 133 hooks, and doing so here — before
# the user's own .zshrc has a chance to add its own precmd/preexec hooks
# (oh-my-zsh, prompt themes, ...) — means ours run FIRST each time. That
# matters because $? is only trustworthy for "the command that just
# finished" if nothing else has run a command in between, and other
# precmd hooks (e.g. anything that shells out for git status) can clobber it
# before a later-registered hook gets to read it.

# Capture the per-session OSC nonce (see generateOSCNonce/writeNonceFile in
# shell_integration.go and stripNonce in terminal_capture.go) into a
# non-exported shell variable, then delete the file and scrub its path from
# the environment before the user's own .zshenv — or anything it runs — can
# execute. The nonce is passed as a file rather than directly as an env var
# value on purpose: a shell `unset`ting an exported variable only edits its
# own live view of the environment — on Linux it does NOT erase the original
# environment block the kernel copied into this process's memory at exec()
# time, and /proc/<pid>/environ keeps exposing that block verbatim (to any
# same-uid process) for as long as this shell runs, regardless of any unset
# done here. A plain command run below — `cat /proc/$PPID/environ` — could
# otherwise recover the nonce that way and forge a "C"/"D" marker in its own
# output. Deleting the file (rather than just an env var) before anything
# else runs means no forked child ever gets a chance to read it, by either
# route.
#
# It does NOT protect against code that runs IN this shell process rather
# than as a child of it — a sourced profile/plugin, a shell function, `eval`
# — since zsh has no privacy between different code sharing one process:
# anything running in-process can read (or overwrite) any variable here by
# name, exported or not, the same way __cmdex_preexec/__cmdex_precmd below
# do. There's no fix for that at this layer (every OSC-133-based terminal
# integration has the same property), and it isn't a materially bigger hole
# regardless: code that already runs in-process in the user's shell can do
# far worse than spoof a copy-output marker — read their history,
# exfiltrate secrets, run anything as them.
if [ -n "$CMDEX_OSC_NONCE_FILE" ] && [ -r "$CMDEX_OSC_NONCE_FILE" ]; then
    __cmdex_nonce="$(cat "$CMDEX_OSC_NONCE_FILE")"
    rm -f "$CMDEX_OSC_NONCE_FILE"
fi
unset CMDEX_OSC_NONCE_FILE

# $ZDOTDIR still points at Cmdex's own integration directory here (it isn't
# permanently restored to the user's real one until .zshrc, the last file in
# this chain — see the comment there for why). But the user's .zshenv may
# itself reference $ZDOTDIR (e.g. `source "$ZDOTDIR/extra.zsh"`, a common
# pattern for locating a companion file alongside it), so it needs to see
# their real directory, not Cmdex's, for the duration of this source call.
# Swap it in just for that, then swap back immediately: zsh itself still
# needs $ZDOTDIR pointing at Cmdex's directory afterward, to find the next
# files in the chain (.zprofile, then .zshrc).
if [ -n "$CMDEX_USER_ZDOTDIR" ] && [ -r "$CMDEX_USER_ZDOTDIR/.zshenv" ]; then
    __cmdex_zdotdir="$ZDOTDIR"
    ZDOTDIR="$CMDEX_USER_ZDOTDIR"
    source "$CMDEX_USER_ZDOTDIR/.zshenv"
    # .zshenv is the one startup file zsh always loads from the default
    # location regardless of $ZDOTDIR, so it's the standard place a zsh
    # dotfiles setup relocates $ZDOTDIR itself to point the REST of the
    # chain (.zprofile/.zshrc/.zlogin) at a custom directory. If the user's
    # .zshenv just did that, $ZDOTDIR no longer equals what we sourced it
    # from — that new value is their real dotfile directory now, so update
    # CMDEX_USER_ZDOTDIR to match. Every later use of CMDEX_USER_ZDOTDIR in
    # this chain (this file's own restore below, .zprofile's and .zshrc's
    # own sourcing of the user's files) reads it, not $ZDOTDIR directly, so
    # this is what makes those look in the user's relocated directory
    # instead of the stale original one.
    #
    # "${ZDOTDIR:-$HOME}", not "$ZDOTDIR": a .zshenv that UNSETS $ZDOTDIR
    # (to restore zsh's own default of looking in $HOME, a less common but
    # equally standard variant of this pattern) would otherwise store an
    # empty CMDEX_USER_ZDOTDIR — every "-n $CMDEX_USER_ZDOTDIR" guard
    # downstream (.zprofile's and .zshrc's own sourcing, and .zshrc's final
    # ZDOTDIR restore) would then see it as unset and skip loading the
    # user's config entirely. Falling back to $HOME here mirrors zsh's own
    # documented behavior for an unset ZDOTDIR, and matches how
    # integrationForZsh in shell_integration.go computed CMDEX_USER_ZDOTDIR
    # in the first place.
    if [ "$ZDOTDIR" != "$CMDEX_USER_ZDOTDIR" ]; then
        CMDEX_USER_ZDOTDIR="${ZDOTDIR:-$HOME}"
    fi
    ZDOTDIR="$__cmdex_zdotdir"
    unset __cmdex_zdotdir
fi

autoload -Uz add-zsh-hook

# __cmdex_preexec fires just before a command's output starts, i.e. right as
# the command begins executing.
__cmdex_preexec() {
    print -n "\e]133;C;$__cmdex_nonce\a"
}

# __cmdex_precmd fires once the command has finished, right before zsh
# redraws the prompt. $? here is the finished command's real exit status —
# reading it as literally the first thing in this function (before any other
# statement can run and change it) is what makes that reliable.
__cmdex_precmd() {
    local __cmdex_ec=$?
    print -n "\e]133;D;$__cmdex_nonce;$__cmdex_ec\a"
}

add-zsh-hook preexec __cmdex_preexec
add-zsh-hook precmd __cmdex_precmd
