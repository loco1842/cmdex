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
    if ($cmdexNonceFile -and (Test-Path -LiteralPath $cmdexNonceFile)) {
        # -ErrorAction Stop turns a read failure (permissions, the file
        # vanishing between the Test-Path above and this read, ...) into a
        # catchable exception instead of a non-terminating error that would
        # otherwise print to the user's console on every session start.
        try {
            $cmdexNonceValue = Get-Content -LiteralPath $cmdexNonceFile -Raw -ErrorAction Stop
        } catch {
            $cmdexNonceValue = $null
        }
        Remove-Item -LiteralPath $cmdexNonceFile -ErrorAction SilentlyContinue

        # Only install $global:cmdexNonce when the read actually produced a
        # real value — a null/empty result (the catch above, or an empty
        # file) must leave it unset rather than defined-but-blank, so the
        # "do we have a nonce at all" check below correctly skips installing
        # the marker-emitting wrappers entirely for this session.
        if ($cmdexNonceValue) {
            $global:cmdexNonce = $cmdexNonceValue
        }
    }
    Remove-Item Env:\CMDEX_OSC_NONCE_FILE -ErrorAction SilentlyContinue
}

# Without a nonce, terminal_capture.go's stripNonce can never authenticate a
# marker this session emits (see the nonce comment above) — GetLastOutput
# would simply stay Available=false forever and the frontend would fall back
# to scraping the xterm buffer, same as an uninstrumented shell. Skipping the
# wrapper installation entirely in that case avoids wrapping the user's real
# prompt/PSConsoleHostReadLine for a feature that can't work this session
# anyway.
if (Test-Path Variable:\global:cmdexNonce) {
    if (-not (Test-Path Variable:\global:__cmdexOriginalPrompt)) {
        # Capture the ORIGINAL function as a scriptblock reference (same
        # technique VS Code's own pwsh shell integration uses) rather than
        # Rename-Item-ing it under a new name. Renaming it turned out to
        # break PSReadLine's real ReadLine implementation below: invoked
        # under any name other than the literal "PSConsoleHostReadLine",
        # it returned an empty line instantly instead of blocking for
        # keyboard input — every prompt redraw looked like the user had
        # just pressed Enter on an empty line, producing an endless
        # self-sustaining "PS>" loop with no real keystrokes involved
        # (confirmed by tracing raw PTY output: C marker, "\r\n", D marker,
        # new prompt, repeat — several times a second, forever). Capturing
        # the scriptblock directly and invoking it via .Invoke() leaves
        # PSReadLine's real function registered under its original name,
        # so it keeps behaving normally.
        $global:__cmdexOriginalPrompt = $function:global:prompt

        function global:prompt {
            # Read *before* anything else runs, so it reflects the command
            # that just finished rather than something this function itself
            # did. $? is PowerShell's own pass/fail signal (works for
            # cmdlets and native commands alike); $LASTEXITCODE is only ever
            # set by native commands, so it's used as a numeric fallback
            # when $? is false.
            $cmdexExitCode = if ($?) { 0 } elseif ($LASTEXITCODE) { $LASTEXITCODE } else { 1 }

            # $LASTEXITCODE is sticky: a non-terminating cmdlet failure
            # (e.g. Get-Item on a missing path) sets $? to $false but never
            # touches $LASTEXITCODE at all. Without clearing it here, a
            # later such failure would silently reuse whatever numeric code
            # the LAST native command happened to leave behind instead of
            # falling back to 1. Clearing it right after reading it above —
            # before the next command runs — means it can only read as
            # non-null again if a native command that actually ran after
            # this point set it.
            $global:LASTEXITCODE = $null

            [Console]::Out.Write("`e]133;D;$cmdexNonce;$cmdexExitCode`a")
            $global:__cmdexOriginalPrompt.Invoke()
        }
    }

    if ((Get-Command PSConsoleHostReadLine -CommandType Function -ErrorAction SilentlyContinue) -and
        -not (Test-Path Variable:\global:__cmdexOriginalReadLine)) {
        # See the prompt wrapper's comment above — this is the exact
        # function whose Rename-Item-based capture broke PSReadLine.
        $global:__cmdexOriginalReadLine = $function:global:PSConsoleHostReadLine

        function global:PSConsoleHostReadLine {
            $cmdexLine = $global:__cmdexOriginalReadLine.Invoke()
            [Console]::Out.Write("`e]133;C;$cmdexNonce`a")
            return $cmdexLine
        }
    }
}
