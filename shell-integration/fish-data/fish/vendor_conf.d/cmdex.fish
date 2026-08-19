# Cmdex shell integration — fish
#
# Loaded automatically because Cmdex prepends this file's directory's parent
# to $XDG_DATA_DIRS (see integrationFor in shell_integration.go): fish
# auto-sources every *.fish file under $XDG_DATA_DIRS/fish/vendor_conf.d/ on
# startup, alongside — not instead of — the user's own
# ~/.config/fish/config.fish. Unlike the zsh integration, nothing here needs
# restoring afterward: we're only adding an extra auto-loaded file, not
# replacing where fish looks for the user's own config.
#
# fish_preexec/fish_postexec are fish's own equivalent of preexec/precmd,
# firing once per top-level command (not per pipeline stage), so no extra
# gating is needed here the way bash's DEBUG-trap-based approach requires.

# Capture the per-session OSC nonce (see generateOSCNonce/writeNonceFile in
# shell_integration.go and stripNonce in terminal_capture.go) into a
# shell-global (but NOT exported) variable, then delete the file and scrub
# its path from the environment before the user's own config.fish — or
# anything it runs — can execute. The nonce is passed as a file rather than
# directly as an env var value on purpose: a shell erasing an exported
# variable only edits its own live view of the environment — on Linux it
# does NOT erase the original environment block the kernel copied into this
# process's memory at exec() time, and /proc/<pid>/environ keeps exposing
# that block verbatim (to any same-uid process) for as long as this shell
# runs, regardless of any `set -e` done here. A plain command run below —
# `cat /proc/$PPID/environ` — could otherwise recover the nonce that way and
# forge a "C"/"D" marker in its own output. Deleting the file (rather than
# just an env var) before anything else runs means no forked child ever gets
# a chance to read it, by either route. `-g` (not `-l`) is required for
# __cmdex_nonce itself: fish functions don't close over a sourced file's
# local variables, only global ones, so $__cmdex_nonce would otherwise be
# invisible inside __cmdex_preexec/__cmdex_postexec below.
#
# This does NOT protect against code that runs IN this shell process rather
# than as a child of it — a sourced config/plugin, another function, `eval`
# — since a fish-global variable has no privacy from other code sharing the
# same process; there's no fix for that at this layer, and it isn't a
# materially bigger hole regardless (such code could already do far worse
# than spoof a copy-output marker).
if test -n "$CMDEX_OSC_NONCE_FILE"; and test -r "$CMDEX_OSC_NONCE_FILE"
    set -g __cmdex_nonce (cat $CMDEX_OSC_NONCE_FILE)
    rm -f $CMDEX_OSC_NONCE_FILE
end
set -e CMDEX_OSC_NONCE_FILE

function __cmdex_preexec --on-event fish_preexec
    printf '\e]133;C;%s\a' "$__cmdex_nonce"
end

function __cmdex_postexec --on-event fish_postexec
    printf '\e]133;D;%s;%s\a' "$__cmdex_nonce" "$status"
end
