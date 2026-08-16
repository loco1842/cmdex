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

function __cmdex_preexec --on-event fish_preexec
    printf '\e]133;C\a'
end

function __cmdex_postexec --on-event fish_postexec
    printf '\e]133;D;%s\a' "$status"
end
