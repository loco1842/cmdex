# Cmdex shell integration — PowerShell
#
# Loaded via `-Command ". '<this file>'"` (see integrationFor in
# shell_integration.go), which runs after pwsh's normal profile-processing
# startup phase — so $PROFILE has already run by the time this executes, and
# the `prompt` function overridden below is whatever the user's own profile
# (oh-my-posh, starship, a custom theme, ...) last defined, not pwsh's
# built-in default.
#
# PowerShell has no preexec/precmd event hooks. This chains the two places
# PowerShell already calls at the right moments:
#   - `prompt`, invoked every time the prompt is about to be (re)displayed,
#     i.e. right after the previous command finished — used for "D".
#   - `PSConsoleHostReadLine`, invoked once to read the next command line
#     from the user, right before it executes — used for "C". This function
#     is defined by the PSReadLine module, which ships with and is loaded by
#     default in every interactive pwsh session; if some minimal
#     configuration doesn't have it, the guard below just skips the "C" half
#     and D-only markers still let GetLastOutput ignore stray D's the same
#     way it does for zsh/bash's own startup-time D-with-no-C case.

# Capture the per-session OSC nonce (see generateOSCNonce/writeNonceFile in
# shell_integration.go and stripNonce in terminal_capture.go) into a global
# variable, then delete the file and scrub its path from the environment.
# The nonce is passed as a file rather than directly as an env var value on
# purpose: a session removing an env var only edits its own live view of the
# environment — on platforms where a process's initial environment block
# stays readable by other means for the process's lifetime regardless of
# later changes (e.g. /proc/<pid>/environ on Linux, which pwsh also runs on)
# — a plain command run from here on could otherwise recover the nonce that
# way and forge a "C"/"D" marker in its own output. Deleting the file
# (rather than just an env var) closes that route. This runs after $PROFILE
# (pwsh has no earlier hook to do this from), so a command the user's own
# profile ran before this point could in principle still have seen the file
# — an unavoidable gap given pwsh's lack of a preexec-style hook, but
# everything launched from here on is protected.
#
# This does NOT protect against code that runs IN this session rather than
# as a separate process — a dot-sourced profile script, another function,
# Invoke-Expression — since a global variable has no privacy from other code
# sharing the same session; there's no fix for that at this layer, and it
# isn't a materially bigger hole regardless (such code could already do far
# worse than spoof a copy-output marker).
if (-not (Test-Path Variable:\global:cmdexNonce)) {
    $cmdexNonceFile = $env:CMDEX_OSC_NONCE_FILE
    if ($cmdexNonceFile -and (Test-Path $cmdexNonceFile)) {
        $global:cmdexNonce = Get-Content -Path $cmdexNonceFile -Raw
        Remove-Item -Path $cmdexNonceFile -ErrorAction SilentlyContinue
    }
    Remove-Item Env:\CMDEX_OSC_NONCE_FILE -ErrorAction SilentlyContinue
}

if (-not (Test-Path Function:\global:__cmdex_original_prompt)) {
    Rename-Item Function:\global:prompt Function:\global:__cmdex_original_prompt -ErrorAction SilentlyContinue

    function global:prompt {
        # Read *before* anything else runs, so it reflects the command that
        # just finished rather than something this function itself did.
        # $? is PowerShell's own pass/fail signal (works for cmdlets and
        # native commands alike); $LASTEXITCODE is only ever set by native
        # commands, so it's used as a numeric fallback when $? is false.
        $cmdexExitCode = if ($?) { 0 } elseif ($LASTEXITCODE) { $LASTEXITCODE } else { 1 }
        [Console]::Out.Write("`e]133;D;$cmdexNonce;$cmdexExitCode`a")
        & (Get-Command __cmdex_original_prompt -CommandType Function)
    }
}

if ((Get-Command PSConsoleHostReadLine -CommandType Function -ErrorAction SilentlyContinue) -and
    -not (Test-Path Function:\global:__cmdex_original_read_line)) {
    Rename-Item Function:\global:PSConsoleHostReadLine Function:\global:__cmdex_original_read_line -ErrorAction SilentlyContinue

    function global:PSConsoleHostReadLine {
        $cmdexLine = & (Get-Command __cmdex_original_read_line -CommandType Function)
        [Console]::Out.Write("`e]133;C;$cmdexNonce`a")
        return $cmdexLine
    }
}
