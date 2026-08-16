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

if (-not (Test-Path Function:\global:__cmdex_original_prompt)) {
    Rename-Item Function:\global:prompt Function:\global:__cmdex_original_prompt -ErrorAction SilentlyContinue

    function global:prompt {
        # Read *before* anything else runs, so it reflects the command that
        # just finished rather than something this function itself did.
        # $? is PowerShell's own pass/fail signal (works for cmdlets and
        # native commands alike); $LASTEXITCODE is only ever set by native
        # commands, so it's used as a numeric fallback when $? is false.
        $cmdexExitCode = if ($?) { 0 } elseif ($LASTEXITCODE) { $LASTEXITCODE } else { 1 }
        [Console]::Out.Write("`e]133;D;$cmdexExitCode`a")
        & (Get-Command __cmdex_original_prompt -CommandType Function)
    }
}

if ((Get-Command PSConsoleHostReadLine -CommandType Function -ErrorAction SilentlyContinue) -and
    -not (Test-Path Function:\global:__cmdex_original_read_line)) {
    Rename-Item Function:\global:PSConsoleHostReadLine Function:\global:__cmdex_original_read_line -ErrorAction SilentlyContinue

    function global:PSConsoleHostReadLine {
        $cmdexLine = & (Get-Command __cmdex_original_read_line -CommandType Function)
        [Console]::Out.Write("`e]133;C`a")
        return $cmdexLine
    }
}
