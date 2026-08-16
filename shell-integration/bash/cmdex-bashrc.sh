# Cmdex shell integration — bash
#
# Passed via `--rcfile <this file> -i`, replacing bash's normal login-shell
# startup (`-l`, which bash ignores whenever --rcfile is also given — the two
# flags are mutually exclusive in bash's startup-file selection logic) so we
# can also install OSC 133 hooks. To preserve the user's usual login-shell
# environment, this manually replicates bash's own login sequence: /etc/profile,
# then the first of ~/.bash_profile, ~/.bash_login, ~/.profile that exists —
# deliberately NOT ~/.bashrc directly, matching real login bash (their own
# .bash_profile is free to source .bashrc itself, same as on a real login
# terminal).

if [ -r /etc/profile ]; then
    source /etc/profile
fi

if [ -r "$HOME/.bash_profile" ]; then
    source "$HOME/.bash_profile"
elif [ -r "$HOME/.bash_login" ]; then
    source "$HOME/.bash_login"
elif [ -r "$HOME/.profile" ]; then
    source "$HOME/.profile"
fi

# --- OSC 133 hooks ---
#
# bash has no preexec/precmd hook mechanism of its own, so this adapts the
# well-known bash-preexec.sh pattern: the DEBUG trap fires before every
# simple command (including each stage of a pipeline and each statement in a
# compound command), so __cmdex_armed gates it to fire exactly once per
# top-level command rather than once per simple command. It starts at 0 (not
# armed) so nothing fires while this file itself — or the user's own sourced
# profile above — is still running; it only becomes armed once
# __cmdex_prompt_command has run for the first time, i.e. right before the
# first real prompt is shown to the user.
#
# __cmdex_prompt_command is appended LAST to PROMPT_COMMAND (after whatever
# the user's own profile may have set), so armed only flips back to 1 once
# bash is done running everything queued for after a command finishes — the
# very next DEBUG trap firing after that is genuinely the user's next typed
# command, not more of our/their own prompt machinery. Capturing $? as the
# first statement of __cmdex_prompt_command is what makes the reported exit
# code reliable: it reads $? before any other statement (even a `[` test)
# gets a chance to run and overwrite it.
#
# Known limitation: bash allows only one DEBUG trap handler at a time. If the
# user's own profile also relies on one (e.g. a command-timing tool), it is
# overwritten here rather than chained — the same limitation bash-preexec.sh
# itself documents.
__cmdex_armed=0

__cmdex_debug_trap() {
    if [ "$__cmdex_armed" = "1" ]; then
        __cmdex_armed=0
        printf '\e]133;C\a'
    fi
}

__cmdex_prompt_command() {
    local __cmdex_ec=$?
    printf '\e]133;D;%s\a' "$__cmdex_ec"
    __cmdex_armed=1
}

trap '__cmdex_debug_trap' DEBUG
PROMPT_COMMAND="${PROMPT_COMMAND:+$PROMPT_COMMAND$'\n'}__cmdex_prompt_command"
