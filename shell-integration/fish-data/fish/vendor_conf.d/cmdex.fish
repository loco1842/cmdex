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

# Capture the per-session OSC nonce (see generateOSCNonce in
# shell_integration.go and stripNonce in terminal_capture.go) into a
# shell-global (but NOT exported) variable, then scrub it from the
# environment before the user's own config.fish — or anything it runs — can
# execute. A child process only ever sees exported environment variables, so
# once erased here, nothing spawned from this shell can read the nonce and
# use it to forge a "C"/"D" marker in its own output. `-g` (not `-l`) is
# required here: fish functions don't close over a sourced file's local
# variables, only global ones, so $__cmdex_nonce would otherwise be invisible
# inside __cmdex_preexec/__cmdex_postexec below.
set -g __cmdex_nonce $CMDEX_OSC_NONCE
set -e CMDEX_OSC_NONCE

function __cmdex_preexec --on-event fish_preexec
    printf '\e]133;C;%s\a' "$__cmdex_nonce"
end

function __cmdex_postexec --on-event fish_postexec
    printf '\e]133;D;%s;%s\a' "$__cmdex_nonce" "$status"
end
