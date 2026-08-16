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

# Capture the per-session OSC nonce (see generateOSCNonce in
# shell_integration.go and stripNonce in terminal_capture.go) into a
# non-exported shell variable, then scrub it from the environment before
# anything below runs. A child process only ever sees exported environment
# variables, so once unset here, nothing this shell ever forks and execs —
# any regular command the user runs — can read the nonce back out of its own
# environment and use it to forge a "C"/"D" marker in its own output. That's
# the threat this closes: a program's stdout/stderr containing (by chance or
# by design) the same bytes our hooks emit.
#
# It does NOT protect against code that runs IN this shell process rather
# than as a child of it — a sourced profile/plugin, a shell function, `eval`
# — since bash has no privacy between different code sharing one process:
# anything running in-process can read (or overwrite) any variable here by
# name, exported or not, the same way our own hooks below do. There's no
# fix for that at this layer (every OSC-133-based terminal integration has
# the same property), and it isn't a materially bigger hole regardless: code
# that already runs in-process in the user's shell can do far worse than
# spoof a copy-output marker — read their history, exfiltrate secrets, run
# anything as them.
__cmdex_nonce="$CMDEX_OSC_NONCE"
unset CMDEX_OSC_NONCE

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
# __cmdex_emit_marker has run for the first time, i.e. right before the
# first real prompt is shown to the user.
#
# The work done once a command finishes is deliberately split into two
# PROMPT_COMMAND entries rather than one:
#
#   - __cmdex_capture_exit is PREPENDED, so it runs BEFORE whatever the
#     user's own profile may have put in PROMPT_COMMAND (git-prompt.sh,
#     starship, a timing plugin, ...). Those commonly run commands of their
#     own, which overwrite $? — capturing it first, before any of that runs,
#     is the only way the reported exit code reliably reflects the command
#     the user actually typed rather than the last thing the user's own
#     prompt machinery happened to run.
#   - __cmdex_emit_marker is APPENDED LAST (after whatever the user's own
#     profile may have set), so armed only flips back to 1 once bash is done
#     running everything queued for after a command finishes — the very next
#     DEBUG trap firing after that is genuinely the user's next typed
#     command, not more of our/their own prompt machinery.
#
# Known limitation: bash allows only one DEBUG trap handler at a time. If the
# user's own profile also relies on one (e.g. a command-timing tool), it is
# overwritten here rather than chained — the same limitation bash-preexec.sh
# itself documents.
__cmdex_armed=0

__cmdex_debug_trap() {
    if [ "$__cmdex_armed" = "1" ]; then
        __cmdex_armed=0
        printf '\e]133;C;%s\a' "$__cmdex_nonce"
    fi
}

__cmdex_capture_exit() {
    __cmdex_ec=$?
}

__cmdex_emit_marker() {
    printf '\e]133;D;%s;%s\a' "$__cmdex_nonce" "$__cmdex_ec"
    __cmdex_armed=1
}

trap '__cmdex_debug_trap' DEBUG
PROMPT_COMMAND="__cmdex_capture_exit${PROMPT_COMMAND:+$'\n'$PROMPT_COMMAND}"$'\n'"__cmdex_emit_marker"
